package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConnectViewRenders(t *testing.T) {
	m := New(Config{})
	m.width = 80
	m.height = 24
	out := m.View()
	if !strings.Contains(out, "mcrcon") {
		t.Fatalf("expected view to mention mcrcon, got:\n%s", out)
	}
	if !strings.Contains(out, "Connect") {
		t.Fatalf("expected connect view, got:\n%s", out)
	}
}

func TestSessionViewRendersAfterResize(t *testing.T) {
	m := New(Config{Host: "127.0.0.1", Port: 25575, Password: "x", Timeout: 2 * time.Second})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	out := m.View()
	if !strings.Contains(out, "127.0.0.1") {
		t.Fatalf("expected session view to show host, got:\n%s", out)
	}
	// Help overlay must not panic.
	m.showHelp = true
	if out := m.View(); !strings.Contains(out, "Local commands") {
		t.Fatalf("expected help overlay, got:\n%s", out)
	}
}

func TestLocalCommands(t *testing.T) {
	m := New(Config{Host: "h", Port: 1, Password: "p"})
	m.width = 80
	m.height = 24
	um, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = um.(Model)

	nm, quit, handled, _ := m.doLocal("/help")
	if quit || !handled {
		t.Fatal("expected /help to be handled")
	}
	m = nm
	if len(m.logs) == 0 {
		t.Fatal("expected /help to log lines")
	}

	if _, quit, _, _ := m.doLocal("/quit"); !quit {
		t.Fatal("expected /quit to quit")
	}
	if _, _, handled, _ := m.doLocal("/list"); handled {
		t.Fatal("expected /list to fall through to server")
	}
}

func TestQuestionMarkTypesWhenInputNonEmpty(t *testing.T) {
	m := New(Config{Host: "h", Port: 1, Password: "p"})
	um, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = um.(Model)
	// Force session screen for the test.
	m.screen = screenSession
	m.ti.SetValue("wh")
	um2, _ := m.updateSession(keyMsg("?"))
	m = um2.(Model)
	if m.showHelp {
		t.Fatal("expected '?' to be typed, not open help, when input is non-empty")
	}
	if !strings.Contains(m.ti.Value(), "?") {
		t.Fatalf("expected '?' in input, got %q", m.ti.Value())
	}
}

func TestQuestionMarkTogglesHelpWhenInputEmpty(t *testing.T) {
	m := New(Config{Host: "h", Port: 1, Password: "p"})
	um, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = um.(Model)
	m.screen = screenSession
	m.ti.SetValue("")
	um2, _ := m.updateSession(keyMsg("?"))
	m = um2.(Model)
	if !m.showHelp {
		t.Fatal("expected '?' to open help when input is empty")
	}
}

func TestSmartFollowKeepsPosition(t *testing.T) {
	m := New(Config{Host: "h", Port: 1, Password: "p"})
	um, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = um.(Model)
	m.screen = screenSession
	// Fill with content and scroll up.
	for i := 0; i < 50; i++ {
		m.pushLog("line")
	}
	m.vp.GotoBottom()
	m.follow = true
	m.vp.PageUp()
	m.syncFollow()
	if m.follow {
		t.Fatal("expected follow=false after scrolling up")
	}
	n := m.newLines
	m.pushLog("fresh")
	m.stickToBottom()
	if m.newLines != n+1 {
		t.Fatalf("expected unseen counter to grow, got %d", m.newLines)
	}
	um2, _ := m.updateSession(keyMsg("end"))
	m = um2.(Model)
	if !m.follow || m.newLines != 0 {
		t.Fatal("expected 'end' to resume following")
	}
}

func TestSuggestions(t *testing.T) {
	m := New(Config{})
	m.ti.SetValue("li")
	matches := m.suggestions()
	found := false
	for _, s := range matches {
		if s == "list" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'list' in suggestions, got %v", matches)
	}
	m.ti.SetValue("list engineered")
	if got := m.suggestions(); got != nil {
		t.Fatalf("expected no suggestions for multi-word input, got %v", got)
	}
}

func TestStripColors(t *testing.T) {
	in := "§aHello §cWorld\x1b[31m!"
	if got := cleanText(in); got != "Hello World!" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatOutputRendersColors(t *testing.T) {
	got := formatOutput("§aHello §cWorld")
	want := "\x1b[0m\x1b[92mHello \x1b[0m\x1b[91mWorld\x1b[0m"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := formatOutput("plain"); got != "plain" {
		t.Fatalf("plain must pass through, got %q", got)
	}
}

func TestStaleConnectIgnored(t *testing.T) {
	m := New(Config{})
	m.connSeq = 2
	m.screen = screenConnect
	um, _ := m.Update(connectErrMsg{err: errTest, seq: 1})
	m = um.(Model)
	if m.connErr != "" {
		t.Fatal("expected stale connect error to be ignored")
	}
}

func sessionModel(width, height int) Model {
	m := New(Config{Host: "127.0.0.1", Port: 25575, Password: "x", Timeout: 2 * time.Second})
	um, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = um.(Model)
	m.screen = screenSession
	return m
}

func TestStaleExecResultIgnored(t *testing.T) {
	m := sessionModel(80, 24)
	m.connSeq = 5
	m.pending = 1
	um, _ := m.Update(execResultMsg{cmd: "list", out: "stale output", seq: 4})
	m = um.(Model)
	if m.pending != 0 {
		t.Fatalf("stale result must still decrement pending, got %d", m.pending)
	}
	for _, l := range m.logs {
		if strings.Contains(l, "stale output") {
			t.Fatal("stale exec result must not be logged")
		}
	}
}

func TestExecResultRenderedForCurrentSession(t *testing.T) {
	m := sessionModel(80, 24)
	m.connSeq = 7
	m.pending = 1
	um, _ := m.Update(execResultMsg{cmd: "list", out: "players: 3 online", seq: 7})
	m = um.(Model)
	found := false
	for _, l := range m.logs {
		if strings.Contains(l, "players: 3 online") {
			found = true
		}
	}
	if !found {
		t.Fatalf("current-session exec result must be rendered, logs:\n%v", m.logs)
	}
	if m.pending != 0 {
		t.Fatalf("expected pending to be decremented, got %d", m.pending)
	}
}

func TestDisconnectResetsPendingAndEpoch(t *testing.T) {
	m := sessionModel(80, 24)
	m.pending = 3
	old := m.connSeq
	um, _ := m.updateSession(keyMsg("ctrl+d"))
	m = um.(Model)
	if m.pending != 0 {
		t.Fatalf("disconnect must clear pending, got %d", m.pending)
	}
	if m.connSeq != old+1 {
		t.Fatalf("disconnect must bump session epoch, got %d want %d", m.connSeq, old+1)
	}
	if m.screen != screenConnect {
		t.Fatal("expected to return to the connect screen")
	}
}

// --- helpers ---

type testErr struct{}

func (testErr) Error() string { return "boom" }

var errTest = testErr{}

func keyMsg(s string) tea.KeyMsg {
	// Build a KeyMsg the same way Bubble Tea does for printable runes.
	var k tea.KeyMsg
	switch s {
	case "?", "q":
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	default:
		k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		// Map well-known names back to special keys.
		if special, ok := map[string]tea.KeyType{
			"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab,
			"up": tea.KeyUp, "down": tea.KeyDown, "pgup": tea.KeyPgUp,
			"pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
			"f1": tea.KeyF1,
		}[s]; ok {
			k = tea.KeyMsg{Type: special}
		}
	}
	return k
}
