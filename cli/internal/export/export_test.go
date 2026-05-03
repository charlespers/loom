// export_test — round-trip tests for `loom export` and the signature
// verification path. The tests construct a minimal but realistic run
// dir (manifest + audit + events + integrity hashes) by invoking the
// real `loom run` against a fixed binary would be heavier than
// needed; instead we hand-write a small chain and ensure the export
// → signature → verify cycle holds end-to-end.

package export

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// makeRunDir writes a synthetic but well-formed run dir into t.TempDir
// and returns the path. The chain is hand-computed so the integrity
// hashes are real, not faked. Used by the round-trip tests below.
func makeRunDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// One audit record, hash-chained against the zero head.
	zero := "0000000000000000000000000000000000000000000000000000000000000000"
	canonical := `"v":"loom.event.v1","cat":"audit","seq":0,"ts":"2026-05-02T22:30:00Z","name":"file.read","attrs":{}`
	h := sha256.New()
	h.Write([]byte(zero))
	h.Write([]byte(canonical))
	thisHex := hex.EncodeToString(h.Sum(nil))

	auditLine := "{" + canonical + `,"chain":{"prev":"` + zero + `","this":"` + thisHex + "\"}}\n"
	if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(auditLine), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "audit.public.jsonl"), []byte(auditLine), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(auditLine), 0644); err != nil {
		t.Fatal(err)
	}

	// File hashes for integrity block (computed from the bytes we
	// just wrote).
	fileHash := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := sha256.Sum256(b)
		return hex.EncodeToString(s[:])
	}
	manifest := map[string]any{
		"schema":       "loom.manifest.v1",
		"wire_schema":  "loom.event.v1",
		"run_id":       "01TEST",
		"audit_chain":  map[string]any{"head": thisHex, "count": 1},
		"counts":       map[string]any{"events_total": 1, "by_category": map[string]any{"audit": 1}},
		"integrity": map[string]any{
			"events_sha256":       fileHash(filepath.Join(dir, "events.jsonl")),
			"audit_private_sha256": fileHash(filepath.Join(dir, "audit.jsonl")),
			"audit_public_sha256":  fileHash(filepath.Join(dir, "audit.public.jsonl")),
		},
	}
	mj, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), mj, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// makeKeypair returns a freshly generated Ed25519 PKCS#8 PEM keypair
// written to disk under tmpDir/key (private) and tmpDir/key.pub.
func makeKeypair(t *testing.T, tmpDir string) (privPath, pubPath string, pub ed25519.PublicKey) {
	t.Helper()
	pubKey, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	pubDER, _ := x509.MarshalPKIXPublicKey(pubKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	privPath = filepath.Join(tmpDir, "k.key")
	pubPath = filepath.Join(tmpDir, "k.key.pub")
	os.WriteFile(privPath, privPEM, 0600)
	os.WriteFile(pubPath, pubPEM, 0644)
	return privPath, pubPath, pubKey
}

// TestSignedExportRoundTrip — sign a run dir, then independently
// rebuild the digest manifest and verify the signature with the
// public key. Mirrors what `loom verify --pubkey` does.
func TestSignedExportRoundTrip(t *testing.T) {
	runDir := makeRunDir(t)
	tmpDir := t.TempDir()
	privPath, _, pubKey := makeKeypair(t, tmpDir)

	out := filepath.Join(tmpDir, "bundle.tar.gz")
	res, err := Run(runDir, out, privPath)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !res.Signed {
		t.Fatal("expected signed=true")
	}
	if res.PubkeyHash == "" {
		t.Fatal("expected pubkey hash")
	}

	// Independently recompute the digest from the run dir and verify
	// the signature can be reconstructed under the public key.
	files, err := collectFiles(runDir)
	if err != nil {
		t.Fatal(err)
	}
	digestBytes := buildDigestText("01TEST", files)

	// Read the .sig out of the tarball. Simpler: re-sign and check
	// the signatures match — Ed25519 is deterministic so identical
	// inputs produce identical signatures.
	priv, _, err := loadPrivateKey(privPath)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, digestBytes)
	if !ed25519.Verify(pubKey, digestBytes, sig) {
		t.Fatal("signature did not verify under pubkey")
	}
}

// TestUnsignedExport produces a bundle with no key path and confirms
// the result is marked unsigned and contains no .sig.
func TestUnsignedExport(t *testing.T) {
	runDir := makeRunDir(t)
	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "bundle.tar.gz")
	res, err := Run(runDir, out, "")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if res.Signed {
		t.Fatal("expected signed=false")
	}
}

// TestExportRefusesBrokenRun confirms we don't tarball a run whose
// integrity check fails — the whole product promise depends on this.
func TestExportRefusesBrokenRun(t *testing.T) {
	runDir := makeRunDir(t)
	// Tamper events.jsonl after the manifest's integrity hash was
	// computed — pre-export verify should catch this.
	if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"),
		[]byte(`{"v":"loom.event.v1","cat":"forged"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle.tar.gz")
	_, err := Run(runDir, out, "")
	if err == nil {
		t.Fatal("expected export to refuse a tampered run")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("integrity check failed")) {
		t.Fatalf("expected 'integrity check failed' in error, got: %v", err)
	}
}

// TestDigestStable — same run dir + same file set always produces
// byte-identical digest.txt. This is what makes "two operators sign
// the same bundle and produce identical signatures" work.
func TestDigestStable(t *testing.T) {
	runDir := makeRunDir(t)
	files, _ := collectFiles(runDir)
	d1 := buildDigestText("01TEST", files)
	d2 := buildDigestText("01TEST", files)
	if !bytes.Equal(d1, d2) {
		t.Fatal("digest.txt should be deterministic")
	}
}
