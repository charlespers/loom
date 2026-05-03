// Package show implements `loom show [<run-id> | latest]` — terse
// terminal summary of a single run. Reads manifest.json and renders
// a designed metadata block, the lifecycle outline, top spans, audit
// chain head, and a one-line pointer to summary.md / report.html.
//
// Compared to summary.md: this is the answer to "what just happened?"
// in 20 lines instead of 80.
package show

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charlespers/loom/cli/internal/runinfo"
	"github.com/charlespers/loom/cli/internal/ui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show [<run-id> | latest]",
		Short: "Terse terminal summary of a single run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			runDir, err := runinfo.ResolveRun(id)
			if err != nil {
				return err
			}
			out, err := Render(runDir)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	return c
}

// Render produces the terminal-formatted summary string.
func Render(runDir string) (string, error) {
	m, err := runinfo.LoadManifest(runDir)
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	// Cover line: short id + status badge + duration.
	header := ui.H1.Render("Run "+m.RunID) + "  " +
		ui.Badge(m.Status) + "  " +
		ui.Soft.Render(ui.HumanDurationMS(m.DurationMS))
	sb.WriteString(header + "\n\n")

	// Metadata KV block.
	cmdStr := readProcField(m.Process, "command")
	pid := readProcUint(m.Process, "pid")
	host := readProcField(m.Host, "hostname")
	osN := readProcField(m.Host, "os")
	arch := readProcField(m.Host, "arch")

	kvs := []ui.KV{
		{Label: "Started",  Value: humanTime(m.StartedAt)},
		{Label: "Ended",    Value: humanTime(m.EndedAt)},
		{Label: "Command",  Value: ui.Mono.Render(cmdStr)},
		{Label: "Host",     Value: ui.Mono.Render(host) + ui.Soft.Render(fmt.Sprintf(" · %s/%s · pid %d", osN, arch, pid))},
	}
	if m.Reproducibility != nil {
		if m.Reproducibility.ModelID != "" {
			kvs = append(kvs, ui.KV{Label: "Model", Value: ui.Mono.Render(m.Reproducibility.ModelID)})
		}
		if m.Reproducibility.ModelHash != "" {
			kvs = append(kvs, ui.KV{Label: "Weights", Value: ui.Mono.Render(ui.Short(m.Reproducibility.ModelHash))})
		}
		if m.Reproducibility.PromptVersion != "" {
			kvs = append(kvs, ui.KV{Label: "Prompt", Value: ui.Mono.Render(m.Reproducibility.PromptVersion)})
		}
		if m.Reproducibility.Tag != "" {
			kvs = append(kvs, ui.KV{Label: "Tag", Value: ui.Mono.Render(m.Reproducibility.Tag)})
		}
	}
	sb.WriteString(ui.RenderKV(kvs))
	sb.WriteString("\n\n")

	// Event counts: small inline bar.
	sb.WriteString(ui.Eyebrow.Render("EVENTS") + "\n")
	c := m.Counts
	parts := []string{
		colored("span", c.ByCategory["span"]),
		colored("metric", c.ByCategory["metric"]),
		colored("audit", c.ByCategory["audit"]),
		colored("lifecycle", c.ByCategory["lifecycle"]),
		colored("error", c.ByCategory["error"]),
	}
	sb.WriteString(strings.Join(parts, "   ") +
		ui.Soft.Render(fmt.Sprintf("   total %d", c.EventsTotal)) +
		"\n\n")

	// Top spans by total time, up to 5.
	if rows := topSpans(m, 5); len(rows) > 0 {
		sb.WriteString(ui.Eyebrow.Render("SPANS  by total time") + "\n")
		sb.WriteString(rows + "\n\n")
	}

	// Audit chain head.
	if m.AuditChain.Count > 0 {
		head := m.AuditChain.Head
		if head == "" {
			head = strings.Repeat("0", 64)
		}
		sb.WriteString(ui.Eyebrow.Render("AUDIT") + "\n")
		sb.WriteString(fmt.Sprintf("%s records · chain head %s\n",
			ui.Mono.Render(fmt.Sprintf("%d", m.AuditChain.Count)),
			ui.Mono.Render(ui.Short(head))))
		sb.WriteString(ui.Soft.Render("verify with `loom verify "+m.RunID+"`") + "\n\n")
	}

	// Pointers to the artifact files.
	sb.WriteString(ui.Eyebrow.Render("ARTIFACTS") + "\n")
	for _, f := range []string{"manifest.json", "events.jsonl", "audit.jsonl", "audit.public.jsonl", "summary.md", "report.html"} {
		path := filepath.Join(runDir, f)
		sb.WriteString("  " + ui.Mono.Render(f) + ui.Mute.Render(" → ") + ui.Soft.Render(path) + "\n")
	}

	return sb.String(), nil
}

func colored(cat string, n uint64) string {
	if n == 0 {
		return ui.Mute.Render(fmt.Sprintf("%s 0", cat))
	}
	return ui.CatTag(cat) + ui.Mono.Render(fmt.Sprintf(" %d", n))
}

func topSpans(m *runinfo.Manifest, limit int) string {
	if m.Spans == nil {
		return ""
	}
	var wrapper struct {
		ByName map[string]struct {
			Count   uint64 `json:"count"`
			TotalNS uint64 `json:"total_ns"`
			P50NS   uint64 `json:"p50_ns"`
			P95NS   uint64 `json:"p95_ns"`
			P99NS   uint64 `json:"p99_ns"`
			MaxNS   uint64 `json:"max_ns"`
		} `json:"by_name"`
	}
	if err := json.Unmarshal(m.Spans, &wrapper); err != nil {
		return ""
	}
	type row struct {
		name             string
		count            uint64
		total, p50, p99  uint64
	}
	rows := make([]row, 0, len(wrapper.ByName))
	for name, st := range wrapper.ByName {
		rows = append(rows, row{name, st.Count, st.TotalNS, st.P50NS, st.P99NS})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].total > rows[j].total })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{
			r.name,
			fmt.Sprintf("%d", r.count),
			formatNS(r.total),
			formatNS(r.p50),
			formatNS(r.p99),
		})
	}
	t := ui.Table{
		Columns: []ui.Column{
			{Title: "name"},
			{Title: "count", Align: "right"},
			{Title: "total", Align: "right"},
			{Title: "p50",   Align: "right"},
			{Title: "p99",   Align: "right"},
		},
		Rows: tableRows,
	}
	return t.Render()
}

func formatNS(ns uint64) string {
	switch {
	case ns < 1000:
		return fmt.Sprintf("%d ns", ns)
	case ns < 1_000_000:
		return fmt.Sprintf("%.2f µs", float64(ns)/1e3)
	case ns < 1_000_000_000:
		return fmt.Sprintf("%.2f ms", float64(ns)/1e6)
	default:
		return fmt.Sprintf("%.2f s", float64(ns)/1e9)
	}
}

func readProcField(raw json.RawMessage, key string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	_ = json.Unmarshal(v, &s)
	return s
}
func readProcUint(raw json.RawMessage, key string) uint64 {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	var n uint64
	_ = json.Unmarshal(v, &n)
	return n
}

func humanTime(iso string) string {
	if iso == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return iso
	}
	return t.UTC().Format("Jan 2, 2006 · 15:04:05 UTC")
}
