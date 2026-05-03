// Package report implements `loom report <run-id>` — renders a
// single-file, self-contained HTML report from a finished run's
// artifact directory. The HTML embeds the run's events as JSON inside
// a <script> tag and ships ~5 KB of vanilla JS so an auditor can open
// the file offline, filter the event stream, and re-verify the audit
// hash chain in their browser without trusting our server.
package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charlespers/loom/cli/internal/runinfo"
	"github.com/spf13/cobra"
)

// ReportData is the shape passed to the HTML template. It's a
// flattened, presentation-ready view of the run; expensive joins
// happen here so the template stays declarative.
type ReportData struct {
	RunID         string
	LoomVersion   string
	WireSchema    string
	Status        string
	StatusBadge   string // "ok", "warn", or "error" — drives CSS color
	StartedAt     string // ISO-8601, UTC
	StartedHuman  string // "May 2, 2026 22:28:41 UTC"
	EndedAt       string
	EndedHuman    string
	Duration      string // "8.76 s" or "1.24 ms" etc.
	Command       string
	PID           uint64
	Hostname      string
	OS            string
	Arch          string
	Kernel        string
	Cwd           string
	GeneratedAt   string

	Counts        Counts
	Lifecycles    []LifecycleEntry
	Spans         []SpanRow
	Metrics       []MetricRow
	AuditRecords  []AuditRow
	AuditHead     string
	AuditCount    uint64
	Errors        []ErrorRow
	EventsJSON    htmltemplate.JS // embedded as one long line for browser parsing
	AuditJSON     htmltemplate.JS // embedded for in-browser verifier
}

type Counts struct {
	Total     uint64
	Span      uint64
	Metric    uint64
	Audit     uint64
	Lifecycle uint64
	Error     uint64
}

type LifecycleEntry struct {
	Anchor string
	Time   string // HH:MM:SS.mmm
	Marker string
}

type SpanRow struct {
	Name    string
	Count   uint64
	Total   string // "8.76 s"
	Mean    string
	P50     string
	P95     string
	P99     string
	Max     string
	BarPct  float64 // 0..100, of total run
}

type MetricRow struct {
	Name      string
	Kind      string  // i64, f64, counter
	Last      string  // last sampled value
	Min       string
	Max       string
	Mean      string
	Count     uint64
	Sparkline string  // inline SVG path data
}

type AuditRow struct {
	Time     string
	Seq      uint64
	Name     string
	Prev     string // 8-char short hash
	This     string
	AttrsRaw string // JSON object literal for display
}

type ErrorRow struct {
	Time     string
	Severity string
	Name     string
	Message  string
	AttrsRaw string
}

// Build reads the run directory and constructs a ReportData ready for
// the template.
func Build(runDir string) (*ReportData, error) {
	m, err := runinfo.LoadManifest(runDir)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	events, err := readEvents(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}

	rd := &ReportData{
		RunID:        m.RunID,
		LoomVersion:  m.LoomVersion,
		WireSchema:   m.WireSchema,
		Status:       m.Status,
		StatusBadge:  statusToBadge(m.Status),
		StartedAt:    m.StartedAt,
		StartedHuman: humanTime(m.StartedAt),
		EndedAt:      m.EndedAt,
		EndedHuman:   humanTime(m.EndedAt),
		Duration:     formatDurationMS(m.DurationMS),
		Command:      readProcessField(m.Process, "command"),
		PID:          readProcessUint(m.Process, "pid"),
		Hostname:     readHostField(m.Host, "hostname"),
		OS:           readHostField(m.Host, "os"),
		Arch:         readHostField(m.Host, "arch"),
		Kernel:       readHostField(m.Host, "kernel_release"),
		Cwd:          readProcessField(m.Process, "cwd"),
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Counts: Counts{
			Total:     m.Counts.EventsTotal,
			Span:      m.Counts.ByCategory["span"],
			Metric:    m.Counts.ByCategory["metric"],
			Audit:     m.Counts.ByCategory["audit"],
			Lifecycle: m.Counts.ByCategory["lifecycle"],
			Error:     m.Counts.ByCategory["error"],
		},
		AuditHead:  m.AuditChain.Head,
		AuditCount: m.AuditChain.Count,
	}

	rd.Lifecycles = collectLifecycles(events)
	rd.Spans = collectSpans(events, m.DurationMS)
	rd.Metrics = collectMetrics(events)
	rd.Errors = collectErrors(events)

	// Read the canonical audit file (private; we still embed it because
	// the report.html lives in the same directory and inherits the same
	// trust boundary as audit.jsonl itself).
	//
	// scriptSafe replaces "</" with "<\\/" so an audit record whose
	// content contains the seven-byte sequence "</script" cannot break
	// out of the JSON island. "\/" is a valid JSON escape for "/", so
	// the bytes remain semantically identical when JSON.parse'd.
	auditPath := filepath.Join(runDir, "audit.jsonl")
	if auditBytes, err := os.ReadFile(auditPath); err == nil {
		rd.AuditRecords = collectAuditRows(auditBytes)
		rd.AuditJSON = htmltemplate.JS(scriptSafe(string(auditBytes)))
	}

	// Embed events.jsonl for the in-browser filter UI. We minify by
	// stripping intra-line whitespace per record (none should exist
	// already — emitter emits dense JSON).
	rd.EventsJSON = htmltemplate.JS(scriptSafe(serializeEvents(events)))
	return rd, nil
}

// Render writes report.html into runDir and returns the path written.
func Render(runDir string) (string, error) {
	rd, err := Build(runDir)
	if err != nil {
		return "", err
	}
	out := filepath.Join(runDir, "report.html")
	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer f.Close()

	tmpl, err := htmltemplate.New("report").Parse(reportTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	if err := tmpl.Execute(f, rd); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────────────
// Internal — event loading + transformations
// ─────────────────────────────────────────────────────────────────────

type rawEvent struct {
	V        string          `json:"v"`
	Cat      string          `json:"cat"`
	Seq      uint64          `json:"seq"`
	Ts       string          `json:"ts"`
	Name     string          `json:"name"`
	Attrs    json.RawMessage `json:"attrs"`
	SpanID   uint64          `json:"span_id,omitempty"`
	Parent   uint64          `json:"parent,omitempty"`
	DurNS    uint64          `json:"dur_ns,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
	Kind     string          `json:"kind,omitempty"`
	Message  string          `json:"message,omitempty"`
	Severity string          `json:"severity,omitempty"`
	rawLine  []byte
}

func readEvents(path string) ([]rawEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	var out []rawEvent
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e rawEvent
		if err := json.Unmarshal(line, &e); err != nil {
			continue // tolerate; tamper-evidence is the audit chain's job
		}
		e.rawLine = append([]byte(nil), line...)
		out = append(out, e)
	}
	return out, scanner.Err()
}

func serializeEvents(events []rawEvent) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, e := range events {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(e.rawLine)
	}
	b.WriteByte(']')
	return b.String()
}

// scriptSafe replaces "</" with "<\\/" so user-controlled JSON content
// embedded in a <script> block cannot break out of the JSON island via
// "</script>". "\/" is a valid JSON escape for "/" (RFC 8259 § 7), so
// the JSON parser yields identical strings either way. Defends against
// an audit record whose name or attribute contains "</script".
func scriptSafe(s string) string {
	return strings.ReplaceAll(s, "</", `<\/`)
}

func collectLifecycles(events []rawEvent) []LifecycleEntry {
	var out []LifecycleEntry
	for _, e := range events {
		if e.Cat != "lifecycle" {
			continue
		}
		out = append(out, LifecycleEntry{
			Anchor: anchorize(e.Name) + "-" + fmt.Sprint(e.Seq),
			Time:   shortTimeOf(e.Ts),
			Marker: e.Name,
		})
	}
	return out
}

func collectSpans(events []rawEvent, runDurationMS uint64) []SpanRow {
	type stat struct {
		count    uint64
		total    uint64
		min, max uint64
		samples  []uint64
	}
	stats := map[string]*stat{}
	for _, e := range events {
		if e.Cat != "span" || e.DurNS == 0 {
			continue
		}
		s, ok := stats[e.Name]
		if !ok {
			s = &stat{min: ^uint64(0)}
			stats[e.Name] = s
		}
		s.count++
		s.total += e.DurNS
		if e.DurNS < s.min {
			s.min = e.DurNS
		}
		if e.DurNS > s.max {
			s.max = e.DurNS
		}
		s.samples = append(s.samples, e.DurNS)
	}
	rows := make([]SpanRow, 0, len(stats))
	runTotalNS := runDurationMS * 1_000_000
	for name, s := range stats {
		mean := uint64(0)
		if s.count > 0 {
			mean = s.total / s.count
		}
		bar := 0.0
		if runTotalNS > 0 {
			bar = float64(s.total) / float64(runTotalNS) * 100
			if bar > 100 {
				bar = 100
			}
		}
		rows = append(rows, SpanRow{
			Name:   name,
			Count:  s.count,
			Total:  formatDurNS(s.total),
			Mean:   formatDurNS(mean),
			P50:    formatDurNS(percentile(s.samples, 50)),
			P95:    formatDurNS(percentile(s.samples, 95)),
			P99:    formatDurNS(percentile(s.samples, 99)),
			Max:    formatDurNS(s.max),
			BarPct: bar,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return durNSFromString(rows[i].Total) > durNSFromString(rows[j].Total)
	})
	return rows
}

func collectMetrics(events []rawEvent) []MetricRow {
	type bucket struct {
		kind   string
		values []float64
	}
	buckets := map[string]*bucket{}
	for _, e := range events {
		if e.Cat != "metric" {
			continue
		}
		b, ok := buckets[e.Name]
		if !ok {
			b = &bucket{kind: e.Kind}
			buckets[e.Name] = b
		}
		var v float64
		if err := json.Unmarshal(e.Value, &v); err == nil {
			b.values = append(b.values, v)
		}
	}
	out := make([]MetricRow, 0, len(buckets))
	for name, b := range buckets {
		if len(b.values) == 0 {
			continue
		}
		min, max, sum := b.values[0], b.values[0], 0.0
		for _, v := range b.values {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
			sum += v
		}
		mean := sum / float64(len(b.values))
		out = append(out, MetricRow{
			Name:      name,
			Kind:      b.kind,
			Last:      formatNum(b.values[len(b.values)-1]),
			Min:       formatNum(min),
			Max:       formatNum(max),
			Mean:      formatNum(mean),
			Count:     uint64(len(b.values)),
			Sparkline: sparklinePath(b.values),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

func collectErrors(events []rawEvent) []ErrorRow {
	var out []ErrorRow
	for _, e := range events {
		if e.Cat != "error" {
			continue
		}
		out = append(out, ErrorRow{
			Time:     shortTimeOf(e.Ts),
			Severity: e.Severity,
			Name:     e.Name,
			Message:  e.Message,
			AttrsRaw: string(e.Attrs),
		})
	}
	return out
}

func collectAuditRows(b []byte) []AuditRow {
	var rows []AuditRow
	for _, line := range bytesSplitLines(b) {
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Seq   uint64          `json:"seq"`
			Ts    string          `json:"ts"`
			Name  string          `json:"name"`
			Attrs json.RawMessage `json:"attrs"`
			Chain struct {
				Prev string `json:"prev"`
				This string `json:"this"`
			} `json:"chain"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		rows = append(rows, AuditRow{
			Time:     shortTimeOf(rec.Ts),
			Seq:      rec.Seq,
			Name:     rec.Name,
			Prev:     shortHash(rec.Chain.Prev),
			This:     shortHash(rec.Chain.This),
			AttrsRaw: string(rec.Attrs),
		})
	}
	return rows
}

// ─────────────────────────────────────────────────────────────────────
// Formatting helpers
// ─────────────────────────────────────────────────────────────────────

func formatDurationMS(ms uint64) string {
	if ms == 0 {
		return "<1 ms"
	}
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.2f s", float64(ms)/1000)
}

func formatDurNS(ns uint64) string {
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

func formatNum(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.4g", v)
}

func percentile(samples []uint64, pct int) uint64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]uint64(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := pct * (len(sorted) - 1) / 100
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func sparklinePath(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	const w, h = 120.0, 28.0
	lo, hi := values[0], values[0]
	for _, v := range values {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	span := hi - lo
	if span == 0 {
		span = 1
	}
	denom := len(values) - 1
	if denom < 1 {
		denom = 1
	}
	var b strings.Builder
	for i, v := range values {
		x := w * float64(i) / float64(denom)
		y := h - h*((v-lo)/span)
		if i == 0 {
			fmt.Fprintf(&b, "M %.2f,%.2f", x, y)
		} else {
			fmt.Fprintf(&b, " L %.2f,%.2f", x, y)
		}
	}
	return b.String()
}

func anchorize(s string) string {
	out := strings.ToLower(s)
	out = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, out)
	return strings.Trim(out, "-")
}

func shortTimeOf(iso string) string {
	if t, err := time.Parse(time.RFC3339Nano, iso); err == nil {
		return t.UTC().Format("15:04:05.000")
	}
	if len(iso) >= 23 {
		return iso[11:23]
	}
	return iso
}

func humanTime(iso string) string {
	if t, err := time.Parse(time.RFC3339Nano, iso); err == nil {
		return t.UTC().Format("Jan 2, 2006 · 15:04:05 UTC")
	}
	return iso
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:8] + "…" + h[len(h)-4:]
}

func bytesSplitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

func statusToBadge(status string) string {
	switch status {
	case "completed", "ok":
		return "ok"
	case "warn":
		return "warn"
	case "failed", "crashed", "error":
		return "error"
	default:
		return "ok"
	}
}

func readProcessField(raw json.RawMessage, key string) string {
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

func readProcessUint(raw json.RawMessage, key string) uint64 {
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

func readHostField(raw json.RawMessage, key string) string {
	return readProcessField(raw, key)
}

// durNSFromString is the inverse of formatDurNS, used only for sorting
// SpanRows back into ns order after they've been pretty-printed. Not
// precise — but rows are equal-precision so the comparison is monotone.
func durNSFromString(s string) uint64 {
	var v float64
	var unit string
	fmt.Sscanf(s, "%f %s", &v, &unit)
	switch unit {
	case "ns":
		return uint64(v)
	case "µs":
		return uint64(v * 1e3)
	case "ms":
		return uint64(v * 1e6)
	case "s":
		return uint64(v * 1e9)
	}
	return 0
}

// ─────────────────────────────────────────────────────────────────────
// CLI wiring
// ─────────────────────────────────────────────────────────────────────

func Cmd() *cobra.Command {
	var open bool
	c := &cobra.Command{
		Use:   "report [<run-id> | latest]",
		Short: "Render a single-file report.html for a finished run",
		Long: "Reads manifest.json + events.jsonl + audit.jsonl from the run\n" +
			"directory and writes report.html alongside them. The HTML is\n" +
			"self-contained: open it offline, filter the event stream, and\n" +
			"verify the audit hash chain in your browser.",
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
			out, err := Render(runDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "report: ✓ %s\n", out)
			if open {
				openInBrowser(out)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&open, "open", false, "open the rendered report in the default browser")
	return c
}

func openInBrowser(path string) {
	// Best-effort, platform-specific; failures are silent.
	candidates := [][]string{
		{"open", path},        // macOS
		{"xdg-open", path},    // Linux
		{"cmd", "/c", "start", path}, // Windows (rare for loom but harmless)
	}
	for _, c := range candidates {
		if err := runSilent(c[0], c[1:]...); err == nil {
			return
		}
	}
}
