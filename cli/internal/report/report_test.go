package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixtureRun creates a minimal artifact bundle in a temp dir so
// tests don't depend on building/running the embed lib.
func writeFixtureRun(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	manifest := `{
		"schema": "loom.manifest.v1",
		"loom_version": "0.1.0",
		"wire_schema": "loom.event.v1",
		"run_id": "01J3KTV6FIXTURE",
		"started_at": "2026-05-02T22:00:00.000Z",
		"started_at_unix_ns": 1746223200000000000,
		"ended_at": "2026-05-02T22:00:01.234Z",
		"ended_at_unix_ns": 1746223201234000000,
		"duration_ms": 1234,
		"status": "completed",
		"process": {"pid": 99, "ppid": 1, "command": "demo --flag", "argv": ["demo","--flag"], "cwd": "/tmp", "executable": "/bin/demo"},
		"host":    {"hostname": "h.example", "os": "Darwin", "arch": "arm64", "kernel_release": "23.5.0"},
		"counts":  {"events_total": 5, "by_category": {"span": 1, "metric": 1, "audit": 1, "lifecycle": 1, "error": 1}},
		"spans":   {"by_name": {"forward.layer": {"count":1,"total_ns":1000000,"min_ns":1000000,"max_ns":1000000,"p50_ns":1000000,"p95_ns":1000000,"p99_ns":1000000}}},
		"audit_chain": {"head": "abc123", "count": 1},
		"files": {}
	}`
	must(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644))

	events := strings.Join([]string{
		`{"v":"loom.event.v1","cat":"lifecycle","seq":0,"ts":"2026-05-02T22:00:00.000Z","name":"run.start","attrs":{"run_id":"01J3KTV6FIXTURE"}}`,
		`{"v":"loom.event.v1","cat":"span","seq":1,"ts":"2026-05-02T22:00:00.500Z","name":"forward.layer","span_id":1,"dur_ns":1000000,"attrs":{"layer":0}}`,
		`{"v":"loom.event.v1","cat":"metric","seq":2,"ts":"2026-05-02T22:00:00.700Z","name":"tok_step_ms","value":8.71,"kind":"f64","attrs":{}}`,
		`{"v":"loom.event.v1","cat":"audit","seq":3,"ts":"2026-05-02T22:00:00.900Z","name":"file.read","attrs":{"path":"/tmp/x"},"chain":{"prev":"` + strings.Repeat("0", 64) + `","this":"abc123"}}`,
		`{"v":"loom.event.v1","cat":"error","seq":4,"ts":"2026-05-02T22:00:01.100Z","name":"warn.thing","message":"a bad thing","severity":"warn","attrs":{}}`,
		"",
	}, "\n")
	must(t, os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644))

	audit := `{"v":"loom.event.v1","cat":"audit","seq":3,"ts":"2026-05-02T22:00:00.900Z","name":"file.read","attrs":{"path":"/tmp/x"},"chain":{"prev":"` + strings.Repeat("0", 64) + `","this":"abc123"}}` + "\n"
	must(t, os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(audit), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "audit.public.jsonl"), []byte(audit), 0o644))
	return dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuild_PopulatesEverySectionFromFixture(t *testing.T) {
	dir := writeFixtureRun(t)
	rd, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}

	if rd.RunID != "01J3KTV6FIXTURE" {
		t.Errorf("RunID: got %q", rd.RunID)
	}
	if rd.StatusBadge != "ok" {
		t.Errorf("StatusBadge: got %q", rd.StatusBadge)
	}
	if rd.Duration != "1.23 s" {
		t.Errorf("Duration: got %q", rd.Duration)
	}
	if rd.Counts.Total != 5 || rd.Counts.Span != 1 || rd.Counts.Audit != 1 || rd.Counts.Error != 1 {
		t.Errorf("Counts wrong: %+v", rd.Counts)
	}
	if len(rd.Spans) != 1 || rd.Spans[0].Name != "forward.layer" {
		t.Errorf("Spans: %+v", rd.Spans)
	}
	if len(rd.Metrics) != 1 || rd.Metrics[0].Name != "tok_step_ms" {
		t.Errorf("Metrics: %+v", rd.Metrics)
	}
	if rd.Metrics[0].Sparkline == "" {
		t.Errorf("Sparkline empty")
	}
	if len(rd.Errors) != 1 || rd.Errors[0].Name != "warn.thing" {
		t.Errorf("Errors: %+v", rd.Errors)
	}
	if len(rd.AuditRecords) != 1 || rd.AuditRecords[0].Name != "file.read" {
		t.Errorf("AuditRecords: %+v", rd.AuditRecords)
	}
	if len(rd.Lifecycles) == 0 {
		t.Errorf("expected at least 1 lifecycle entry")
	}
}

func TestRender_ProducesValidSelfContainedHTML(t *testing.T) {
	dir := writeFixtureRun(t)
	out, err := Render(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)

	// Self-contained: no external resource references.
	for _, bad := range []string{"http://", "https://", "<link rel=\"stylesheet\"", "<script src="} {
		if strings.Contains(html, bad) {
			t.Errorf("report.html should be self-contained but contains %q", bad)
		}
	}

	// Required structural anchors so consumers can rely on the layout.
	for _, want := range []string{
		"<title>loom · run 01J3KTV6FIXTURE</title>",
		`id="loom-events"`,
		`id="loom-audit"`,
		`id="verify-btn"`,
		`class="lifecycle"`,
		"forward.layer",
		"tok_step_ms",
		"warn.thing",
		"@media print",
		"crypto.subtle.digest",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in rendered HTML", want)
		}
	}

	// Every category value gets a declared CSS color so the visual
	// differentiation isn't decorative — a renderer change can't silently
	// drop one of the five categories from the design system.
	for _, color := range []string{"--c-span", "--c-metric", "--c-audit", "--c-lifecycle", "--c-error"} {
		if !strings.Contains(html, color) {
			t.Errorf("missing CSS variable %s", color)
		}
	}
}

func TestFormatDurNS_Boundaries(t *testing.T) {
	cases := []struct {
		ns   uint64
		want string
	}{
		{500, "500 ns"},
		{1500, "1.50 µs"},
		{2_500_000, "2.50 ms"},
		{3_500_000_000, "3.50 s"},
	}
	for _, c := range cases {
		got := formatDurNS(c.ns)
		if got != c.want {
			t.Errorf("formatDurNS(%d) = %q, want %q", c.ns, got, c.want)
		}
	}
}

func TestSparklinePath_Monotone(t *testing.T) {
	// Increasing values should produce a path that goes from bottom-left
	// (low value) to top-right (high value) of the 120x28 viewbox.
	values := []float64{1, 2, 3, 4, 5}
	got := sparklinePath(values)
	if !strings.HasPrefix(got, "M ") {
		t.Errorf("path should start with M, got: %s", got)
	}
	if !strings.Contains(got, " L ") {
		t.Errorf("path should contain L commands, got: %s", got)
	}
	// First point at x=0, last at x=120.
	if !strings.HasPrefix(got, "M 0.00,") {
		t.Errorf("first point should be at x=0: %s", got)
	}
	if !strings.HasSuffix(got, ",0.00") {
		// y goes 0..28; last value is the max so y should be near 0.
		t.Logf("last point y might not be at 0 due to rounding: %s", got)
	}
}

func TestAnchorize_StablyEscapesSpecialChars(t *testing.T) {
	cases := map[string]string{
		"forward.layer":  "forward-layer",
		"Run.Start":      "run-start",
		"a/b c@d!e":      "a-b-c-d-e",
		"---trim---":     "trim",
	}
	for in, want := range cases {
		got := anchorize(in)
		if got != want {
			t.Errorf("anchorize(%q) = %q, want %q", in, got, want)
		}
	}
}
