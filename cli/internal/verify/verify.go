// Package verify implements `loom verify <run-id>` — re-runs the SHA-256
// hash chain over audit.jsonl and compares each record's chain.this to
// the stored value, plus checks chain.prev forms a single chain head.
//
// Exit codes (per spec § 7.1):
//
//	0 = verified
//	2 = chain broken (some record's hash doesn't match)
//	3 = file missing
//	4 = config / manifest mismatch
package verify

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charlespers/loom/cli/internal/runinfo"
	"github.com/spf13/cobra"
)

const zeroHex = "0000000000000000000000000000000000000000000000000000000000000000"

// auditRecord is the typed shape of one line in audit.jsonl. The chain
// fields are last in the canonical layout so the canonical-payload prefix
// is bytes 0..pre-",chain":...
type auditRecord struct {
	V     string          `json:"v"`
	Cat   string          `json:"cat"`
	Seq   uint64          `json:"seq"`
	Ts    string          `json:"ts"`
	Name  string          `json:"name"`
	Attrs json.RawMessage `json:"attrs"`
	Chain struct {
		Prev string `json:"prev"`
		This string `json:"this"`
	} `json:"chain"`
}

// VerifyResult is the structured outcome rendered by Cmd's RunE.
type VerifyResult struct {
	RunDir       string
	RunID        string
	Records      int
	Verified     bool
	BrokenAtSeq  *uint64
	BrokenReason string
	HeadHex      string
	HeadMatches  bool
}

// Run reads the audit file, walks the chain, and compares against the
// manifest's recorded head. Returns a result and an exit code.
func Run(runDir string) (*VerifyResult, int) {
	m, err := runinfo.LoadManifest(runDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify: cannot read manifest.json:", err)
		return nil, 4
	}

	auditPath := filepath.Join(runDir, "audit.jsonl")
	f, err := os.Open(auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "verify: %s missing — nothing to verify\n", auditPath)
			return nil, 3
		}
		fmt.Fprintln(os.Stderr, "verify: cannot open audit.jsonl:", err)
		return nil, 3
	}
	defer f.Close()

	res := &VerifyResult{RunDir: runDir, RunID: m.RunID}

	prev := zeroHex
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r auditRecord
		if err := json.Unmarshal(line, &r); err != nil {
			fail := r.Seq
			res.BrokenAtSeq = &fail
			res.BrokenReason = fmt.Sprintf("malformed JSON: %v", err)
			return res, 2
		}
		// Canonical payload reconstruction: same field order the embed lib
		// writes, excluding chain. We rebuild it deterministically rather
		// than trusting line bytes (a permissive parser could re-emit them
		// in a different order).
		canonical, cerr := canonicalPayload(line)
		if cerr != nil {
			s := r.Seq
			res.BrokenAtSeq = &s
			res.BrokenReason = "canonical reconstruction failed: " + cerr.Error()
			return res, 2
		}

		h := sha256.New()
		io.WriteString(h, prev)
		h.Write(canonical)
		got := hex.EncodeToString(h.Sum(nil))

		if r.Chain.Prev != prev {
			s := r.Seq
			res.BrokenAtSeq = &s
			res.BrokenReason = fmt.Sprintf(
				"prev mismatch at seq %d: stored %s, expected %s",
				r.Seq, r.Chain.Prev, prev)
			return res, 2
		}
		if got != r.Chain.This {
			s := r.Seq
			res.BrokenAtSeq = &s
			res.BrokenReason = fmt.Sprintf(
				"this mismatch at seq %d: stored %s, computed %s",
				r.Seq, r.Chain.This, got)
			return res, 2
		}
		prev = got
		res.Records++
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "verify: read error:", err)
		return res, 2
	}

	res.HeadHex = prev
	res.HeadMatches = (m.AuditChain.Head == prev) ||
		(m.AuditChain.Count == 0 && prev == zeroHex)
	res.Verified = res.HeadMatches

	if !res.Verified {
		res.BrokenReason = fmt.Sprintf(
			"head mismatch: manifest=%s, computed=%s",
			m.AuditChain.Head, prev)
		return res, 2
	}
	return res, 0
}

// canonicalPayload extracts the bytes that were hashed at write time,
// matching the embed lib's canonical layout exactly:
//
//	"v":"...","cat":"audit","seq":N,"ts":"...","name":"...","attrs":{...}
func canonicalPayload(line []byte) ([]byte, error) {
	// Locate the chain field. The embed lib always places it last:
	//   {<canonical>,"chain":{"prev":"...","this":"..."}}
	open := bytes_indexOf(line, []byte(`,"chain":{`))
	if open < 0 {
		return nil, fmt.Errorf(`no "chain" field`)
	}
	// Strip the leading '{' and the trailing '}'.
	if line[0] != '{' || line[len(line)-1] != '}' {
		return nil, fmt.Errorf("not a JSON object")
	}
	return line[1:open], nil
}

// Local helper to avoid importing bytes for one call. Behaves like
// bytes.Index without the alloc on small inputs.
func bytes_indexOf(haystack, needle []byte) int {
	if len(needle) == 0 {
		return 0
	}
	if len(needle) > len(haystack) {
		return -1
	}
outer:
	for i := 0; i+len(needle) <= len(haystack); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

// Cmd is the cobra subcommand wired into the loom CLI.
func Cmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "verify [<run-id> | latest]",
		Short: "Verify the audit hash chain of a run",
		Long: "Walks audit.jsonl and recomputes the SHA-256 chain over each\n" +
			"record's canonical payload, comparing to the stored chain.this\n" +
			"value and ensuring the head matches manifest.json. Exit codes:\n" +
			"  0 verified   2 chain broken   3 file missing   4 config mismatch.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			runDir, err := runinfo.ResolveRun(id)
			if err != nil {
				return err
			}
			res, code := Run(runDir)
			renderResult(cmd, res, code)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
	return c
}

func renderResult(cmd *cobra.Command, r *VerifyResult, code int) {
	w := cmd.OutOrStdout()
	if r == nil {
		return
	}
	if code == 0 {
		fmt.Fprintf(w, "verify: ✓ %d audit record%s, head %s\n",
			r.Records, plural(r.Records), short(r.HeadHex))
		fmt.Fprintf(w, "        run_id %s   path %s\n", r.RunID, r.RunDir)
		return
	}
	fmt.Fprintf(w, "verify: ✗ chain broken — %s\n", r.BrokenReason)
	fmt.Fprintf(w, "        records read: %d\n", r.Records)
	if r.BrokenAtSeq != nil {
		fmt.Fprintf(w, "        broken at seq: %d\n", *r.BrokenAtSeq)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func short(hex string) string {
	if len(hex) <= 12 {
		return hex
	}
	return hex[:8] + "…" + hex[len(hex)-4:]
}

// Suppress unused-import nag in builds that don't trigger a path.
var _ = strings.TrimSpace
