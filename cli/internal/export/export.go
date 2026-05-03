// Package export implements `loom export <run-id>` — bundles a finished
// run's artifact directory into a single tar.gz suitable for handing
// to an auditor or opposing counsel, with an optional Ed25519
// signature over a digest manifest of every file in the bundle.
//
// The export wraps the existing per-run artifacts (manifest.json,
// events.jsonl, audit.jsonl, audit.public.jsonl, summary.md,
// report.html) and adds:
//
//   - digest.txt — line-per-file: "<sha256-hex>  <bytes>  <name>"
//                  followed by a header "loom.export.v1 <run_id>".
//                  This is the bytes that get signed.
//   - digest.sig — raw 64-byte Ed25519 signature over digest.txt
//                  (only when --key or LOOM_SIGN_KEY is provided).
//   - pubkey.pem — the operator's public key (informational; the
//                  auditor MUST verify they received this pubkey
//                  through a trusted out-of-band channel).
//
// Auditor verification: `loom verify <tarball> --pubkey path/to/pub.pem`
// or extract first then `loom verify <dir> --pubkey ...`.
package export

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charlespers/loom/cli/internal/runinfo"
	"github.com/charlespers/loom/cli/internal/verify"
	"github.com/spf13/cobra"
)

// digestVersion identifies the digest manifest format. Bumped if
// the digest.txt structure ever changes; verifiers reject unknown
// versions.
const digestVersion = "loom.export.v1"

// FileEntry is one row in digest.txt — a SHA-256 over a file's full
// bytes plus its size, used to detect any post-export tampering.
type FileEntry struct {
	Name   string // relative path inside the bundle (e.g. "manifest.json")
	SHA256 string // 64-char lowercase hex
	Bytes  int64
}

// Result is the structured outcome of an export run.
type Result struct {
	RunID       string
	RunDir      string
	OutPath     string
	Signed      bool
	PubkeyHash  string // sha256 of the pubkey bytes, for auditor cross-check
	Files       []FileEntry
}

// Run produces an export bundle for runDir. If keyPath is non-empty,
// the digest manifest is signed with Ed25519 using the loaded key,
// and pubkey.pem + digest.sig are added to the bundle. If keyPath is
// empty, the export is unsigned (still tamper-evident via digest.txt
// once the auditor receives the digest hash through a trusted
// channel, but signing is the recommended path).
//
// Run refuses to export a run whose own integrity is broken — it runs
// `loom verify` first and bails on anything other than exit 0.
func Run(runDir, outPath, keyPath string) (*Result, error) {
	m, err := runinfo.LoadManifest(runDir)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	// Refuse to export a broken run. The whole point of the export
	// bundle is that it's a clean handoff; if verify fails, fix the
	// run before exporting.
	res, code := verify.Run(runDir)
	if code != 0 {
		reason := "unknown"
		if res != nil && res.BrokenReason != "" {
			reason = res.BrokenReason
		}
		return nil, fmt.Errorf(
			"refusing to export: integrity check failed — %s "+
				"(run `loom verify %s` for details)", reason, m.RunID)
	}

	// Collect the file list. Order is deterministic so the digest is
	// reproducible across operators on the same artifact set.
	files, err := collectFiles(runDir)
	if err != nil {
		return nil, fmt.Errorf("collect files: %w", err)
	}

	// Build the digest manifest text. This is what gets signed.
	digestBytes := buildDigestText(m.RunID, files)

	// Optional Ed25519 signing.
	var sig []byte
	var pubPEM []byte
	var pubHash string
	if keyPath != "" {
		priv, pub, err := loadPrivateKey(keyPath)
		if err != nil {
			return nil, fmt.Errorf("load signing key %q: %w", keyPath, err)
		}
		sig = ed25519.Sign(priv, digestBytes)
		pubPEM, err = encodePublicKeyPEM(pub)
		if err != nil {
			return nil, fmt.Errorf("encode pubkey: %w", err)
		}
		h := sha256.Sum256(pub)
		pubHash = hex.EncodeToString(h[:])
	}

	// Resolve output path.
	if outPath == "" {
		outPath = filepath.Join(".", m.RunID+".tar.gz")
	}

	// Write the bundle.
	if err := writeTarball(outPath, runDir, files, digestBytes, sig, pubPEM); err != nil {
		return nil, fmt.Errorf("write tarball: %w", err)
	}

	return &Result{
		RunID:      m.RunID,
		RunDir:     runDir,
		OutPath:    outPath,
		Signed:     sig != nil,
		PubkeyHash: pubHash,
		Files:      files,
	}, nil
}

// collectFiles enumerates the run-dir artifacts that should land in
// the bundle. It walks the directory but skips hidden files, the
// previous report.html (regenerated below if missing — actually, no:
// we keep whatever's there — auditor wants the same artifact the
// operator inspected, not a re-render), and any *.tar.gz files (so
// re-exporting an already-exported dir doesn't recurse).
func collectFiles(runDir string) ([]FileEntry, error) {
	var out []FileEntry
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tar") {
			continue
		}
		// Reserved names produced by export itself; should never exist
		// in a regular run dir, but be defensive.
		if name == "digest.txt" || name == "digest.sig" || name == "pubkey.pem" {
			continue
		}

		full := filepath.Join(runDir, name)
		hash, size, err := hashFile(full)
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", name, err)
		}
		out = append(out, FileEntry{Name: name, SHA256: hash, Bytes: size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// buildDigestText produces the canonical bytes that get signed. Format:
//
//	loom.export.v1 <run_id>
//	<sha256>  <bytes>  <name>
//	<sha256>  <bytes>  <name>
//	...
//
// Two-space separator between fields, LF line ends, trailing newline.
// Stable across operators given the same files, so two independent
// exports of the same run produce byte-identical digest.txt.
func buildDigestText(runID string, files []FileEntry) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s %s\n", digestVersion, runID)
	for _, f := range files {
		fmt.Fprintf(&b, "%s  %d  %s\n", f.SHA256, f.Bytes, f.Name)
	}
	return b.Bytes()
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

// writeTarball assembles the gzipped tar at outPath.
func writeTarball(outPath, runDir string, files []FileEntry,
	digest, sig, pubPEM []byte) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Run dir artifacts.
	for _, fe := range files {
		full := filepath.Join(runDir, fe.Name)
		if err := writeTarFile(tw, fe.Name, full); err != nil {
			return fmt.Errorf("tar %s: %w", fe.Name, err)
		}
	}
	// digest.txt (always).
	if err := writeTarBytes(tw, "digest.txt", digest, 0644); err != nil {
		return err
	}
	// Optional signature + pubkey.
	if sig != nil {
		if err := writeTarBytes(tw, "digest.sig", sig, 0644); err != nil {
			return err
		}
	}
	if pubPEM != nil {
		if err := writeTarBytes(tw, "pubkey.pem", pubPEM, 0644); err != nil {
			return err
		}
	}
	return nil
}

func writeTarFile(tw *tar.Writer, nameInTar, srcPath string) error {
	st, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:    nameInTar,
		Mode:    int64(st.Mode().Perm()),
		Size:    st.Size(),
		ModTime: st.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(tw, src)
	return err
}

func writeTarBytes(tw *tar.Writer, name string, body []byte, mode os.FileMode) error {
	hdr := &tar.Header{
		Name: name,
		Mode: int64(mode),
		Size: int64(len(body)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(body)
	return err
}

// loadPrivateKey reads a PKCS#8-encoded Ed25519 private key from a PEM
// file at path. Returns the private key and its derived public key.
//
// Supported formats:
//   - PEM block "PRIVATE KEY" with PKCS#8 contents (the format
//     `loom keygen` produces, and `openssl genpkey -algorithm Ed25519`).
//   - Raw 64-byte Ed25519 seed||pubkey concatenation, no PEM (compat
//     with crude key-rotation scripts).
func loadPrivateKey(path string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	// Try PEM first.
	if block, _ := pem.Decode(raw); block != nil {
		if block.Type != "PRIVATE KEY" {
			return nil, nil, fmt.Errorf("unexpected PEM type %q (want PRIVATE KEY)", block.Type)
		}
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse PKCS#8: %w", err)
		}
		priv, ok := k.(ed25519.PrivateKey)
		if !ok {
			return nil, nil, fmt.Errorf("not an Ed25519 key (%T)", k)
		}
		return priv, priv.Public().(ed25519.PublicKey), nil
	}
	// Raw key bytes (64 bytes = seed || pubkey).
	if len(raw) == ed25519.PrivateKeySize {
		priv := ed25519.PrivateKey(raw)
		return priv, priv.Public().(ed25519.PublicKey), nil
	}
	return nil, nil, fmt.Errorf("unrecognized key format (size=%d bytes; expected PEM or %d raw bytes)",
		len(raw), ed25519.PrivateKeySize)
}

// encodePublicKeyPEM serializes an Ed25519 public key as a PKIX PEM block.
func encodePublicKeyPEM(pub ed25519.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// Cmd wires the `loom export` subcommand.
func Cmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "export <run-id | latest>",
		Short: "Bundle a run as a signed tar.gz for auditor handoff",
		Long: "Produces a self-contained tar.gz of the run dir plus a digest\n" +
			"manifest of every file's SHA-256. With --key (or LOOM_SIGN_KEY),\n" +
			"the digest is signed with Ed25519 and the public key is embedded\n" +
			"so an auditor with the operator's pubkey-fingerprint can verify\n" +
			"the bundle came from the claimed key and was not modified.\n\n" +
			"Refuses to export a run whose own integrity check fails (run\n" +
			"`loom verify` first if you want to inspect the failure).",
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
			out, _ := cmd.Flags().GetString("out")
			key, _ := cmd.Flags().GetString("key")
			if key == "" {
				key = os.Getenv("LOOM_SIGN_KEY")
			}
			res, err := Run(runDir, out, key)
			if err != nil {
				return err
			}
			renderResult(cmd, res)
			return nil
		},
	}
	c.Flags().String("out", "", "output path for the tar.gz (default: <run-id>.tar.gz in cwd)")
	c.Flags().String("key", "", "path to Ed25519 private key (PEM); falls back to $LOOM_SIGN_KEY")
	return c
}

func renderResult(cmd *cobra.Command, r *Result) {
	w := cmd.OutOrStdout()
	signedNote := ""
	if r.Signed {
		signedNote = fmt.Sprintf("  signed pubkey-sha256=%s", short(r.PubkeyHash))
	} else {
		signedNote = "  unsigned (no --key / LOOM_SIGN_KEY provided)"
	}
	fmt.Fprintf(w, "export: ✓ %s · %d files%s\n", r.OutPath, len(r.Files), signedNote)
	if r.Signed {
		fmt.Fprintf(w, "        share pubkey-sha256 with the auditor through a "+
			"trusted channel; they pass it to `loom verify --pubkey`.\n")
	}
}

func short(hex string) string {
	if len(hex) <= 12 {
		return hex
	}
	return hex[:8] + "…" + hex[len(hex)-4:]
}

// Suppress unused-import nag if a later refactor removes the bufio path.
var _ = bufio.ScanLines
