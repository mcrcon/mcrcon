package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mcrcon/mcrcon/internal/rcon"
)

// Config carries connection settings into the TUI.
type Config struct {
	Host        string
	Port        int
	Password    string
	Timeout     time.Duration
	HistoryFile string
	ConnFile    string // last host/port persistence ("" → default location)
}

// screen selects which view is active.
type screen int

const (
	screenConnect screen = iota
	screenSession
)

// Messages for async RCON work. seq invalidates stale dials
// (e.g. the user pressed Esc while connecting).
type connectedMsg struct {
	client *rcon.Client
	seq    int
}
type connectErrMsg struct {
	err error
	seq int
}
type execResultMsg struct {
	cmd string
	out string
	err error
	dur time.Duration
	seq int // connection/session epoch; stale results (older session) are ignored
}

// History caps are kept identical in-memory and on disk so the persisted
// file can never grow unbounded.
const (
	maxInMemHistory     = 500
	maxPersistedHistory = 500
)

// Model is the root Bubble Tea model.
type Model struct {
	cfg    Config
	screen screen

	// Connection form.
	inputs     []textinput.Model
	focusIdx   int
	connErr    string
	connecting bool
	connSeq    int

	// Session.
	vp        viewport.Model
	ti        textinput.Model
	spinner   spinner.Model
	client    *rcon.Client
	logs      []string
	history   []string
	histIdx   int // position in history nav; len(history) == fresh input
	draft     string
	ready     bool
	width     int
	height    int
	pending   int
	connected bool
	showHelp  bool
	lastPing  time.Duration

	// Smart scroll: follow output unless the user scrolled up.
	follow   bool
	newLines int

	// Modern sidebar (wide terminals) + session statistics.
	sidebarW int
	cmdCount int
	errCount int
	pings    []float64 // recent command latencies in ms, for the sparkline
}

// minSidebarWidth and friends decide when the host/server sidebar is shown.
// The threshold (84 cols) covers phones in landscape and small laptops while
// keeping portrait-sized windows single-pane.
const (
	minSidebarWidth = 84 // terminal columns required to enable the sidebar
	maxSidebarWidth = 32
	minSidebarW     = 18
	minLogWidth     = 40
	sidebarGap      = 2
	maxTrackedPings = 32
)

// Common Minecraft / Paper / Spigot commands for Tab-completion.
var commonCommands = []string{
	"advancement", "ban", "ban-ip", "bossbar", "clear", "clone", "data",
	"datapack", "deop", "difficulty", "effect", "enchant", "fill", "forceload",
	"function", "gamemode", "gamerule", "gc", "give", "help", "kill", "list",
	"locate", "me", "memory", "mspt", "op", "pardon", "pardon-ip", "particle",
	"perf", "pl", "playsound", "plugins", "recipe", "reload", "restart",
	"save-all", "save-off", "save-on", "schedule", "scoreboard", "seed",
	"setblock", "setworldspawn", "spawnpoint", "spreadplayers", "stop",
	"summon", "team", "teleport", "tell", "tellraw", "time", "timings",
	"title", "tp", "tps", "trigger", "ver", "version", "weather", "whitelist",
	"worldborder", "xp",
}

// New creates the TUI model. If host+password are set it starts on the
// session screen and auto-connects; otherwise it shows the connect form.
func New(cfg Config) Model {
	if cfg.ConnFile == "" {
		cfg.ConnFile = defaultConnFile()
	}
	// Remember last used host/port (never the password).
	if saved := loadConn(cfg.ConnFile); saved.Host != "" {
		if cfg.Host == "" {
			cfg.Host = saved.Host
		}
		if cfg.Port == 0 {
			cfg.Port = saved.Port
		}
	}
	if cfg.Port == 0 {
		cfg.Port = 25575
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.HistoryFile == "" {
		cfg.HistoryFile = defaultHistoryFile()
	}

	// --- connection form inputs ---
	hostIn := textinput.New()
	hostIn.Placeholder = "127.0.0.1"
	hostIn.Prompt = "Host     > "
	hostIn.CharLimit = 255
	hostIn.SetValue(cfg.Host)

	portIn := textinput.New()
	portIn.Placeholder = "25575"
	portIn.Prompt = "Port     > "
	portIn.CharLimit = 5
	portIn.Validate = validatePort
	if cfg.Port != 0 {
		portIn.SetValue(strconv.Itoa(cfg.Port))
	}

	passIn := textinput.New()
	passIn.Placeholder = "rcon password"
	passIn.Prompt = "Password > "
	passIn.EchoMode = textinput.EchoPassword
	passIn.EchoCharacter = '•'
	passIn.CharLimit = 512
	passIn.SetValue(cfg.Password)

	inputs := []textinput.Model{hostIn, portIn, passIn}

	// --- session input ---
	ti := textinput.New()
	ti.Placeholder = `Type a command…  ("/help" for local commands, "/quit" to exit)`
	ti.Prompt = "> "
	ti.CharLimit = 4096
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(ColorWarning)

	m := Model{
		cfg:      cfg,
		inputs:   inputs,
		focusIdx: 0,
		ti:       ti,
		spinner:  sp,
		history:  loadHistory(cfg.HistoryFile),
		width:    80,
		height:   24,
		follow:   true,
	}
	m.histIdx = len(m.history)

	if cfg.Host != "" && cfg.Password != "" {
		m.screen = screenSession
		m.connecting = true
		m.connected = false
		m.connSeq = 1
	} else {
		m.screen = screenConnect
		m.inputs[0].Focus()
	}
	m.pushLog(StyleSystem.Render("Welcome to mcrcon — Minecraft RCON console"))
	return m
}

// Init implements tea.Model. Auto-connect when credentials were provided.
func (m Model) Init() tea.Cmd {
	if m.screen == screenSession && m.cfg.Host != "" && m.cfg.Password != "" {
		return tea.Batch(connectCmd(m.cfg, m.connSeq), m.spinner.Tick)
	}
	return textinput.Blink
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func connectCmd(cfg Config, seq int) tea.Cmd {
	return func() tea.Msg {
		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		c, err := rcon.Dial(addr, cfg.Password, cfg.Timeout)
		if err != nil {
			return connectErrMsg{err, seq}
		}
		return connectedMsg{c, seq}
	}
}

func execCmd(c *rcon.Client, cmd string, seq int) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		out, err := c.Execute(cmd)
		return execResultMsg{cmd: cmd, out: out, err: err, dur: time.Since(start), seq: seq}
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Spinner ticks while busy.
	if tick, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(tick)
		if m.connecting || m.pending > 0 {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.resizeViewport()
		inputW := max(msg.Width-8, 10)
		formW := min(max(msg.Width-16, 20), 48)
		for i := range m.inputs {
			m.inputs[i].Width = formW
		}
		m.ti.Width = inputW
		return m, nil

	case connectedMsg:
		if msg.seq != m.connSeq {
			// Stale dial (cancelled); avoid leaking the connection.
			if msg.client != nil {
				msg.client.Close()
			}
			return m, nil
		}
		m.client = msg.client
		m.connecting = false
		m.connected = true
		m.connErr = ""
		m.screen = screenSession
		m.follow = true
		m.newLines = 0
		m.cmdCount = 0
		m.errCount = 0
		m.pings = nil
		m.ti.Focus()
		m.pushLog(StyleSystem.Render(fmt.Sprintf("Connected to %s:%d", m.cfg.Host, m.cfg.Port)))
		m.pushLog(StyleSystem.Render(`Type "/help" for local commands • F1 for keybindings`))
		m.vp.GotoBottom()
		saveConn(m.cfg.ConnFile, m.cfg.Host, m.cfg.Port)
		return m, nil

	case connectErrMsg:
		if msg.seq != m.connSeq {
			return m, nil // stale dial, ignore
		}
		m.connecting = false
		m.connected = false
		errStr := friendlyConnError(msg.err)
		if m.screen == screenSession && m.client != nil {
			// Lost a live session (reconnect failed): stay, show in log.
			m.pushLog(StyleError.Render("Connection failed: " + errStr))
			m.pushLog(StyleSystem.Render("Press ctrl+r to retry, or ctrl+d to change server"))
		} else {
			// (Auto-)connect failed before ever connecting: show the form.
			m.screen = screenConnect
			m.connErr = errStr
			for i := range m.inputs {
				if i == m.focusIdx {
					m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			cmds = append(cmds, textinput.Blink)
		}
		return m, tea.Batch(cmds...)

	case execResultMsg:
		if m.pending > 0 {
			m.pending--
		}
		if msg.seq != m.connSeq {
			// Result from a previous connection/session (user reconnected,
			// switched servers, or disconnected). Keep the counter accurate
			// but ignore the stale response entirely.
			return m, nil
		}
		m.cmdCount++
		if msg.err != nil {
			m.errCount++
			m.pushLog(StyleError.Render(fmt.Sprintf("Error (%s): %s", msg.dur.Round(time.Millisecond), cleanText(msg.err.Error()))))
			if isConnError(msg.err) {
				m.connected = false
				m.pushLog(StyleSystem.Render("Connection lost — type /reconnect or press ctrl+r"))
			}
		} else {
			m.lastPing = msg.dur
			m.pings = append(m.pings, float64(msg.dur.Microseconds())/1000.0)
			if len(m.pings) > maxTrackedPings {
				m.pings = m.pings[len(m.pings)-maxTrackedPings:]
			}
			if strings.TrimSpace(rcon.StripColors(msg.out)) == "" {
				m.pushLog(StyleSystem.Render(fmt.Sprintf("(ok, %s — no output)", msg.dur.Round(time.Millisecond))))
			} else {
				for _, line := range strings.Split(formatOutput(msg.out), "\n") {
					m.pushLog(StyleResponse.Render(line))
				}
				m.pushLog(StyleSystem.Render(fmt.Sprintf("— %s", msg.dur.Round(time.Millisecond))))
			}
		}
		m.stickToBottom()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.shutdown()
			return m, tea.Quit
		}
		if m.screen == screenConnect {
			return m.updateConnect(msg)
		}
		return m.updateSession(msg)

	case tea.MouseMsg:
		// Mouse wheel scrolls the log; keep smart-follow in sync.
		if m.screen == screenSession && m.ready {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			m.syncFollow()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil
	}

	// Pass through to focused components on other message types
	// (e.g. cursor blink).
	if m.screen == screenConnect {
		for i := range m.inputs {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	} else {
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// --- connect screen ---

func (m Model) updateConnect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Allow cancelling an in-flight dial.
	if m.connecting {
		if msg.String() == "esc" {
			m.connSeq++ // invalidate the in-flight dial
			m.connecting = false
			m.connErr = "Connection cancelled"
			return m, textinput.Blink
		}
		return m, nil
	}
	switch msg.String() {
	case "tab", "down", "enter":
		if msg.String() == "enter" && m.focusIdx == len(m.inputs)-1 {
			return m.submitConnect()
		}
		m.focusIdx = (m.focusIdx + 1) % len(m.inputs)
	case "shift+tab", "up":
		m.focusIdx = (m.focusIdx - 1 + len(m.inputs)) % len(m.inputs)
	case "esc":
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.inputs[m.focusIdx], cmd = m.inputs[m.focusIdx].Update(msg)
		return m, cmd
	}
	for i := range m.inputs {
		if i == m.focusIdx {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	return m, textinput.Blink
}

func validatePort(s string) error {
	if s == "" {
		return nil
	}
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("port must be 1–65535")
	}
	return nil
}

func (m Model) submitConnect() (tea.Model, tea.Cmd) {
	host := strings.TrimSpace(m.inputs[0].Value())
	portStr := strings.TrimSpace(m.inputs[1].Value())
	pass := m.inputs[2].Value()
	if host == "" {
		host = "127.0.0.1"
	}
	port := 25575
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			m.connErr = "Invalid port — must be 1–65535"
			m.focusIdx = 1
			for i := range m.inputs {
				if i == 1 {
					m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, nil
		}
		port = p
	}
	if pass == "" {
		m.connErr = "Password is required"
		m.focusIdx = 2
		for i := range m.inputs {
			if i == 2 {
				m.inputs[i].Focus()
			} else {
				m.inputs[i].Blur()
			}
		}
		return m, nil
	}
	m.cfg.Host = host
	m.cfg.Port = port
	m.cfg.Password = pass
	m.connErr = ""
	m.connecting = true
	m.connected = false
	m.pending = 0
	m.connSeq++
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
	// Stay on the form until the dial succeeds; Update switches screens.
	return m, tea.Batch(connectCmd(m.cfg, m.connSeq), m.spinner.Tick)
}

// --- session screen ---

func (m Model) updateSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help overlay: explicit close keys only, so "?" remains typeable.
	if m.showHelp {
		switch msg.String() {
		case "esc", "f1", "q":
			m.showHelp = false
			return m, nil
		}
	}

	switch msg.String() {
	case "f1":
		m.showHelp = !m.showHelp
		return m, nil
	case "?":
		// Only toggle help when the input is empty; otherwise type "?".
		if strings.TrimSpace(m.ti.Value()) == "" {
			m.showHelp = !m.showHelp
			return m, nil
		}
	case "ctrl+l":
		m.logs = nil
		m.vp.SetContent("")
		m.newLines = 0
		m.follow = true
		m.pushLog(StyleSystem.Render("— screen cleared —"))
		m.vp.GotoBottom()
		return m, nil
	case "ctrl+r":
		return m.reconnect()
	case "ctrl+d":
		// Back to the connect form (disconnect). Bump the session epoch so
		// in-flight command results from the old connection are ignored.
		if m.client != nil {
			m.client.Close()
			m.client = nil
		}
		m.connected = false
		m.connecting = false
		m.pending = 0
		m.connSeq++
		m.screen = screenConnect
		m.focusIdx = 0
		for i := range m.inputs {
			if i == 0 {
				m.inputs[i].Focus()
			} else {
				m.inputs[i].Blur()
			}
		}
		m.inputs[0].SetValue(m.cfg.Host)
		m.inputs[1].SetValue(strconv.Itoa(m.cfg.Port))
		m.inputs[2].SetValue("")
		return m, textinput.Blink
	case "ctrl+u":
		m.ti.SetValue("")
		m.histIdx = len(m.history)
		m.draft = ""
		return m, nil
	case "esc":
		// Clear the input first; never quit from here (ctrl+c quits).
		if m.ti.Value() != "" {
			m.ti.SetValue("")
			m.histIdx = len(m.history)
			m.draft = ""
			return m, nil
		}
		return m, nil
	case "up", "ctrl+p":
		if len(m.history) == 0 {
			return m, nil
		}
		if m.histIdx == len(m.history) {
			m.draft = m.ti.Value()
		}
		if m.histIdx > 0 {
			m.histIdx--
			m.ti.SetValue(m.history[m.histIdx])
			m.ti.CursorEnd()
		}
		return m, nil
	case "down", "ctrl+n":
		if m.histIdx < len(m.history) {
			m.histIdx++
			if m.histIdx == len(m.history) {
				m.ti.SetValue(m.draft)
			} else {
				m.ti.SetValue(m.history[m.histIdx])
			}
			m.ti.CursorEnd()
		}
		return m, nil
	case "tab":
		m.complete()
		return m, nil
	case "pgup":
		m.vp.PageUp()
		m.syncFollow()
		return m, nil
	case "pgdown":
		m.vp.PageDown()
		m.syncFollow()
		return m, nil
	case "home":
		m.vp.GotoTop()
		m.syncFollow()
		return m, nil
	case "end":
		m.vp.GotoBottom()
		m.follow = true
		m.newLines = 0
		return m, nil
	case "shift+up":
		m.vp.LineUp(1)
		m.syncFollow()
		return m, nil
	case "shift+down":
		m.vp.LineDown(1)
		m.syncFollow()
		return m, nil
	case "enter":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		cmdStr := strings.TrimSpace(m.ti.Value())
		if cmdStr == "" {
			return m, nil
		}
		m.ti.SetValue("")
		m.draft = ""
		if strings.HasPrefix(cmdStr, "/") {
			var quit, handled bool
			var cmd tea.Cmd
			m, quit, handled, cmd = m.doLocal(cmdStr)
			if quit {
				m.shutdown()
				return m, tea.Quit
			}
			if handled {
				return m, cmd
			}
			// Not a local command: "/list" → send "list".
			cmdStr = strings.TrimSpace(strings.TrimPrefix(cmdStr, "/"))
			if cmdStr == "" {
				return m, nil
			}
		}
		return m.sendCommand(cmdStr)
	}

	// Regular typing.
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

// sendCommand logs, records history, and dispatches async exec.
func (m Model) sendCommand(cmdStr string) (tea.Model, tea.Cmd) {
	m.pushLog(StyleCmd.Render("> " + cmdStr))
	m.follow = true
	m.newLines = 0
	m.vp.GotoBottom()
	m.history = appendCmd(m.history, cmdStr)
	if len(m.history) > maxInMemHistory {
		m.history = m.history[len(m.history)-maxInMemHistory:]
	}
	m.histIdx = len(m.history)
	appendHistory(m.cfg.HistoryFile, cmdStr)
	if m.client == nil || !m.connected {
		m.pushLog(StyleError.Render("Not connected — type /reconnect or press ctrl+r"))
		m.vp.GotoBottom()
		return m, nil
	}
	m.pending++
	return m, execCmd(m.client, cmdStr, m.connSeq)
}

// doLocal processes slash-commands. Returns updated model + (quit, handled, cmd).
func (m Model) doLocal(raw string) (Model, bool, bool, tea.Cmd) {
	parts := strings.Fields(raw)
	name := strings.ToLower(parts[0])
	switch name {
	case "/quit", "/exit", "/q":
		return m, true, true, nil
	case "/clear", "/cls":
		m.logs = nil
		m.vp.SetContent("")
		m.newLines = 0
		m.follow = true
		m.pushLog(StyleSystem.Render("— screen cleared —"))
		m.vp.GotoBottom()
		return m, false, true, nil
	case "/reconnect", "/re":
		nm, cmd := m.reconnect()
		return nm.(Model), false, true, cmd
	case "/help", "/h", "/?":
		m.pushLog(StyleHelpTitle.Render("Local commands:"))
		for _, l := range localHelpLines() {
			m.pushLog("  " + l)
		}
		m.stickToBottom()
		return m, false, true, nil
	case "/history":
		if len(m.history) == 0 {
			m.pushLog(StyleSystem.Render("(history is empty)"))
		} else {
			start := 0
			if len(m.history) > 20 {
				start = len(m.history) - 20
			}
			for i := start; i < len(m.history); i++ {
				m.pushLog(StyleSystem.Render(fmt.Sprintf("%4d  %s", i+1, m.history[i])))
			}
		}
		m.stickToBottom()
		return m, false, true, nil
	default:
		// Unknown slash-command → treat as a server command with the
		// leading slash stripped by the caller.
		return m, false, false, nil
	}
}

func (m Model) reconnect() (tea.Model, tea.Cmd) {
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
	m.connected = false
	m.connecting = true
	m.pending = 0
	m.connSeq++
	m.pushLog(StyleSystem.Render(fmt.Sprintf("Reconnecting to %s:%d…", m.cfg.Host, m.cfg.Port)))
	m.vp.GotoBottom()
	return m, tea.Batch(connectCmd(m.cfg, m.connSeq), m.spinner.Tick)
}

// complete finishes the current single-word command from the known list.
// Multiple candidates are shown above the input; Tab completes the common
// prefix, or the full word plus a space when there is exactly one match.
func (m Model) complete() {
	val := m.ti.Value()
	stripped := strings.TrimPrefix(val, "/")
	if strings.Contains(stripped, " ") {
		return // only complete the command word
	}
	fields := strings.Fields(stripped)
	if len(fields) != 1 || fields[0] == "" {
		return
	}
	prefix := strings.ToLower(fields[0])
	var matches []string
	for _, c := range commonCommands {
		if strings.HasPrefix(c, prefix) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 0 {
		return
	}
	sort.Strings(matches)
	lead := longestCommonPrefix(matches)
	if strings.HasPrefix(val, "/") {
		lead = "/" + lead
	}
	if len(matches) == 1 {
		lead += " "
	}
	m.ti.SetValue(lead)
	m.ti.CursorEnd()
}

// suggestions returns completion candidates for the current input,
// or nil when the input is not a completable single command word.
func (m Model) suggestions() []string {
	val := strings.TrimPrefix(m.ti.Value(), "/")
	if val == "" || strings.Contains(val, " ") {
		return nil
	}
	fields := strings.Fields(val)
	if len(fields) != 1 {
		return nil
	}
	prefix := strings.ToLower(fields[0])
	var matches []string
	for _, c := range commonCommands {
		if strings.HasPrefix(c, prefix) && c != prefix {
			matches = append(matches, c)
		}
	}
	sort.Strings(matches)
	return matches
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m Model) View() string {
	if m.width <= 0 {
		return "Loading…"
	}
	if m.screen == screenConnect {
		return m.viewConnect()
	}
	return m.viewSession()
}

func (m Model) viewConnect() string {
	var b strings.Builder
	b.WriteString(StyleHeader.Render("  "+styleBrand+" — Connect to server  ") + "\n\n")
	b.WriteString(StyleSystem.Render("Enter your RCON details  (Tab to move, Enter to continue)") + "\n\n")
	for i := range m.inputs {
		style := StyleInputBlurred
		if i == m.focusIdx && !m.connecting {
			style = StyleInputFocused
		}
		b.WriteString(style.Render(m.inputs[i].View()) + "\n")
	}
	b.WriteString("\n")
	if m.connecting {
		b.WriteString(fmt.Sprintf("%s  %s\n\n",
			m.spinner.View(),
			StyleConnecting.Render(fmt.Sprintf("Connecting to %s:%d…  (Esc to cancel)", m.cfg.Host, m.cfg.Port))))
	} else {
		btn := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorAccentDim).
			Bold(true).
			Padding(0, 2).
			Render("Connect >")
		b.WriteString(btn + "\n\n")
	}
	if m.connErr != "" {
		b.WriteString(StyleBox.Render(StyleError.Render("! "+m.connErr)) + "\n\n")
	}
	b.WriteString(StyleFooter.Render("tab navigate • enter continue • esc quit"))
	return StyleApp.Render(b.String())
}

func (m Model) viewSession() string {
	header := m.headerLine()

	// The suggestion row appears/disappears while typing, so keep the log
	// pane height in sync with it (m is a copy; vp scroll offset survives).
	sug := m.suggestionLine()
	vh := m.viewportHeight(sug != "")
	m.vp.Height = vh

	var middle string
	switch {
	case m.showHelp:
		// Modern modal: the live log is dimmed behind a floating panel.
		m.vp.Style = lipgloss.NewStyle().Faint(true)
		middle = m.helpModal(vh)
	case m.sidebarW > 0:
		middle = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.sidebar(),
			strings.Repeat(" ", sidebarGap),
			m.vp.View(),
		)
	default:
		middle = m.vp.View()
	}

	var sb strings.Builder
	if sug != "" {
		sb.WriteString(sug + "\n")
	}
	sb.WriteString(m.inputBoxStyle().Render(m.ti.View()))

	footer := m.footerLine()
	return StyleApp.Render(strings.Join([]string{header, middle, sb.String(), footer}, "\n"))
}

// headerLine renders the status bar without nesting pre-styled strings
// inside another style (which would corrupt widths/colors).
func (m Model) headerLine() string {
	left := StyleHeader.Render("  " + styleBrand + "  │  " + m.cfg.Host + ":" + strconv.Itoa(m.cfg.Port) + "  ")

	var pill string
	switch {
	case m.connecting:
		pill = StyleConnecting.Render(m.spinner.View() + " CONNECTING")
	case m.connected:
		extra := ""
		if m.lastPing > 0 {
			extra = " · " + m.lastPing.Round(time.Millisecond).String()
		}
		if m.pending > 0 {
			extra += fmt.Sprintf(" · %s %d", m.spinner.View(), m.pending)
		}
		pill = StyleConnected.Render("● CONNECTED" + extra)
	default:
		pill = StyleDisconnected.Render("● OFFLINE")
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, left, "  ", pill)
}

func (m Model) suggestionLine() string {
	matches := m.suggestions()
	if len(matches) == 0 {
		return ""
	}
	const maxShow = 8
	shown := matches
	more := ""
	if len(matches) > maxShow {
		shown = matches[:maxShow]
		more = fmt.Sprintf("  +%d more", len(matches)-maxShow)
	}
	line := "Tab ▸ " + strings.Join(shown, "  ") + more
	// Truncate to the available width (rune-aware).
	w := m.width - 4
	if w < 20 {
		return ""
	}
	runes := []rune(line)
	if len(runes) > w {
		line = string(runes[:w-1]) + "…"
	}
	return StyleSuggest.Render(line)
}

func (m Model) inputBoxStyle() lipgloss.Style {
	switch {
	case m.connecting:
		return StyleInputConnecting
	case m.connected:
		return StyleInputFocused
	default:
		return StyleInputOffline
	}
}

func (m Model) footerLine() string {
	var hints string
	switch {
	case !m.connected && !m.connecting:
		hints = "enter send • ↑↓ history • tab complete • PgUp/PgDn scroll • F1 help • ctrl+r reconnect • ctrl+c quit"
	case m.newLines > 0 && !m.follow:
		return StyleNewOutput.Render(fmt.Sprintf("↓ %d new line(s) — press end to jump to latest", m.newLines))
	case m.width < 100:
		hints = "enter send • ↑↓ history • tab complete • PgUp/PgDn scroll • F1 help"
	default:
		hints = "enter send • ↑↓ history • tab complete • PgUp/PgDn scroll • esc clear • F1 help • ctrl+l clear • ctrl+r reconnect • ctrl+d servers • ctrl+c quit"
	}
	// Hard-truncate the footer so narrow windows never wrap the layout.
	w := m.width - 2
	if w > 0 {
		if runes := []rune(hints); len(runes) > w {
			hints = string(runes[:max(w-1, 0)]) + "…"
		}
	}
	return StyleFooter.Render(hints)
}

// helpModal overlays a floating help panel onto the dimmed log content so
// the session stays visible underneath (row-by-row overlay; ANSI-safe).
func (m Model) helpModal(vh int) string {
	avail := m.width - 2
	if avail < 24 {
		return m.vp.View()
	}
	bg := m.vp.View()
	bgLines := splitNoTrailing(bg)
	if len(bgLines) > vh {
		bgLines = bgLines[:vh]
	}
	boxed := StyleBox.Render(m.helpBody(max(avail-4, 4)))
	modalLines := splitNoTrailing(boxed)
	if len(modalLines) > vh {
		modalLines = modalLines[:vh]
	}
	top := (vh - len(modalLines)) / 2
	if top < 0 {
		top = 0
	}
	for i := 0; i < len(modalLines); i++ {
		if row := top + i; row >= 0 && row < len(bgLines) {
			bgLines[row] = modalLines[i]
		}
	}
	return strings.Join(bgLines, "\n")
}

// helpBody renders the help panel rows, each padded to inner width so the
// surrounding border produces a fixed overall width.
func (m Model) helpBody(inner int) string {
	var rows []string
	row := func(s string, pad bool) {
		if pad {
			s = padVisible(s, inner)
		}
		rows = append(rows, s)
	}

	row(StyleHelpTitle.Render("mcrcon help"), true)
	row("", true)
	for _, k := range [][2]string{
		{"enter", "send command"},
		{"up / down", "command history (also ctrl+p / ctrl+n)"},
		{"tab", "complete command"},
		{"PgUp / PgDn", "scroll output (Home top, End latest)"},
		{"shift+up/down", "scroll one line"},
		{"esc", "clear input"},
		{"ctrl+u", "clear input"},
		{"ctrl+l", "clear screen"},
		{"ctrl+r", "reconnect"},
		{"ctrl+d", "disconnect → server screen"},
		{"ctrl+c", "quit"},
		{"F1", "toggle this panel (? works when input is empty)"},
	} {
		row(fmt.Sprintf("  %s  %s", StyleHelpKey.Render(fmt.Sprintf("%-16s", k[0])), k[1]), true)
	}
	row("", true)
	row(StyleHelpTitle.Render("Local commands"), true)
	for _, l := range localHelpLines() {
		row("  "+l, true)
	}
	row("", true)
	row(StyleFooter.Render("press esc / F1 to close — the console stays live underneath"), true)
	return strings.Join(rows, "\n")
}

// sidebar renders the server panel shown on wide terminals: connection
// details, a live latency sparkline, recent commands, and session stats.
func (m Model) sidebar() string {
	inner := m.sidebarW - 4
	if inner < 1 {
		inner = 1
	}
	vh := m.viewportHeight(m.suggestions() != nil)
	contentH := max(vh-2, 1)
	pad := func(s string) string { return padVisible(s, inner) }
	sep := strings.Repeat("─", inner)
	row := func(label, value string) string {
		l := StyleSidebarLabel.Render(label)
		return pad(l + StyleSidebarValue.Render(truncate(value, inner-visibleWidth(l))))
	}

	var lines []string
	lines = append(lines, pad(StyleSidebarTitle.Render("SERVER")))
	lines = append(lines, sep)
	lines = append(lines, row("Host ", m.cfg.Host))
	lines = append(lines, row("Port ", strconv.Itoa(m.cfg.Port)))

	dot, status := StyleDisconnected.Render("●"), "Disconnected"
	switch {
	case m.connecting:
		dot, status = StyleConnecting.Render(m.spinner.View()), "Connecting…"
	case m.connected:
		dot, status = StyleConnected.Render("●"), "Connected"
	}
	lines = append(lines, pad(dot+" "+StyleSidebarValue.Render(status)))

	if m.connected {
		pingLbl := StyleSidebarLabel.Render("Ping ")
		var pingVal string
		if len(m.pings) > 0 {
			pingVal = StyleSidebarValue.Render(fmt.Sprintf(" %.0fms", m.pings[len(m.pings)-1]))
		} else {
			pingVal = StyleSidebarDim.Render(" —")
		}
		sparkW := inner - visibleWidth(pingLbl) - visibleWidth(pingVal)
		lines = append(lines, pad(pingLbl+StyleSidebarDim.Render(sparkline(m.pings, max(sparkW, 1)))+pingVal))
	}
	if m.pending > 0 {
		lines = append(lines, pad(StyleSidebarDim.Render(fmt.Sprintf("%s %d in flight", m.spinner.View(), m.pending))))
	}

	lines = append(lines, sep)
	lines = append(lines, pad(StyleSidebarTitle.Render("RECENT")))
	if cnt := min(len(m.history), 6); cnt == 0 {
		lines = append(lines, pad(StyleSidebarDim.Render("— no commands yet")))
	} else {
		for i := len(m.history) - cnt; i < len(m.history); i++ {
			lines = append(lines, pad(StyleSidebarDim.Render("· "+truncate(m.history[i], inner-2))))
		}
	}

	lines = append(lines, sep)
	lines = append(lines, pad(StyleSidebarDim.Render(fmt.Sprintf("sent %d · errors %d", m.cmdCount, m.errCount))))
	if len(m.pings) > 0 {
		sum := 0.0
		for _, p := range m.pings {
			sum += p
		}
		lines = append(lines, pad(StyleSidebarDim.Render(fmt.Sprintf("avg %.0fms · n=%d", sum/float64(len(m.pings)), len(m.pings)))))
	}
	lines = append(lines, pad(StyleSidebarLabel.Render("ctrl+d servers • F1 help")))

	for len(lines) < contentH {
		lines = append(lines, "")
	}
	return StyleSidebar.Render(strings.Join(lines, "\n"))
}

func localHelpLines() []string {
	return []string{
		StyleHelpKey.Render("/help") + "        show this help in the log",
		StyleHelpKey.Render("/history") + "     show the last 20 commands",
		StyleHelpKey.Render("/clear") + "       clear the screen",
		StyleHelpKey.Render("/reconnect") + "   reconnect to the server",
		StyleHelpKey.Render("/quit") + "        quit mcrcon",
		StyleSystem.Render("(anything else goes to the server; a single leading \"/\" is stripped, e.g. /list → list)"),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// viewportHeight returns the log pane height for the current window. The
// budget is header(1) + input(3) + footer(1) + 1 safety row; the suggestion
// row (0-1) is reclaimed dynamically while it is visible.
func (m Model) viewportHeight(suggesting bool) int {
	vh := m.height - 6
	if suggesting {
		vh--
	}
	if vh < 3 {
		vh = 3
	}
	return vh
}

func (m *Model) resizeViewport() {
	vh := m.viewportHeight(m.suggestions() != nil)
	avail := m.width - 2

	// Enable the server sidebar only when both panes stay comfortably wide.
	m.sidebarW = 0
	if m.width >= minSidebarWidth {
		w := avail / 5
		if w > maxSidebarWidth {
			w = maxSidebarWidth
		}
		if w < minSidebarW {
			w = minSidebarW
		}
		if avail-w-sidebarGap >= minLogWidth {
			m.sidebarW = w
		}
	}

	vw := avail
	if m.sidebarW > 0 {
		vw = avail - m.sidebarW - sidebarGap
	}
	if vw < 10 {
		vw = 10
	}

	wasNew := m.vp.Width == 0
	if wasNew {
		m.vp = viewport.New(vw, vh)
		m.vp.MouseWheelEnabled = true
		m.vp.MouseWheelDelta = 3
	} else {
		m.vp.Width = vw
		m.vp.Height = vh
	}
	m.vp.SetContent(strings.Join(m.logs, "\n"))
	if m.follow || wasNew {
		m.vp.GotoBottom()
	}
}

func (m *Model) pushLog(line string) {
	ts := StyleTimestamp.Render(time.Now().Format("15:04:05") + " ")
	m.logs = append(m.logs, ts+line)
	if len(m.logs) > 2000 {
		m.logs = m.logs[len(m.logs)-2000:]
	}
	m.vp.SetContent(strings.Join(m.logs, "\n"))
}

// stickToBottom moves to the latest output when following, otherwise counts
// unseen lines so the footer can offer a way back.
func (m *Model) stickToBottom() {
	if m.follow {
		m.vp.GotoBottom()
		m.newLines = 0
	} else {
		m.newLines++
	}
}

func (m *Model) syncFollow() {
	m.follow = m.vp.AtBottom()
	if m.follow {
		m.newLines = 0
	}
}

// friendlyConnError turns dial/auth failures into actionable messages.
func friendlyConnError(err error) string {
	if err == nil {
		return "unknown error"
	}
	if errors.Is(err, rcon.ErrAuthFailed) {
		return "Authentication failed — check rcon.password and restart the server after changing it"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "connection refused"):
		return "Connection refused — is enable-rcon=true, is the port correct, is the server online?"
	case strings.Contains(s, "no such host"), strings.Contains(s, "not known"):
		return "Unknown host — check the hostname/IP: " + cleanText(err.Error())
	case strings.Contains(s, "timeout"), strings.Contains(s, "deadline"):
		return "Timed out — host unreachable or firewall blocking the port? Try --timeout 15"
	default:
		return cleanText(err.Error())
	}
}

// cleanText strips Minecraft §-codes and ANSI escapes for contexts where
// styling is unwanted (error messages), and normalizes line endings.
func cleanText(s string) string {
	return trimLines(rcon.StripColors(s))
}

// formatOutput prepares server output for the log: line endings normalized,
// trailing whitespace trimmed, Minecraft §-codes rendered as ANSI colors.
func formatOutput(s string) string {
	return rcon.ToANSI(trimLines(s))
}

// trimLines normalizes CRLF and trims trailing spaces per line while
// preserving intentional blank lines.
func trimLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

func isConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, rcon.ErrAuthFailed) {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, sub := range []string{"broken pipe", "connection reset", "eof", "closed", "refused", "timeout", "i/o"} {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func longestCommonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	p := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, p) && p != "" {
			p = p[:len(p)-1]
		}
	}
	return p
}

func appendCmd(hist []string, cmd string) []string {
	if cmd == "" {
		return hist
	}
	if len(hist) > 0 && hist[len(hist)-1] == cmd {
		return hist // skip consecutive dupes
	}
	return append(hist, cmd)
}

// --- history persistence ---

func defaultHistoryFile() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "mcrcon", "history")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".mcrcon_history")
	}
	return ".mcrcon_history"
}

// --- last-connection persistence ---

func defaultConnFile() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "mcrcon", "conn.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".mcrcon_conn.json")
	}
	return ".mcrcon_conn.json"
}

// savedConn persists the last host/port so the TUI can pre-fill the form.
// The password is intentionally never stored.
type savedConn struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func loadConn(path string) savedConn {
	var sc savedConn
	data, err := os.ReadFile(path)
	if err != nil {
		return sc
	}
	if json.Unmarshal(data, &sc) != nil {
		return savedConn{}
	}
	if sc.Host == "" || sc.Port < 1 || sc.Port > 65535 {
		return savedConn{}
	}
	return sc
}

func saveConn(path, host string, port int) {
	if host == "" || port < 1 || port > 65535 {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	data, err := json.MarshalIndent(savedConn{Host: host, Port: port}, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func loadHistory(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, l)
	}
	if len(out) > maxPersistedHistory {
		out = out[len(out)-maxPersistedHistory:]
	}
	return out
}

// appendHistory persists a command. Appending is cheap until the file would
// exceed the cap; past that the file is rewritten trimmed to the latest
// maxPersistedHistory lines so it can never grow unbounded.
func appendHistory(path, cmd string) {
	if path == "" || cmd == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	existing := loadHistory(path)
	if len(existing) < maxPersistedHistory {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(cmd + "\n")
		return
	}
	next := append(existing, cmd)
	next = next[len(next)-maxPersistedHistory:]
	data := []byte(strings.Join(next, "\n") + "\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func (m *Model) shutdown() {
	if m.client != nil {
		_ = m.client.Close()
	}
}
