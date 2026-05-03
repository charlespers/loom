// Package doctor implements `loom doctor` — environment diagnostic.
// Answers "is loom set up correctly on this box?" with a checklist
// the operator can read at a glance. Each check is one line: status
// glyph, name, current value, brief note.
//
// Exit code 0 if every required check passes. Non-zero if a required
// check fails; warnings don't fail the run.
package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charlespers/loom/cli/internal/runinfo"
	"github.com/charlespers/loom/cli/internal/ui"
	"github.com/charlespers/loom/cli/internal/version"
	"github.com/spf13/cobra"
)

type level int

const (
	pass level = iota
	warn
	fail
)

type check struct {
	level    level
	name     string
	value    string
	note     string
	required bool
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the local Loom environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := []check{
				cliVersionCheck(),
				wireSchemaCheck(),
				goRuntimeCheck(),
				loomHomeCheck(),
				runsDirCheck(),
				latestRunCheck(),
				colorTerminalCheck(),
				readmeCheck(),
			}

			fmt.Fprintln(cmd.OutOrStdout(), ui.H1.Render("loom doctor"))
			fmt.Fprintln(cmd.OutOrStdout())

			anyFail := false
			for _, c := range checks {
				renderCheck(cmd, c)
				if c.level == fail && c.required {
					anyFail = true
				}
			}

			fmt.Fprintln(cmd.OutOrStdout())
			if anyFail {
				fmt.Fprintln(cmd.OutOrStdout(), ui.Fail("one or more required checks failed"))
				os.Exit(2)
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.OK("environment ready"))
			return nil
		},
	}
}

func renderCheck(cmd *cobra.Command, c check) {
	w := cmd.OutOrStdout()
	var icon string
	switch c.level {
	case pass:
		icon = ui.OK("")
	case warn:
		icon = ui.Warn("")
	case fail:
		icon = ui.Fail("")
	}
	name := ui.Mono.Render(fmt.Sprintf("%-22s", c.name))
	value := ui.Soft.Render(c.value)
	out := fmt.Sprintf("  %s %s %s", icon, name, value)
	if c.note != "" {
		out += "  " + ui.Mute.Render("· "+c.note)
	}
	fmt.Fprintln(w, out)
}

// ── individual checks ────────────────────────────────────────────────────

func cliVersionCheck() check {
	return check{
		level:    pass,
		name:     "cli version",
		value:    "v" + version.CLIVersion,
		required: true,
	}
}

func wireSchemaCheck() check {
	return check{
		level:    pass,
		name:     "wire schema",
		value:    version.WireSchema,
		required: true,
	}
}

func goRuntimeCheck() check {
	return check{
		level: pass,
		name:  "go runtime",
		value: fmt.Sprintf("%s on %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH),
	}
}

func loomHomeCheck() check {
	home, err := runinfo.LoomHome()
	if err != nil {
		return check{level: fail, name: "loom home", value: "(error)", note: err.Error(), required: true}
	}
	src := "default"
	if v := os.Getenv("LOOM_HOME"); v != "" {
		src = "$LOOM_HOME"
	}
	if _, err := os.Stat(home); err != nil {
		if os.IsNotExist(err) {
			return check{level: warn, name: "loom home", value: home, note: "not yet created (will be on first run); from " + src}
		}
		return check{level: fail, name: "loom home", value: home, note: err.Error(), required: true}
	}
	return check{level: pass, name: "loom home", value: home, note: "from " + src}
}

func runsDirCheck() check {
	home, err := runinfo.LoomHome()
	if err != nil {
		return check{level: fail, name: "runs dir", value: "(error)", required: true}
	}
	dir := filepath.Join(home, "runs")
	st, err := os.Stat(dir)
	if err != nil {
		return check{level: warn, name: "runs dir", value: dir, note: "not yet created"}
	}
	if !st.IsDir() {
		return check{level: fail, name: "runs dir", value: dir, note: "exists but not a directory", required: true}
	}
	// Try a probe write to confirm writability.
	probe, err := os.CreateTemp(dir, ".loom-doctor-*")
	if err != nil {
		return check{level: fail, name: "runs dir", value: dir, note: "not writable: " + err.Error(), required: true}
	}
	probe.Close()
	os.Remove(probe.Name())
	return check{level: pass, name: "runs dir", value: dir, note: "writable"}
}

func latestRunCheck() check {
	runs, err := runinfo.ListRuns(1)
	if err != nil || len(runs) == 0 {
		return check{level: warn, name: "latest run", value: "(none)", note: "try `loom run -- echo hi`"}
	}
	r := runs[0]
	val := r.RunID[:8] + "…" + r.RunID[len(r.RunID)-3:]
	return check{level: pass, name: "latest run", value: val, note: r.Status + ", " + ui.HumanDurationMS(r.DurationMS)}
}

func colorTerminalCheck() check {
	if os.Getenv("NO_COLOR") != "" {
		return check{level: warn, name: "terminal color", value: "disabled", note: "$NO_COLOR set"}
	}
	term := os.Getenv("TERM")
	colorTerm := os.Getenv("COLORTERM")
	if strings.Contains(strings.ToLower(colorTerm), "truecolor") || strings.Contains(colorTerm, "24bit") {
		return check{level: pass, name: "terminal color", value: "truecolor", note: term}
	}
	if term != "" {
		return check{level: pass, name: "terminal color", value: "256-color", note: term + " (truecolor would be richer)"}
	}
	return check{level: warn, name: "terminal color", value: "unknown", note: "$TERM not set"}
}

func readmeCheck() check {
	// Best-effort: the README lives next to the binary in source builds;
	// we don't need it but the hint helps new users.
	if _, err := os.Stat("README.md"); err == nil {
		return check{level: pass, name: "docs", value: "README.md found"}
	}
	return check{level: pass, name: "docs", value: "see github.com/charlespers/loom"}
}
