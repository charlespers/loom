// loom — M1 CLI. `loom run` and `loom version` are real; every other
// subcommand is a stub that exits non-zero with a deferral message.
package main

import (
	"fmt"
	"os"

	"github.com/charlespers/loom/cli/internal/run"
	"github.com/charlespers/loom/cli/internal/stub"
	"github.com/charlespers/loom/cli/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "loom",
		Short: "Observability harness for local-compute AI systems",
		Long: "Loom instruments local AI workloads. M1 ships skeleton " +
			"plumbing only; ring buffer and artifacts arrive in M2.",
		// Silence both Cobra's auto-printed usage and its auto-printed error;
		// main() owns error rendering so each error appears exactly once.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			version.Print(cmd.OutOrStdout())
		},
	})

	runCmd := &cobra.Command{
		Use:   "run -- <command> [args...]",
		Short: "Launch a command under the harness",
		Args:  cobra.MinimumNArgs(1),
		RunE:  run.RunE,
	}
	runCmd.Flags().Bool("quiet", false, "suppress the run-id banner")
	root.AddCommand(runCmd)

	root.AddCommand(&cobra.Command{Use: "watch", Short: "Live TUI", RunE: stub.NotImplementedYet("M4")})
	root.AddCommand(&cobra.Command{Use: "view", Short: "Static TUI", RunE: stub.NotImplementedYet("M4")})
	root.AddCommand(&cobra.Command{Use: "report", Short: "Render report", RunE: stub.NotImplementedYet("M5")})
	root.AddCommand(&cobra.Command{Use: "verify", Short: "Verify chain", RunE: stub.NotImplementedYet("M3")})
	root.AddCommand(&cobra.Command{Use: "redact", Short: "Re-run pipe", RunE: stub.NotImplementedYet("M3")})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Env diagnostic", RunE: stub.NotImplementedYet("M7")})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
