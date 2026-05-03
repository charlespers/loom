package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chainHash mirrors the embed lib's hashing rule:
// sha256(prev_hex_string_bytes || canonical_payload_bytes).
func chainHash(prev string, canonical string) string {
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write([]byte(canonical))
	return hex.EncodeToString(h.Sum(nil))
}

// writeChainFixture creates manifest.json + audit.jsonl with a valid
// chain over `n` records. Returns the run dir and the head hash.
func writeChainFixture(t *testing.T, n int) (string, string) {
	t.Helper()
	dir := t.TempDir()

	zero := strings.Repeat("0", 64)
	prev := zero

	var auditLines []string
	for i := 0; i < n; i++ {
		canonical := `"v":"loom.event.v1","cat":"audit","seq":` +
			itoa(i) +
			`,"ts":"2026-05-02T22:00:00.` + leadZero(i, 3) + `Z","name":"step.` +
			itoa(i) + `","attrs":{}`
		this := chainHash(prev, canonical)
		line := `{` + canonical + `,"chain":{"prev":"` + prev + `","this":"` + this + `"}}`
		auditLines = append(auditLines, line)
		prev = this
	}
	auditContent := strings.Join(auditLines, "\n") + "\n"
	must(t, os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(auditContent), 0o600))

	head := zero
	if n > 0 {
		head = prev
	}
	manifest := `{"schema":"loom.manifest.v1","loom_version":"0.1.0","wire_schema":"loom.event.v1",` +
		`"run_id":"01J3K","started_at":"2026-05-02T22:00:00.000Z","started_at_unix_ns":0,` +
		`"ended_at":"","ended_at_unix_ns":0,"duration_ms":0,"status":"completed",` +
		`"counts":{"events_total":` + itoa(n) + `,"by_category":{"audit":` + itoa(n) + `}},` +
		`"audit_chain":{"head":"` + head + `","count":` + itoa(n) + `}}`
	must(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644))
	return dir, head
}

func TestRun_VerifiesValidChain(t *testing.T) {
	dir, head := writeChainFixture(t, 5)
	res, code := Run(dir)
	if code != 0 {
		t.Fatalf("expected code 0, got %d (%s)", code, res.BrokenReason)
	}
	if !res.Verified {
		t.Errorf("expected Verified=true")
	}
	if res.Records != 5 {
		t.Errorf("expected 5 records, got %d", res.Records)
	}
	if res.HeadHex != head {
		t.Errorf("head mismatch: got %s, want %s", res.HeadHex, head)
	}
}

func TestRun_DetectsTamperedRecord(t *testing.T) {
	dir, _ := writeChainFixture(t, 3)
	// Tamper with the second record's name. The chain.this for record 1
	// will no longer match the recomputed hash.
	auditPath := filepath.Join(dir, "audit.jsonl")
	b, _ := os.ReadFile(auditPath)
	tampered := strings.Replace(string(b), `"name":"step.1"`, `"name":"step.X"`, 1)
	must(t, os.WriteFile(auditPath, []byte(tampered), 0o600))

	res, code := Run(dir)
	if code != 2 {
		t.Fatalf("expected code 2 (chain broken), got %d", code)
	}
	if res.Verified {
		t.Errorf("Verified should be false on tamper")
	}
	if res.BrokenAtSeq == nil || *res.BrokenAtSeq != 1 {
		t.Errorf("expected break at seq 1, got %v", res.BrokenAtSeq)
	}
}

func TestRun_EmptyChain(t *testing.T) {
	dir, _ := writeChainFixture(t, 0)
	res, code := Run(dir)
	if code != 0 {
		t.Errorf("empty chain should verify, got code %d (%s)", code, res.BrokenReason)
	}
}

func TestRun_MissingAuditFile(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "manifest.json"),
		[]byte(`{"schema":"loom.manifest.v1","run_id":"x","audit_chain":{"head":"","count":0}}`), 0o644))
	_, code := Run(dir)
	if code != 3 {
		t.Errorf("missing audit.jsonl should exit 3, got %d", code)
	}
}

func TestRun_MissingManifest(t *testing.T) {
	dir := t.TempDir()
	_, code := Run(dir)
	if code != 4 {
		t.Errorf("missing manifest.json should exit 4, got %d", code)
	}
}

// Helpers because tests should not pull in strconv just for two int->string calls.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func leadZero(n, width int) string {
	s := itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
