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
	Schema           string             `json:"schema"`
	LoomVersion      string             `json:"loom_version"`
	WireSchema       string             `json:"wire_schema"`
	RunID            string             `json:"run_id"`
	StartedAt        string             `json:"started_at"`
	StartedAtUnixNS  uint64             `json:"started_at_unix_ns"`
	EndedAt          string             `json:"ended_at"`
	EndedAtUnixNS    uint64             `json:"ended_at_unix_ns"`
	DurationMS       uint64             `json:"duration_ms"`
	Status           string             `json:"status"`
	Process          json.RawMessage    `json:"process"`
	Host             json.RawMessage    `json:"host"`
	Counts           ManifestCounts     `json:"counts"`
	Spans            json.RawMessage    `json:"spans"`
	AuditChain       ManifestAuditChain `json:"audit_chain"`
	Files            json.RawMessage    `json:"files"`
}

type ManifestCounts struct {
	EventsTotal uint64            `json:"events_total"`
	ByCategory  map[string]uint64 `json:"by_category"`
}

type ManifestAuditChain struct {
	Head  string `json:"head"`
	Count uint64 `json:"count"`
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
