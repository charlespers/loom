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
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	Mode         string // "full" (default, audit.jsonl) or "public" (audit.public.jsonl, structural)
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

	res := &VerifyResult{RunDir: runDir, RunID: m.RunID, Mode: "full"}

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

	// Integrity check: events.jsonl + audit files match the hashes
	// recorded at finalization. Catches post-hoc tampering of events.
	// jsonl, which the chain alone doesn't anchor.
	if m.Integrity != nil {
		for _, fc := range []struct {
			name, want string
		}{
			{"events.jsonl", m.Integrity.EventsSHA256},
			{"audit.jsonl", m.Integrity.AuditPrivateSHA256},
			{"audit.public.jsonl", m.Integrity.AuditPublicSHA256},
		} {
			if err := verifyFileHash(filepath.Join(runDir, fc.name), fc.want); err != nil {
				res.Verified = false
				res.BrokenReason = fc.name + ": " + err.Error()
				return res, 2
			}
		}
	}
	return res, 0
}

// RunPublic verifies the structural chain of audit.public.jsonl.
//
// Public records share the same chain hashes as private records (the
// hash is computed from the private canonical payload), so a holder
// of only audit.public.jsonl cannot recompute and confirm chain.this
// — the private content is required for that. What they CAN check
// from the public file alone:
//
//   - chain links are continuous: each record's chain.prev equals the
//     prior record's chain.this
//   - the first record's chain.prev is the all-zeros sentinel
//   - the last record's chain.this matches manifest.audit_chain.head
//   - audit.public.jsonl's file hash matches manifest.integrity
//
// This is "the chain is internally consistent and was anchored to the
// manifest at finalization" — meaningful, but weaker than full mode.
// Catches: dropped records, re-ordering, head tampering relative to
// the chain. Misses: any forgery that updates chain hashes coherently
// across records and the manifest.
func RunPublic(runDir string) (*VerifyResult, int) {
	m, err := runinfo.LoadManifest(runDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify: cannot read manifest.json:", err)
		return nil, 4
	}

	auditPath := filepath.Join(runDir, "audit.public.jsonl")
	f, err := os.Open(auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "verify: %s missing — nothing to verify\n", auditPath)
			return nil, 3
		}
		fmt.Fprintln(os.Stderr, "verify: cannot open audit.public.jsonl:", err)
		return nil, 3
	}
	defer f.Close()

	res := &VerifyResult{RunDir: runDir, RunID: m.RunID, Mode: "public"}

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
		if r.Chain.Prev != prev {
			s := r.Seq
			res.BrokenAtSeq = &s
			res.BrokenReason = fmt.Sprintf(
				"prev mismatch at seq %d: stored %s, expected %s "+
					"(chain link broken)",
				r.Seq, r.Chain.Prev, prev)
			return res, 2
		}
		prev = r.Chain.This
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

	// File integrity hash for the public file (if recorded).
	if m.Integrity != nil && m.Integrity.AuditPublicSHA256 != "" {
		if err := verifyFileHash(auditPath, m.Integrity.AuditPublicSHA256); err != nil {
			res.Verified = false
			res.BrokenReason = "audit.public.jsonl: " + err.Error()
			return res, 2
		}
	}
	return res, 0
}

// verifyFileHash re-hashes the file at path and compares to want.
// An empty want means the manifest didn't record a hash for that file
// (older runs pre-integrity) and is treated as a no-op success.
func verifyFileHash(path, want string) error {
	if want == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("hash mismatch (stored %s, computed %s)",
			short(want), short(got))
	}
	return nil
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
			"value and ensuring the head matches manifest.json.\n\n" +
			"--public reads audit.public.jsonl instead. Public-mode verification\n" +
			"is structural only — without the private record content, the hash\n" +
			"itself cannot be recomputed; we verify chain.prev/chain.this form\n" +
			"a valid chain and that the head matches the manifest. This is\n" +
			"what a regulator with only the public artifact can independently\n" +
			"check.\n\n" +
			"Exit codes: 0 verified  2 chain broken  3 file missing  4 config mismatch.",
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
			public, _ := cmd.Flags().GetBool("public")
			pubkeyPath, _ := cmd.Flags().GetString("pubkey")
			if pubkeyPath == "" {
				pubkeyPath = os.Getenv("LOOM_VERIFY_PUBKEY")
			}

			var res *VerifyResult
			var code int
			if public {
				res, code = RunPublic(runDir)
			} else {
				res, code = Run(runDir)
			}
			renderResult(cmd, res, code)
			if code != 0 {
				os.Exit(code)
			}

			// Signature pass — only runs if the run dir came from an
			// extracted export bundle (digest.txt present). If
			// digest.sig is also present, signature verification is
			// attempted; --pubkey makes signature failure fatal.
			sigCode := verifySignedExport(cmd, runDir, pubkeyPath)
			if sigCode != 0 {
				os.Exit(sigCode)
			}
			return nil
		},
	}
	c.Flags().Bool("public", false, "verify against audit.public.jsonl (structural chain only)")
	c.Flags().String("pubkey", "", "path to operator's Ed25519 public key (PEM); falls back to $LOOM_VERIFY_PUBKEY")
	return c
}

// verifySignedExport runs the post-chain signature check when the run
// directory looks like an extracted `loom export` bundle (digest.txt
// present). Behavior:
//
//   - no digest.txt:           silent no-op (regular run dir)
//   - digest.txt only:         re-checks file hashes match digest.txt;
//                              prints note that the bundle is unsigned
//   - digest.txt + digest.sig: same plus Ed25519 signature check;
//                              requires a pubkey (from --pubkey or
//                              $LOOM_VERIFY_PUBKEY) to be fatal — if
//                              no pubkey supplied, prints a warning
//                              that the signature is present but
//                              unverified
//
// Returns the exit code to surface (0 on success, 2 on tamper).
func verifySignedExport(cmd *cobra.Command, runDir, pubkeyPath string) int {
	digestPath := filepath.Join(runDir, "digest.txt")
	digestBytes, err := os.ReadFile(digestPath)
	if err != nil {
		// No digest.txt — not an export bundle. Done.
		return 0
	}

	w := cmd.OutOrStdout()

	// Re-derive every file's SHA-256 and compare against digest.txt.
	entries, runID, perr := parseDigest(digestBytes)
	if perr != nil {
		fmt.Fprintf(w, "verify: ✗ digest.txt malformed: %v\n", perr)
		return 2
	}
	for _, e := range entries {
		got, _, herr := hashFile(filepath.Join(runDir, e.Name))
		if herr != nil {
			fmt.Fprintf(w, "verify: ✗ digest.txt names %s but: %v\n", e.Name, herr)
			return 2
		}
		if got != e.SHA256 {
			fmt.Fprintf(w, "verify: ✗ %s digest mismatch (stored %s, computed %s)\n",
				e.Name, short(e.SHA256), short(got))
			return 2
		}
	}

	sigPath := filepath.Join(runDir, "digest.sig")
	sig, sigErr := os.ReadFile(sigPath)

	switch {
	case sigErr != nil && pubkeyPath != "":
		fmt.Fprintf(w, "verify: ✗ --pubkey provided but digest.sig missing — bundle is unsigned\n")
		return 2
	case sigErr != nil:
		fmt.Fprintf(w, "verify: ✓ %s · %d-file digest re-checked (unsigned bundle)\n",
			runID, len(entries))
		return 0
	}

	// digest.sig present.
	if pubkeyPath == "" {
		fp := sha256.Sum256(loadPubkeyBytesIfAny(runDir))
		fmt.Fprintf(w, "verify: ⚠ digest.sig present but no --pubkey provided\n"+
			"        bundle's pubkey-sha256 = %s\n"+
			"        re-run with --pubkey <path> (or $LOOM_VERIFY_PUBKEY) to verify signature\n",
			short(hex.EncodeToString(fp[:])))
		return 0
	}

	pubBytes, err := os.ReadFile(pubkeyPath)
	if err != nil {
		fmt.Fprintf(w, "verify: ✗ cannot read --pubkey %s: %v\n", pubkeyPath, err)
		return 4
	}
	pub, err := parsePublicKeyPEM(pubBytes)
	if err != nil {
		fmt.Fprintf(w, "verify: ✗ parse pubkey: %v\n", err)
		return 4
	}
	if !ed25519.Verify(pub, digestBytes, sig) {
		fmt.Fprintf(w, "verify: ✗ Ed25519 signature INVALID for digest.txt under provided pubkey\n")
		return 2
	}
	fp := sha256.Sum256(pub)
	fmt.Fprintf(w, "verify: ✓ %s · digest re-checked, signature valid (pubkey-sha256 %s)\n",
		runID, short(hex.EncodeToString(fp[:])))
	return 0
}

type digestEntry struct {
	Name   string
	SHA256 string
	Bytes  int64
}

// parseDigest reads a "loom.export.v1 <run_id>\n<sha256>  <bytes>  <name>\n..."
// digest manifest. Empty lines are tolerated. The format is intentionally
// simple so an auditor can sanity-check by eye.
func parseDigest(b []byte) ([]digestEntry, string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(b)))
	scanner.Buffer(make([]byte, 1<<16), 1<<24)
	first := true
	runID := ""
	var out []digestEntry
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if first {
			first = false
			parts := strings.SplitN(line, " ", 2)
			if len(parts) != 2 || parts[0] != "loom.export.v1" {
				return nil, "", fmt.Errorf("missing or unknown header (got %q)", line)
			}
			runID = parts[1]
			continue
		}
		fields := strings.SplitN(line, "  ", 3)
		if len(fields) != 3 {
			return nil, "", fmt.Errorf("malformed line %q", line)
		}
		var size int64
		if _, err := fmt.Sscanf(fields[1], "%d", &size); err != nil {
			return nil, "", fmt.Errorf("malformed size in %q: %w", line, err)
		}
		out = append(out, digestEntry{Name: fields[2], SHA256: fields[0], Bytes: size})
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	return out, runID, nil
}

// hashFile streams a file through SHA-256 and returns hex hash + size.
func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// loadPubkeyBytesIfAny reads pubkey.pem from the bundle if present
// and returns the parsed Ed25519 public key bytes; on any error
// returns nil so the caller can show "(unknown)" gracefully.
func loadPubkeyBytesIfAny(runDir string) []byte {
	raw, err := os.ReadFile(filepath.Join(runDir, "pubkey.pem"))
	if err != nil {
		return nil
	}
	pub, err := parsePublicKeyPEM(raw)
	if err != nil {
		return nil
	}
	return pub
}

// parsePublicKeyPEM decodes a PKIX-PEM Ed25519 public key.
func parsePublicKeyPEM(raw []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("not PEM-encoded")
	}
	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("unexpected PEM type %q (want PUBLIC KEY)", block.Type)
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 public key (%T)", k)
	}
	return pub, nil
}

func renderResult(cmd *cobra.Command, r *VerifyResult, code int) {
	w := cmd.OutOrStdout()
	if r == nil {
		return
	}
	if code == 0 {
		modeNote := ""
		if r.Mode == "public" {
			modeNote = " (public/structural)"
		}
		fmt.Fprintf(w, "verify: ✓ %d audit record%s, head %s%s\n",
			r.Records, plural(r.Records), short(r.HeadHex), modeNote)
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
