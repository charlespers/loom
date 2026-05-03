// keygen.go — `loom keygen` subcommand. Generates an Ed25519 keypair
// for signing export bundles. Emits two files:
//
//	<out>      private key, PKCS#8 PEM, mode 0600
//	<out>.pub  public key,  PKIX PEM,   mode 0644
//
// The pubkey-sha256 fingerprint is printed to stdout — the operator
// should record this fingerprint somewhere durable (config repo,
// password manager, signed git tag) so any future auditor can confirm
// "this is the pubkey we expect."

package export

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// KeygenCmd wires the `loom keygen` subcommand.
func KeygenCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "keygen --out <key-path>",
		Short: "Generate an Ed25519 keypair for signing exports",
		Long: "Writes <out> (private key, PKCS#8 PEM, mode 0600) and <out>.pub\n" +
			"(public key, PKIX PEM, mode 0644). Prints the pubkey-sha256\n" +
			"fingerprint — record this somewhere durable; auditors will use\n" +
			"it to confirm a future export bundle came from this key.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, _ := cmd.Flags().GetString("out")
			if out == "" {
				return fmt.Errorf("--out is required (e.g. --out ./loom-sign.key)")
			}
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return fmt.Errorf("generate key: %w", err)
			}

			privDER, err := x509.MarshalPKCS8PrivateKey(priv)
			if err != nil {
				return fmt.Errorf("marshal private: %w", err)
			}
			privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

			pubDER, err := x509.MarshalPKIXPublicKey(pub)
			if err != nil {
				return fmt.Errorf("marshal public: %w", err)
			}
			pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

			if _, err := os.Stat(out); err == nil {
				return fmt.Errorf("refusing to overwrite existing %s", out)
			}
			if err := os.WriteFile(out, privPEM, 0600); err != nil {
				return fmt.Errorf("write %s: %w", out, err)
			}
			pubPath := out + ".pub"
			if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
				return fmt.Errorf("write %s: %w", pubPath, err)
			}

			fp := sha256.Sum256(pub)
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "keygen: ✓ wrote %s (private, 0600) + %s (public)\n", out, pubPath)
			fmt.Fprintf(w, "        pubkey-sha256: %s\n", hex.EncodeToString(fp[:]))
			fmt.Fprintf(w, "        record the fingerprint above; auditors pass it as\n")
			fmt.Fprintf(w, "        `loom verify --pubkey %s` (or via fingerprint).\n", pubPath)
			return nil
		},
	}
	c.Flags().String("out", "", "output path for the private key (the public key goes to <out>.pub)")
	return c
}
