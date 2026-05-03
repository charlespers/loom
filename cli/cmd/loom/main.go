// loom — observability harness CLI.
//
// Operator surface:
//   loom              shorthand for `loom ls` — table of recent runs
//   loom ls           same, explicit form
//   loom show <id>    terse terminal summary of one run
//   loom run -- ...   launch a command under the harness
//   loom verify <id>  walk the audit hash chain
//   loom report <id>  render report.html
//   loom version      version + wire schema
package main

import (
	"fmt"
	"os"

	"github.com/charlespers/loom/cli/internal/doctor"
	"github.com/charlespers/loom/cli/internal/ls"
	"github.com/charlespers/loom/cli/internal/report"
	"github.com/charlespers/loom/cli/internal/run"
	"github.com/charlespers/loom/cli/internal/show"
	"github.com/charlespers/loom/cli/internal/stub"
	"github.com/charlespers/loom/cli/internal/verify"
	"github.com/charlespers/loom/cli/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "loom",
		Short: "Observability harness for local-compute AI systems",
		Long: "Loom instruments local AI workloads. Each run produces a self-\n" +
			"describing artifact bundle: events.jsonl, audit.jsonl + audit.public.\n" +
			"jsonl (hash-chained), manifest.json, summary.md, and report.html.\n\n" +
			"Run with no args: list recent runs.",
		SilenceUsage:  true,
		SilenceErrors: true,
		// When invoked as plain `loom`, default to `loom ls`. Cobra's
		// PersistentPreRun fires before subcommand resolution so we use
		// RunE on the root for this.
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cobra calls this only when no subcommand was matched.
			return ls.Cmd().Execute()
		},
	}

	root.AddCommand(ls.Cmd())
	root.AddCommand(show.Cmd())
	root.AddCommand(verify.Cmd())
	root.AddCommand(report.Cmd())

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

	// Future-milestone subcommands keep the deferral message so the
	// command surface is honest about what's there.
	root.AddCommand(doctor.Cmd())

	root.AddCommand(&cobra.Command{Use: "watch",  Short: "Live TUI",         RunE: stub.NotImplementedYet("M4")})
	root.AddCommand(&cobra.Command{Use: "view",   Short: "Static TUI",       RunE: stub.NotImplementedYet("M4")})
	root.AddCommand(&cobra.Command{Use: "redact", Short: "Re-run redactor",  RunE: stub.NotImplementedYet("M5")})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
