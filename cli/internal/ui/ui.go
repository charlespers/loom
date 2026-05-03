// Package ui is the terminal design system shared by every Loom CLI
// command. The goal is one consistent visual language: a single accent
// per event category, restrained color, tabular numerals, no decorative
// chrome. The same five categories (span / metric / audit / lifecycle /
// error) get the same treatment in `loom ls`, `loom show`, `loom verify`
// terminal output, and (later) the live TUI.
//
// Color is opt-out: NO_COLOR / non-terminal stdout disables it.
// Glyphs are ASCII fallbacks-friendly; no emoji.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Color palette ────────────────────────────────────────────────────────
//
// The same accents the report.html uses, mapped to lipgloss colors that
// degrade gracefully on non-truecolor terminals.
var (
	cInk      = lipgloss.AdaptiveColor{Light: "#0f172a", Dark: "#e2e8f0"}
	cInkSoft  = lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94a3b8"}
	cInkMute  = lipgloss.AdaptiveColor{Light: "#94a3b8", Dark: "#64748b"}
	cRule     = lipgloss.AdaptiveColor{Light: "#e2e8f0", Dark: "#334155"}
	cAccent   = lipgloss.AdaptiveColor{Light: "#1e293b", Dark: "#f1f5f9"}

	cSpan      = lipgloss.Color("#0d9488")
	cMetric    = lipgloss.Color("#4f46e5")
	cAudit     = lipgloss.Color("#b45309")
	cLifecycle = lipgloss.AdaptiveColor{Light: "#64748b", Dark: "#94a3b8"}
	cError     = lipgloss.Color("#dc2626")
	cOK        = lipgloss.Color("#15803d")
	cWarn      = lipgloss.Color("#b45309")
)

// CategoryColor returns the accent color for a Loom event category.
// Unknown categories fall back to the muted-ink color so they never
// look like first-class citizens.
func CategoryColor(cat string) lipgloss.TerminalColor {
	switch cat {
	case "span":
		return cSpan
	case "device_span":
		// Device spans share the span accent intentionally — they're a
		// timing measurement of the same logical work, just resolved
		// asynchronously on an accelerator. The render code disambiguates
		// via section heading and label, not color.
		return cSpan
	case "metric":
		return cMetric
	case "audit":
		return cAudit
	case "lifecycle":
		return cLifecycle
	case "error":
		return cError
	default:
		return cInkMute
	}
}

// ── Reusable styles ──────────────────────────────────────────────────────

var (
	// Eyebrow is a small caps section label. Used above tables and
	// section headers.
	Eyebrow = lipgloss.NewStyle().
		Foreground(cInkMute).
		Bold(true)

	// H1 is the page title style for the terminal — used at the top of
	// `loom show` and `loom ls`. One per output.
	H1 = lipgloss.NewStyle().
		Foreground(cAccent).
		Bold(true)

	// H2 is a section header.
	H2 = lipgloss.NewStyle().
		Foreground(cAccent).
		Bold(true).
		Underline(false)

	// Mono is for hashes, IDs, and other things the operator shouldn't
	// have to scan word-by-word.
	Mono = lipgloss.NewStyle().Foreground(cInk)

	// Soft is the secondary text color: timestamps, byte counts, inline
	// metadata.
	Soft = lipgloss.NewStyle().Foreground(cInkSoft)

	// Mute is the tertiary text color: things that need to be present
	// but read past.
	Mute = lipgloss.NewStyle().Foreground(cInkMute)

	// Rule is a horizontal line.
	Rule = lipgloss.NewStyle().Foreground(cRule)
)

// Badge renders a small status pill — "completed", "warn", "broken".
func Badge(status string) string {
	style := lipgloss.NewStyle().
		Padding(0, 1).
		Bold(true).
		Foreground(cOK).
		Background(lipgloss.Color("#dcfce7"))
	switch normalizeStatus(status) {
	case "warn":
		style = style.Foreground(cWarn).Background(lipgloss.Color("#fef3c7"))
	case "error":
		style = style.Foreground(cError).Background(lipgloss.Color("#fee2e2"))
	}
	return style.Render(strings.ToUpper(status))
}

func normalizeStatus(s string) string {
	switch s {
	case "completed", "ok":
		return "ok"
	case "warn", "warning":
		return "warn"
	case "failed", "broken", "error", "crashed":
		return "error"
	default:
		return "ok"
	}
}

// CatTag renders the category name in its accent color, padded to a
// fixed width so columns of mixed categories line up.
func CatTag(cat string) string {
	return lipgloss.NewStyle().
		Foreground(CategoryColor(cat)).
		Bold(true).
		Width(9).
		Render(cat)
}

// OK / Fail / Warn render a one-glyph indicator + label in the matching
// color. Glyphs are plain ASCII so they survive every terminal.
func OK(label string) string   { return iconLabel("✓", cOK, label) }
func Fail(label string) string { return iconLabel("✗", cError, label) }
func Warn(label string) string { return iconLabel("!", cWarn, label) }

func iconLabel(icon string, c lipgloss.TerminalColor, label string) string {
	style := lipgloss.NewStyle().Foreground(c).Bold(true)
	if label == "" {
		return style.Render(icon)
	}
	return style.Render(icon) + " " + lipgloss.NewStyle().Foreground(cInk).Render(label)
}

// ── Tables ───────────────────────────────────────────────────────────────

// Column describes one column of a Table.
type Column struct {
	Title string
	// Min is a lower bound on width (columns won't shrink past it). 0 = no min.
	Min int
	// Max caps width. 0 = no max.
	Max int
	// Align: "left" (default) or "right".
	Align string
	// Style transforms the rendered cell. nil = no style.
	Style func(s string) string
}

// Table is a quiet, designed text table. Header in eyebrow style;
// thin rule below header; consistent column gutters; right-aligned
// numbers; truncates long cells with an ellipsis.
type Table struct {
	Columns []Column
	Rows    [][]string
	// Gutter between columns, in spaces.
	Gutter int
}

// Render returns the table as a multi-line string. No trailing newline.
func (t Table) Render() string {
	gutter := t.Gutter
	if gutter == 0 {
		gutter = 2
	}
	widths := make([]int, len(t.Columns))
	for i, c := range t.Columns {
		widths[i] = lipgloss.Width(c.Title)
		if c.Min > widths[i] {
			widths[i] = c.Min
		}
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			w := lipgloss.Width(cell)
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i, c := range t.Columns {
		if c.Max > 0 && widths[i] > c.Max {
			widths[i] = c.Max
		}
	}

	var sb strings.Builder

	// Header.
	for i, c := range t.Columns {
		if i > 0 {
			sb.WriteString(strings.Repeat(" ", gutter))
		}
		title := strings.ToUpper(c.Title)
		sb.WriteString(Eyebrow.Render(padOrTrunc(title, widths[i], c.Align)))
	}
	sb.WriteByte('\n')

	// Rule.
	totalWidth := 0
	for i, w := range widths {
		if i > 0 {
			totalWidth += gutter
		}
		totalWidth += w
	}
	sb.WriteString(Rule.Render(strings.Repeat("─", totalWidth)))
	sb.WriteByte('\n')

	// Rows.
	for ri, row := range t.Rows {
		for i, c := range t.Columns {
			if i > 0 {
				sb.WriteString(strings.Repeat(" ", gutter))
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			cell = padOrTrunc(cell, widths[i], c.Align)
			if c.Style != nil {
				cell = c.Style(cell)
			}
			sb.WriteString(cell)
		}
		if ri < len(t.Rows)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// padOrTrunc pads or truncates `s` to width `w`. Truncation uses an
// ellipsis. Alignment is "right" or anything else (left).
func padOrTrunc(s string, w int, align string) string {
	cw := lipgloss.Width(s)
	if cw == w {
		return s
	}
	if cw > w {
		// Truncate from the end with ellipsis.
		if w <= 1 {
			return string([]rune(s)[:w])
		}
		runes := []rune(s)
		return string(runes[:w-1]) + "…"
	}
	pad := strings.Repeat(" ", w-cw)
	if align == "right" {
		return pad + s
	}
	return s + pad
}

// ── KV blocks ────────────────────────────────────────────────────────────

// KV is a label-value pair rendered with consistent gutters. Used for
// the metadata block at the top of `loom show`.
type KV struct {
	Label string
	Value string
}

// RenderKV writes a list of KVs aligned on the value column.
func RenderKV(items []KV) string {
	maxLabel := 0
	for _, it := range items {
		w := lipgloss.Width(it.Label)
		if w > maxLabel {
			maxLabel = w
		}
	}
	var sb strings.Builder
	for i, it := range items {
		label := strings.ToUpper(it.Label)
		sb.WriteString(Eyebrow.Render(padOrTrunc(label, maxLabel+2, "left")))
		sb.WriteString(it.Value)
		if i < len(items)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// ── Output sinks & color enable ──────────────────────────────────────────

// IsTTY reports whether w looks like a terminal. Used to decide whether
// to emit color escapes; lipgloss does its own auto-detection but we
// expose this for downstream conditionals (e.g. machine-readable mode).
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// Println writes a styled line followed by a newline.
func Println(w io.Writer, s string) {
	fmt.Fprintln(w, s)
}

// Short formats an SHA-256 hex string as "abcd1234…f7e9" so it fits
// in a terminal column without losing the prefix-and-suffix structure
// auditors actually use to disambiguate.
func Short(hex string) string {
	if len(hex) <= 14 {
		return hex
	}
	return hex[:8] + "…" + hex[len(hex)-4:]
}

// HumanCount formats a count with a trailing space-separated unit; "k"
// suffix for >=1000, etc. Keeps the units terse so columns stay narrow.
func HumanCount(n uint64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

// HumanDurationMS picks a unit ms/s/min/h depending on magnitude.
func HumanDurationMS(ms uint64) string {
	switch {
	case ms == 0:
		return "—"
	case ms < 1000:
		return fmt.Sprintf("%d ms", ms)
	case ms < 60_000:
		return fmt.Sprintf("%.2f s", float64(ms)/1000)
	case ms < 3_600_000:
		return fmt.Sprintf("%.1f min", float64(ms)/60_000)
	default:
		return fmt.Sprintf("%.1f h", float64(ms)/3_600_000)
	}
}
