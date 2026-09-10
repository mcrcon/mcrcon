package tui

import (
	"fmt"
	"os"
	"path/filepath"
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

// --- connect form keymap ---

func connectModel() Model {
	m := New(Config{})
	m.width = 80
	m.height = 24
	return m
}

func TestConnectTabCyclesFocus(t *testing.T) {
	m := connectModel()
	if m.focusIdx != 0 {
		t.Fatalf("initial focus=%d want 0", m.focusIdx)
	}
	for _, want := range []int{1, 2, 0} {
		um, _ := m.updateConnect(keyMsg("tab"))
		m = um.(Model)
		if m.focusIdx != want {
			t.Fatalf("after tab focus=%d want %d", m.focusIdx, want)
		}
	}
	for _, want := range []int{2, 1, 0} {
		um, _ := m.updateConnect(keyMsg("shift+tab"))
		m = um.(Model)
		if m.focusIdx != want {
			t.Fatalf("after shift+tab focus=%d want %d", m.focusIdx, want)
		}
	}
}

func TestConnectRejectsInvalidPort(t *testing.T) {
	m := connectModel()
	m.inputs[0].SetValue("mc.example.com")
	m.inputs[1].SetValue("70000")
	m.inputs[2].SetValue("secret")
	m.focusIdx = 2
	um, _ := m.updateConnect(keyMsg("enter"))
	m = um.(Model)
	if m.connErr == "" || !strings.Contains(m.connErr, "port") {
		t.Fatalf("expected port error, got %q", m.connErr)
	}
	if m.focusIdx != 1 {
		t.Fatalf("expected focus on port, got %d", m.focusIdx)
	}
	if m.connecting {
		t.Fatal("invalid port must not start connecting")
	}
}

func TestConnectRequiresPassword(t *testing.T) {
	m := connectModel()
	m.inputs[0].SetValue("mc.example.com")
	m.inputs[1].SetValue("25575")
	m.focusIdx = 2
	um, _ := m.updateConnect(keyMsg("enter"))
	m = um.(Model)
	if !strings.Contains(m.connErr, "Password") {
		t.Fatalf("expected password error, got %q", m.connErr)
	}
	if m.focusIdx != 2 {
		t.Fatalf("expected focus on password, got %d", m.focusIdx)
	}
	if m.connecting {
		t.Fatal("missing password must not start connecting")
	}
}

func TestConnectSubmitStartsDial(t *testing.T) {
	m := connectModel()
	m.inputs[0].SetValue("mc.example.com")
	m.inputs[1].SetValue("25575")
	m.inputs[2].SetValue("secret")
	m.focusIdx = 2
	um, cmd := m.updateConnect(keyMsg("enter"))
	m = um.(Model)
	if !m.connecting {
		t.Fatal("expected connecting to start")
	}
	if cmd == nil {
		t.Fatal("expected a dial command")
	}
	if m.screen != screenConnect {
		t.Fatal("connect form should stay until dial succeeds")
	}
}

func TestConnectEscCancelsInFlightDial(t *testing.T) {
	m := connectModel()
	m.connecting = true
	old := m.connSeq
	um, _ := m.updateConnect(keyMsg("esc"))
	m = um.(Model)
	if m.connecting {
		t.Fatal("esc must cancel a connecting dial")
	}
	if m.connSeq != old+1 {
		t.Fatalf("esc must invalidate dial epoch, got %d want %d", m.connSeq, old+1)
	}
	if !strings.Contains(m.connErr, "cancelled") {
		t.Fatalf("expected cancellation notice, got %q", m.connErr)
	}
}

// --- local commands ---

func TestLocalHistoryLogsEntries(t *testing.T) {
	m := sessionModel(80, 24)
	m.history = []string{"list", "time set day", "whitelist add Steve"}
	um, quit, handled, _ := m.doLocal("/history")
	m = um
	if quit || !handled {
		t.Fatal("expected /history to be handled, not quit")
	}
	found := false
	for _, l := range m.logs {
		if strings.Contains(l, "whitelist add Steve") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected /history to log commands, logs:\n%v", m.logs)
	}
}

func TestLocalReconnectBumpsEpoch(t *testing.T) {
	m := sessionModel(80, 24)
	old := m.connSeq
	um, quit, handled, cmd := m.doLocal("/reconnect")
	m = um
	if quit || !handled {
		t.Fatal("expected /reconnect to be handled, not quit")
	}
	if cmd == nil {
		t.Fatal("expected a reconnect command")
	}
	if m.connSeq != old+1 {
		t.Fatalf("reconnect must bump epoch, got %d want %d", m.connSeq, old+1)
	}
	if !m.connecting {
		t.Fatal("expected model to be connecting")
	}
}

func TestLocalClearEmptiesLog(t *testing.T) {
	m := sessionModel(80, 24)
	for i := 0; i < 10; i++ {
		m.pushLog("line")
	}
	if len(m.logs) < 2 {
		t.Fatalf("expected logs to accumulate, got %d", len(m.logs))
	}
	um, quit, handled, _ := m.doLocal("/clear")
	m = um
	if quit || !handled {
		t.Fatal("/clear must be handled")
	}
	// pushLog kept the "screen cleared" marker, so only that remains.
	if len(m.logs) != 1 {
		t.Fatalf("expected cleared marker only, got %d logs", len(m.logs))
	}
}

// --- suggestions overflow ---

func TestSuggestionLineShowsMoreForManyCandidates(t *testing.T) {
	m := sessionModel(200, 40)
	m.ti.SetValue("s")
	line := m.suggestionLine()
	if line == "" {
		t.Fatal("expected suggestion line for prefix 's'")
	}
	if !strings.Contains(line, "+") || !strings.Contains(line, "more") {
		t.Fatalf("expected '+N more' for many candidates, got %q", line)
	}
}

func TestSuggestionLineEmptyWhenNoCandidates(t *testing.T) {
	m := sessionModel(200, 40)
	m.ti.SetValue("zzz")
	if got := m.suggestionLine(); got != "" {
		t.Fatalf("expected no suggestion line, got %q", got)
	}
	m.ti.SetValue("")
	if got := m.suggestionLine(); got != "" {
		t.Fatalf("expected no suggestion on empty input, got %q", got)
	}
}

// --- auto-connect ---

func TestInitAutoConnectsWithCredentials(t *testing.T) {
	m := New(Config{Host: "h", Port: 1, Password: "p"})
	if m.screen != screenSession {
		t.Fatal("expected session screen when credentials are provided")
	}
	if cmd := m.Init(); cmd == nil {
		t.Fatal("expected Init to return a dial command")
	}
	if !m.connecting {
		t.Fatal("expected model to be connecting from startup")
	}
}

func TestInitShowsFormWithoutCredentials(t *testing.T) {
	m := New(Config{})
	if m.screen != screenConnect {
		t.Fatal("expected connect form when no credentials")
	}
	// Init still returns a blink so the cursor animates on the form.
	if cmd := m.Init(); cmd == nil {
		t.Fatal("expected Init to return a blink command")
	}
}

// --- viewport pagination ---

func TestSessionPaginationKeys(t *testing.T) {
	m := sessionModel(80, 24)
	for i := 0; i < 60; i++ {
		m.pushLog("line")
	}
	m.vp.GotoBottom()
	m.follow = true
	um, _ := m.updateSession(keyMsg("pgup"))
	m = um.(Model)
	if m.follow {
		t.Fatal("expected pgup to leave follow mode")
	}
	um, _ = m.updateSession(keyMsg("home"))
	m = um.(Model)
	if !m.vp.AtTop() {
		t.Fatal("expected 'home' to jump to top")
	}
	um, _ = m.updateSession(keyMsg("end"))
	m = um.(Model)
	if !m.follow || !m.vp.AtBottom() {
		t.Fatal("expected 'end' to jump to bottom and resume follow")
	}
}

// --- history caps ---

func TestHistoryCapIsUniform(t *testing.T) {
	if maxInMemHistory != maxPersistedHistory {
		t.Fatalf("history caps must match: mem=%d disk=%d", maxInMemHistory, maxPersistedHistory)
	}
	m := sessionModel(80, 24)
	for i := 0; i < maxInMemHistory+50; i++ {
		m.history = appendCmd(m.history, fmt.Sprintf("cmd %d", i))
		if len(m.history) > maxInMemHistory {
			m.history = m.history[len(m.history)-maxInMemHistory:]
		}
	}
	if len(m.history) != maxInMemHistory {
		t.Fatalf("expected in-memory history capped at %d, got %d", maxInMemHistory, len(m.history))
	}
}

// --- last-connection persistence ---

func TestSavedConnRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conn.json")
	saveConn(path, "mc.example.com", 25575)
	sc := loadConn(path)
	if sc.Host != "mc.example.com" || sc.Port != 25575 {
		t.Fatalf("roundtrip failed: %+v", sc)
	}
}

func TestLoadConnRejectsBadData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conn.json")
	if sc := loadConn(path); sc.Host != "" {
		t.Fatalf("missing file should yield empty, got %+v", sc)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sc := loadConn(path); sc.Host != "" {
		t.Fatalf("invalid json should yield empty, got %+v", sc)
	}
	if err := os.WriteFile(path, []byte(`{"host":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if sc := loadConn(path); sc.Host != "" {
		t.Fatalf("empty host should be rejected, got %+v", sc)
	}
}

func TestNewPrefillsHostFromSavedConn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conn.json")
	saveConn(path, "persisted.example.com", 25577)
	m := New(Config{ConnFile: path})
	if got := m.inputs[0].Value(); got != "persisted.example.com" {
		t.Fatalf("expected saved host prefill, got %q", got)
	}
	if got := m.inputs[1].Value(); got != "25577" {
		t.Fatalf("expected saved port prefill, got %q", got)
	}
	// Explicit config must win over the saved file.
	m2 := New(Config{ConnFile: path, Host: "flag.example.com"})
	if got := m2.inputs[0].Value(); got != "flag.example.com" {
		t.Fatalf("explicit host must win, got %q", got)
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
