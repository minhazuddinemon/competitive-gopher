package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// CaseOutcome is a platform-agnostic view of one test case's result.
// Codeforces, AtCoder, and LeetCode all produce this shape so they can
// render through the same component.
type CaseOutcome struct {
	CaseNum  int
	Passed   bool
	TimedOut bool
	Duration time.Duration // zero for LeetCode (timed per-binary, not per-case)
	Input    string
	Got      string
	Expected string
}

// Pre-built status badges — created once, reused on every render.
var (
	acBadge  = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true).Render("✓ AC")
	waBadge  = lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render("✗ WA")
	tleBadge = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render("⏳ TLE")

	// lineWidth is the target width (in terminal columns) for dividers, the
	// header row, and the outer test-block box -- kept as one shared
	// constant so everything stays visually aligned to the same width.
	lineWidth = 144
)

// splitRow places left and right on one terminal line, right-aligned.
// It uses lipgloss's ANSI-aware width measurement so styled/colored text
// doesn't throw off the alignment.
func splitRow(left, right string, totalWidth int) string {
	gap := max(totalWidth-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// justifyThree spreads left/mid/right across the full totalWidth: left
// pinned to the left edge, right pinned to the right edge, mid centered
// between them. Used so CASE/STATUS/TIME (and their per-row values) fill
// the whole box width instead of bunching up at the left. Uses lipgloss's
// ANSI-aware width measurement so styled/colored text doesn't throw off
// the spacing.
func justifyThree(left, mid, right string, totalWidth int) string {
	lw := lipgloss.Width(left)
	mw := lipgloss.Width(mid)
	rw := lipgloss.Width(right)

	midStart := (totalWidth - mw) / 2
	if midStart < lw+2 {
		midStart = lw + 2
	}
	rightStart := totalWidth - rw
	if rightStart < midStart+mw+2 {
		rightStart = midStart + mw + 2
	}

	gap1 := max(midStart-lw, 1)
	gap2 := max(rightStart-(midStart+mw), 1)

	return left + strings.Repeat(" ", gap1) + mid + strings.Repeat(" ", gap2) + right
}

// ─── Header ──────────────────────────────────────────────────────────────────

// RenderHeaderImages draws the platform icon (left) and gopher icon (right)
// as real pixel images (kitty graphics protocol via chafa), justified
// against lineWidth so they sit at the box's left/right edges rather than
// bunched together. Falls back to plain colored labels, justified the same
// way, when chafa/kitty support isn't available.
//
// Call this first, immediately after clearing the screen -- the text info
// row (RenderHeaderInfoRow) is separate because it needs the compile
// result, which isn't known yet at this point.
func RenderHeaderImages(platform string) {
	theme := ThemeFor(platform)

	if PrintImages(theme.IconName, "gopher", 14, 7, lineWidth) {
		return
	}

	platformLabel := lipgloss.NewStyle().Foreground(theme.Color).Bold(true).Render("◉ " + theme.Name)
	gopherLabel := lipgloss.NewStyle().Foreground(ColorGopherAccent).Bold(true).Render("◉ Go Gopher")
	fmt.Println(splitRow(" "+platformLabel, gopherLabel+" ", lineWidth))
	fmt.Println()
}

// RenderHeaderInfoRow prints the platform badge, problem title, time-limit
// constraint, AND the compile outcome (DONE/FAILED) all on one row -- title
// info left, compile status right-aligned to lineWidth -- followed by a
// divider. Call this once the compile result is actually known; print any
// compiler error text yourself immediately after -- it belongs below this
// row, not on it.
func RenderHeaderInfoRow(platform, title string, timeLimitMs int, compiled bool) {
	theme := ThemeFor(platform)

	badge := theme.Badge()
	titleText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(title)
	constraint := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).
		Render(fmt.Sprintf("⏳ %dms", timeLimitMs))
	left := " " + badge + "  " + titleText + "   " + constraint

	status := lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render("⚙ DONE")
	if !compiled {
		status = lipgloss.NewStyle().Bold(true).Foreground(ColorError).Render("⚙ FAILED")
	}

	fmt.Println(splitRow(left, status, lineWidth))
	fmt.Println(strings.Repeat("─", lineWidth))
}

// ─── Test block ──────────────────────────────────────────────────────────────

// RenderTestBlock wraps the complete test-run output — column header, every
// per-case row (with INPUT/EXPECTED/GOT diff on wrong answers), and the
// final pass/fail summary banner — inside a single rounded-border box
// coloured with the platform's brand accent.
//
// All three platforms route through here. Collect every CaseOutcome first,
// then call this once; do NOT call the old RenderCaseTableHeader / RenderCase
// / RenderSummary functions separately.
func RenderTestBlock(outcomes []CaseOutcome, allPassed bool, copiedToClipboard bool, platform string) {
	theme := ThemeFor(platform)
	// innerWidth = lineWidth minus 2 border chars and 2 padding chars (1 each side)
	innerWidth := lineWidth - 4

	var sb strings.Builder

	// ── Column header — spread CASE / STATUS / TIME across the box's
	// full width (left / center / right) instead of bunching at the left ──
	head := lipgloss.NewStyle().Bold(true).Foreground(ColorMuted)
	sb.WriteString(" ")
	sb.WriteString(justifyThree(head.Render("CASE"), head.Render("STATUS"), head.Render("TIME"), innerWidth-1))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", innerWidth))
	sb.WriteString("\n")

	// ── Per-case rows ─────────────────────────────────────────────────────
	for _, c := range outcomes {
		sb.WriteString(caseRowStr(c, innerWidth))
		sb.WriteString("\n")
		if !c.Passed && !c.TimedOut {
			sb.WriteString(diffBlockStr(c.Input, c.Expected, c.Got, innerWidth))
			sb.WriteString("\n")
		}
	}

	// ── Pass / fail summary ───────────────────────────────────────────────
	sb.WriteString(strings.Repeat("─", innerWidth))
	sb.WriteString("\n")
	sb.WriteString(summaryStr(allPassed, copiedToClipboard))

	// ── Outer border box ──────────────────────────────────────────────────
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Color).
		Padding(0, 1)

	fmt.Println(box.Render(sb.String()))
}

// ─── Internal helpers (return strings, don't print) ──────────────────────────

func caseRowStr(c CaseOutcome, width int) string {
	status := acBadge
	switch {
	case c.TimedOut:
		status = tleBadge
	case !c.Passed:
		status = waBadge
	}
	durStr := "-"
	if c.Duration > 0 {
		durStr = c.Duration.String()
	}
	caseNum := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("%d", c.CaseNum))
	return " " + justifyThree(caseNum, status, durStr, width-1)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(empty)"
	}
	return strings.TrimSpace(s)
}

func diffBlockStr(input, expected, got string, width int) string {
	// diffBox nests inside the outer box's content area (which is `width`
	// wide), indented by MarginLeft(2). Its own border(2) + padding(0,2 =
	// 4) eat into that before any column gets drawn, so back those out to
	// get the space actually available for the three columns.
	const marginLeft, borderAndPadding = 2, 2 + 4
	available := max(width-marginLeft-borderAndPadding, 24)

	colWidth := available / 3
	lastColWidth := available - colWidth*2 // give any remainder to GOT

	diffBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorMuted).
		Padding(0, 2).
		MarginLeft(marginLeft)

	col := func(label, value string, color lipgloss.Color, w int) string {
		h := lipgloss.NewStyle().Bold(true).Foreground(color).Width(w).Render(label)
		b := lipgloss.NewStyle().Foreground(ColorText).Width(w).Render(orDash(value))
		return lipgloss.JoinVertical(lipgloss.Left, h, b)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top,
		col("INPUT", input, ColorMuted, colWidth),
		col("EXPECTED", expected, ColorWarning, colWidth),
		col("GOT", got, ColorError, lastColWidth),
	)
	return diffBox.Render(row)
}

func summaryStr(allPassed, copiedToClipboard bool) string {
	if allPassed {
		label := "  ALL TESTS PASSED"
		if copiedToClipboard {
			label += "    📋 Copied to Clipboard"
		}
		return " " + lipgloss.NewStyle().Bold(true).
			Foreground(ColorDarkBg).Background(ColorSuccess).
			Padding(0, 3).Render(label)
	}
	return " " + lipgloss.NewStyle().Bold(true).
		Foreground(ColorDarkBg).Background(ColorError).
		Padding(0, 3).Render("  SOME TESTS FAILED")
}

// ─── Menu chrome ─────────────────────────────────────────────────────────────

// RenderMenuHint prints the keyboard navigation hint pill above the menu.
func RenderMenuHint() {
	pill := lipgloss.NewStyle().Bold(true).Foreground(ColorDarkBg).Background(ColorGopherAccent).
		Padding(0, 2).Render("↑↓ Navigate   ⏎ Confirm")
	fmt.Println("\n " + pill)
}

// MenuHighlightColor is a fixed bright color for the selected menu item.
// It is intentionally NOT the platform brand color (AtCoder's brand is
// near-black, which is invisible as a dark-terminal selection highlight).
var MenuHighlightColor = ColorGopherAccent
