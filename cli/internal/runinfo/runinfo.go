// Package runinfo locates a loom run on disk and reads its manifest. It's
// the small read-only library that `loom verify`, `loom report`, and the
// future `loom view` all share.
package runinfo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoomHome returns the artifact root, honoring $LOOM_HOME with a fallback
// to ~/.loom.
func LoomHome() (string, error) {
	if h := os.Getenv("LOOM_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".loom"), nil
}

// ResolveRun turns a user-supplied identifier into a run directory.
//
//   - "" or "latest" — most recently modified subdir of $LOOM_HOME/runs
//   - a 26-char ULID — exact-match under $LOOM_HOME/runs
//   - any prefix of a ULID — disambiguates if exactly one run matches
//   - an absolute path — used directly
func ResolveRun(id string) (string, error) {
	if filepath.IsAbs(id) {
		if _, err := os.Stat(id); err != nil {
			return "", fmt.Errorf("run dir not found: %s", id)
		}
		return id, nil
	}
	home, err := LoomHome()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, "runs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("no runs found at %s: %w", root, err)
	}
	if id == "" || id == "latest" {
		return mostRecent(root, entries)
	}
	// Exact match first.
	exact := filepath.Join(root, id)
	if _, err := os.Stat(exact); err == nil {
		return exact, nil
	}
	// Prefix match.
	var matches []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), id) {
			matches = append(matches, e.Name())
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no run matches %q under %s", id, root)
	case 1:
		return filepath.Join(root, matches[0]), nil
	default:
		return "", fmt.Errorf("ambiguous prefix %q: matches %d runs", id, len(matches))
	}
}

func mostRecent(root string, entries []fs.DirEntry) (string, error) {
	type cand struct {
		name string
		mod  int64
	}
	var c []cand
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		c = append(c, cand{name: e.Name(), mod: info.ModTime().UnixNano()})
	}
	if len(c) == 0 {
		return "", errors.New("no runs to choose from")
	}
	sort.Slice(c, func(i, j int) bool { return c[i].mod > c[j].mod })
	return filepath.Join(root, c[0].name), nil
}

// Manifest is the typed view of manifest.json. Fields not relevant to the
// verifier are decoded as json.RawMessage so we don't have to track every
// schema addition here.
type Manifest struct {
	Schema           string                  `json:"schema"`
	LoomVersion      string                  `json:"loom_version"`
	WireSchema       string                  `json:"wire_schema"`
	RunID            string                  `json:"run_id"`
	StartedAt        string                  `json:"started_at"`
	StartedAtUnixNS  uint64                  `json:"started_at_unix_ns"`
	EndedAt          string                  `json:"ended_at"`
	EndedAtUnixNS    uint64                  `json:"ended_at_unix_ns"`
	DurationMS       uint64                  `json:"duration_ms"`
	Status           string                  `json:"status"`
	Process          json.RawMessage         `json:"process"`
	Host             json.RawMessage         `json:"host"`
	Counts           ManifestCounts          `json:"counts"`
	Spans            json.RawMessage         `json:"spans"`
	DeviceSpans      json.RawMessage         `json:"device_spans,omitempty"`
	AuditChain       ManifestAuditChain      `json:"audit_chain"`
	Files            json.RawMessage         `json:"files"`
	Reproducibility  *Reproducibility        `json:"reproducibility,omitempty"`
	Integrity        *Integrity              `json:"integrity,omitempty"`
}

// Reproducibility carries the model + prompt + seed metadata operators
// inject through env vars at run time so a regulator can replay an
// auditable decision. None of these fields are required; missing
// values just leave gaps in the report.
type Reproducibility struct {
	ModelID       string `json:"model_id,omitempty"`
	ModelHash     string `json:"model_hash,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
	Seed          string `json:"seed,omitempty"`
	Tag           string `json:"tag,omitempty"`
}

// Integrity carries hashes of the per-run files so a verifier can
// detect post-hoc tampering of events.jsonl in addition to the audit
// chain. The audit chain head is duplicated here for one-stop verify.
type Integrity struct {
	EventsSHA256       string `json:"events_sha256,omitempty"`
	AuditPrivateSHA256 string `json:"audit_private_sha256,omitempty"`
	AuditPublicSHA256  string `json:"audit_public_sha256,omitempty"`
	AuditChainHead     string `json:"audit_chain_head,omitempty"`
}

type ManifestCounts struct {
	EventsTotal uint64            `json:"events_total"`
	ByCategory  map[string]uint64 `json:"by_category"`
}

type ManifestAuditChain struct {
	Head  string `json:"head"`
	Count uint64 `json:"count"`
}

// RunSummary is the lightweight view of a run, suitable for tabular
// listings (`loom ls`). Built from manifest.json + a single ProcessField
// lookup. Failure to read manifest yields a placeholder summary marked
// `Status: "incomplete"` so partially-written runs are still visible.
type RunSummary struct {
	RunID      string
	Path       string
	Status     string
	StartedAt  string
	DurationMS uint64
	Command    string
	EventsTotal uint64
	AuditCount  uint64
	ErrorCount  uint64
}

// ListRuns enumerates every run under $LOOM_HOME/runs, newest first.
// Best-effort — runs whose manifest is missing or unparseable are
// included as `Status: "incomplete"`. Limit to 0 for "all".
func ListRuns(limit int) ([]RunSummary, error) {
	home, err := LoomHome()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, "runs")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list runs: %w", err)
	}
	type pair struct {
		name string
		mod  int64
	}
	var ps []pair
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		ps = append(ps, pair{name: e.Name(), mod: info.ModTime().UnixNano()})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].mod > ps[j].mod })
	if limit > 0 && len(ps) > limit {
		ps = ps[:limit]
	}
	out := make([]RunSummary, 0, len(ps))
	for _, p := range ps {
		dir := filepath.Join(root, p.name)
		summary := RunSummary{RunID: p.name, Path: dir, Status: "incomplete"}
		if m, err := LoadManifest(dir); err == nil {
			summary.Status = m.Status
			summary.StartedAt = m.StartedAt
			summary.DurationMS = m.DurationMS
			summary.EventsTotal = m.Counts.EventsTotal
			summary.AuditCount = m.AuditChain.Count
			if m.Counts.ByCategory != nil {
				summary.ErrorCount = m.Counts.ByCategory["error"]
			}
			var proc map[string]json.RawMessage
			if err := json.Unmarshal(m.Process, &proc); err == nil {
				if v, ok := proc["command"]; ok {
					_ = json.Unmarshal(v, &summary.Command)
				}
			}
		}
		out = append(out, summary)
	}
	return out, nil
}

// LoadManifest reads manifest.json from a run dir.
func LoadManifest(runDir string) (*Manifest, error) {
	path := filepath.Join(runDir, "manifest.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Schema != "loom.manifest.v1" {
		return nil, fmt.Errorf("unsupported manifest schema: %q", m.Schema)
	}
	return &m, nil
}
