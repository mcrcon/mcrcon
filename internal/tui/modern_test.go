package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestDetectThemeEnvOverrides(t *testing.T) {
	t.Setenv("MCRCON_THEME", "light")
	if got := detectTheme(); got != lightPalette {
		t.Fatal("MCRCON_THEME=light must select the light palette")
	}
	t.Setenv("MCRCON_THEME", "dark")
	if got := detectTheme(); got != darkPalette {
		t.Fatal("MCRCON_THEME=dark must select the dark palette")
	}
	t.Setenv("MCRCON_THEME", "auto")
	want := darkPalette
	if !lipgloss.HasDarkBackground() {
		want = lightPalette
	}
	if got := detectTheme(); got != want {
		t.Fatal("auto must follow the terminal background")
	}
	t.Setenv("MCRCON_THEME", "bogus")
	if got := detectTheme(); got != darkPalette {
		t.Fatal("unknown theme values must fall back to dark")
	}
}

func TestGradTextKeepsWidth(t *testing.T) {
	s := gradText("mcrcon", theme.accent, theme.accentHi)
	if got := visibleWidth(s); got != 6 {
		t.Fatalf("gradient must keep visible width 6, got %d", got)
	}
}

func TestVisibleWidthIgnoresANSI(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#5EBB2B")).Bold(true).Render("hello")
	if got := visibleWidth(styled); got != 5 {
		t.Fatalf("expected visible width 5, got %d", got)
	}
	if got := visibleWidth("--mcrcon--"); got != 10 {
		t.Fatalf("expected visible width 10, got %d", got)
	}
}

func TestPadVisible(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(theme.accent).Render("x")
	got := padVisible(styled, 5)
	if visibleWidth(got) != 5 {
		t.Fatalf("padVisible must produce width 5, got %q (w=%d)", got, visibleWidth(got))
	}
	if w := visibleWidth(padVisible("", 3)); w != 3 {
		t.Fatalf("empty padding must reach width 3, got %d", w)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("short strings must pass through, got %q", got)
	}
	got := truncate("abcdefghij", 4)
	if got != "abc…" {
		t.Fatalf("expected truncated 'abc…', got %q", got)
	}
	if truncate("x", 0) != "" {
		t.Fatal("width 0 must return empty")
	}
}

func TestSparkline(t *testing.T) {
	if got := sparkline(nil, 5); got != "░░░░░" {
		t.Fatalf("empty sparkline must be all light dots, got %q", got)
	}
	if got := sparkline([]float64{0, 0, 0}, 3); got != "░░░" {
		t.Fatalf("zero values must render as dots, got %q", got)
	}
	flat := sparkline([]float64{10, 10, 10, 10}, 4)
	if visibleWidth(flat) != 4 {
		t.Fatalf("flat sparkline width must be 4, got %d (%q)", visibleWidth(flat), flat)
	}
	// A high value fills its bar, a tiny value shows the lowest shard.
	spike := sparkline([]float64{100, 1}, 2)
	if !strings.Contains(string([]rune(spike)[0]), "█") || !strings.Contains(string([]rune(spike)[1]), "▁") {
		t.Fatalf("expected █ then ▁ for imbalanced values, got %q", spike)
	}
}

func TestSidebarShownOnWideTerminal(t *testing.T) {
	m := sessionModel(150, 32)
	if m.sidebarW <= 0 {
		t.Fatalf("expected sidebar on a wide terminal, got %d", m.sidebarW)
	}
	out := m.View()
	for _, want := range []string{"SERVER", "Host", "Port", "RECENT"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sidebar missing %q in:\n%s", want, out)
		}
	}
}

func TestSidebarHiddenOnNarrowTerminal(t *testing.T) {
	m := sessionModel(96, 30)
	if m.sidebarW != 0 {
		t.Fatalf("sidebar must be hidden on narrow terminals, got width %d", m.sidebarW)
	}
	if out := m.View(); strings.Contains(out, "SERVER") {
		t.Fatalf("sidebar content must not render when hidden:\n%s", out)
	}
}

func TestExecResultsRecordStatsAndPing(t *testing.T) {
	m := sessionModel(120, 30)
	m.connSeq = 1
	m.pending = 1

	upd, _ := m.Update(execResultMsg{cmd: "list", out: "players online: 3", err: nil, dur: 123 * time.Millisecond, seq: 1})
	m = upd.(Model)
	if m.cmdCount != 1 {
		t.Fatalf("expected 1 command recorded, got %d", m.cmdCount)
	}
	if m.errCount != 0 {
		t.Fatalf("expected no errors, got %d", m.errCount)
	}
	if len(m.pings) != 1 || m.pings[0] < 122 || m.pings[0] > 124 {
		t.Fatalf("expected ping ≈123ms recorded, got %v", m.pings)
	}

	m.pending = 1
	upd, _ = m.Update(execResultMsg{cmd: "list", out: "", err: errTest, dur: 5 * time.Millisecond, seq: 1})
	m = upd.(Model)
	if m.cmdCount != 2 || m.errCount != 1 {
		t.Fatalf("expected cmdCount=2 errCount=1, got %d/%d", m.cmdCount, m.errCount)
	}
	if len(m.pings) != 1 {
		t.Fatalf("errors must not append pings, got %v", m.pings)
	}
}

func TestStaleExecResultIgnoresCounters(t *testing.T) {
	m := sessionModel(120, 30)
	m.connSeq = 5 // current epoch
	upd, _ := m.Update(execResultMsg{cmd: "list", out: "x", err: nil, dur: 1 * time.Millisecond, seq: 4})
	m = upd.(Model)
	if m.cmdCount != 0 || len(m.pings) != 0 {
		t.Fatalf("stale results must not touch stats, got %d/%v", m.cmdCount, m.pings)
	}
}

func TestHelpModalKeepsRowsAligned(t *testing.T) {
	m := sessionModel(130, 34)
	inner := 126
	lines := strings.Split(m.helpBody(inner), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected several help rows, got %d", len(lines))
	}
	for i, l := range lines {
		if got := visibleWidth(l); got != inner {
			t.Fatalf("help row %d must span %d columns, got %d (%q)", i, inner, got, l)
		}
	}
	out := m.helpModal(m.viewportHeight(false))
	if !strings.Contains(out, "Local commands") {
		t.Fatalf("help modal must contain local commands section:\n%s", out)
	}
	vh := m.viewportHeight(false)
	if got := len(splitNoTrailing(out)); got != vh {
		t.Fatalf("modal must keep the pane height stable, got %d rows, want %d", got, vh)
	}
}
