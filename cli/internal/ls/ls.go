// Package ls implements `loom ls` — a tabular view of recent runs.
//
// Operator UX target: the answer to "what runs do I have?" must be one
// command, two seconds, one screenful. No flags required for the
// 90-percent case; --limit and --json for the long-tail.
package ls

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charlespers/loom/cli/internal/runinfo"
	"github.com/charlespers/loom/cli/internal/ui"
	"github.com/spf13/cobra"
)

// Cmd returns the cobra subcommand.
func Cmd() *cobra.Command {
	var limit int
	var asJSON bool
	c := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List recent runs in $LOOM_HOME/runs",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runs, err := runinfo.ListRuns(limit)
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd, runs)
			}
			if len(runs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(),
					ui.Mute.Render("no runs yet — try `loom run -- <command>`"))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), renderTable(runs))
			return nil
		},
	}
	c.Flags().IntVar(&limit, "limit", 20, "max runs to show (0 = all)")
	c.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return c
}

func renderTable(runs []runinfo.RunSummary) string {
	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, []string{
			shortID(r.RunID),
			styleStatus(r.Status),
			ageOf(r.StartedAt),
			ui.HumanDurationMS(r.DurationMS),
			ui.HumanCount(r.EventsTotal),
			ui.HumanCount(r.AuditCount),
			styleErrCount(r.ErrorCount),
			truncCmd(r.Command, 40),
		})
	}

	t := ui.Table{
		Columns: []ui.Column{
			{Title: "id"},
			{Title: "status"},
			{Title: "age",     Align: "right"},
			{Title: "duration",Align: "right"},
			{Title: "events",  Align: "right"},
			{Title: "audit",   Align: "right"},
			{Title: "errors",  Align: "right"},
			{Title: "command", Max: 40},
		},
		Rows: rows,
	}
	return t.Render() + "\n\n" + ui.Mute.Render(
		fmt.Sprintf("%d run%s · sorted by recency · `loom show <id>` for detail",
			len(runs), plural(len(runs))))
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "…" + id[len(id)-3:]
}

func styleStatus(s string) string {
	switch s {
	case "completed", "ok":
		return ui.OK("ok")
	case "incomplete":
		return ui.Warn("incomplete")
	case "warn":
		return ui.Warn("warn")
	default:
		return ui.Fail(s)
	}
}

func styleErrCount(n uint64) string {
	if n == 0 {
		return ui.Mute.Render("0")
	}
	return ui.Fail(fmt.Sprintf("%d", n))
}

func ageOf(iso string) string {
	if iso == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

func truncCmd(cmd string, max int) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ui.Mute.Render("(unknown)")
	}
	// If it's a path, prefer the basename + args.
	parts := strings.SplitN(cmd, " ", 2)
	parts[0] = filepath.Base(parts[0])
	cmd = strings.Join(parts, " ")
	if len([]rune(cmd)) > max {
		return string([]rune(cmd)[:max-1]) + "…"
	}
	return cmd
}

func writeJSON(cmd *cobra.Command, runs []runinfo.RunSummary) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(runs)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
