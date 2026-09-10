// Command mcrcon is a Minecraft RCON client with TUI, one-shot and plain modes.
//
// Examples:
//
//	mcrcon -H 127.0.0.1 -P 25575 -p secret "list"
//	mcrcon -H play.example.com -p secret            # open the TUI
//	echo "list" | mcrcon -H 127.0.0.1 -p secret     # pipe / plain mode
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/mcrcon/mcrcon/internal/rcon"
	"github.com/mcrcon/mcrcon/internal/tui"
)

// version is overridden at release time via:
//
//	go build -ldflags "-X main.version=v1.2.3"
var version = "dev"

func main() {
	var (
		host      string
		port      int
		password  string
		timeoutS  int
		colorMode string
		forceTUI  bool
		noTUI     bool
		showVer   bool
		showHelp  bool
	)

	// Short flags (classic mcrcon style) + long flags.
	flag.StringVar(&host, "H", "", "RCON host (env MCRCON_HOST)")
	flag.StringVar(&host, "host", "", "RCON host (env MCRCON_HOST)")
	flag.IntVar(&port, "P", 0, "RCON port (default 25575, env MCRCON_PORT)")
	flag.IntVar(&port, "port", 0, "RCON port (default 25575, env MCRCON_PORT)")
	flag.StringVar(&password, "p", "", "RCON password (env MCRCON_PASSWORD)")
	flag.StringVar(&password, "password", "", "RCON password (env MCRCON_PASSWORD)")
	flag.IntVar(&timeoutS, "timeout", 8, "connection & command timeout in seconds")
	flag.StringVar(&colorMode, "color", "auto", "Minecraft color output: auto, always, never")
	flag.BoolVar(&forceTUI, "t", false, "force interactive TUI mode")
	flag.BoolVar(&forceTUI, "tui", false, "force interactive TUI mode")
	flag.BoolVar(&noTUI, "no-tui", false, "disable the TUI (plain/pipe mode)")
	flag.BoolVar(&showVer, "v", false, "print version")
	flag.BoolVar(&showVer, "version", false, "print version")
	flag.BoolVar(&showHelp, "h", false, "print help")
	flag.BoolVar(&showHelp, "help", false, "print help")
	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		os.Exit(0)
	}
	if showVer {
		fmt.Printf("mcrcon %s (go TUI)\n", version)
		os.Exit(0)
	}

	// Env fallback (flag > env > default).
	if host == "" {
		host = firstEnv("MCRCON_HOST", "RCON_HOST", "MINECRAFT_HOST")
	}
	if port == 0 {
		if s := firstEnv("MCRCON_PORT", "RCON_PORT"); s != "" {
			if p, err := strconv.Atoi(s); err == nil {
				port = p
			}
		}
		if port == 0 {
			port = 25575
		}
	}
	if password == "" {
		password = firstEnv("MCRCON_PASSWORD", "RCON_PASSWORD", "MCRCON_PASS")
	}
	timeout := time.Duration(timeoutS) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	switch colorMode {
	case "auto", "always", "never":
	default:
		fatal("invalid --color %q: use auto, always, or never", colorMode)
	}
	color := useColor(colorMode)

	args := flag.Args()

	// --- one-shot mode: mcrcon -H host -p pass "list" ---
	if len(args) > 0 {
		if host == "" {
			fatal("missing host: use -H <host> or env MCRCON_HOST")
		}
		if password == "" {
			fatal("missing password: use -p <password> or env MCRCON_PASSWORD")
		}
		cmd := strings.Join(args, " ")
		addr := fmt.Sprintf("%s:%d", host, port)
		c, err := rcon.Dial(addr, password, timeout)
		if err != nil {
			fatal("connect failed: %v", err)
		}
		defer c.Close()
		out, err := c.Execute(cmd)
		if err != nil {
			fatal("command failed: %v", err)
		}
		out = renderOutput(out, color)
		fmt.Print(out)
		if out != "" && !strings.HasSuffix(out, "\n") {
			fmt.Println()
		}
		return
	}

	// --- no command args: TUI vs plain ---
	useTUI := forceTUI || (!noTUI && isTTY(os.Stdin) && isTTY(os.Stdout))
	if !useTUI {
		runPlain(host, port, password, timeout, color)
		return
	}

	cfg := tui.Config{
		Host:     host,
		Port:     port,
		Password: password,
		Timeout:  timeout,
	}
	prog := tea.NewProgram(tui.New(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := prog.Run(); err != nil {
		fatal("TUI error: %v", err)
	}
}

// runPlain is a dumb-terminal REPL: read lines from stdin, exec, print.
// Used for pipes/scripts and when no TTY is available.
func runPlain(host string, port int, password string, timeout time.Duration, color bool) {
	if host == "" {
		fatal("missing host: use -H <host> or env MCRCON_HOST")
	}
	if password == "" {
		fatal("missing password: use -p <password> or env MCRCON_PASSWORD")
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	c, err := rcon.Dial(addr, password, timeout)
	if err != nil {
		fatal("connect failed: %v", err)
	}
	defer c.Close()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	interactive := isTTY(os.Stdin)
	for {
		if interactive {
			fmt.Print("> ")
		}
		if !sc.Scan() {
			break
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Local conveniences in plain mode.
		switch strings.ToLower(strings.Fields(line)[0]) {
		case "/quit", "/exit", "quit", "exit":
			if strings.HasPrefix(line, "/") || line == "quit" || line == "exit" {
				return
			}
		case "/clear":
			continue
		}
		cmd := strings.TrimPrefix(line, "/")
		out, err := c.Execute(cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			// Keep the session alive for transient command errors; exit
			// only on auth/connection failures.
			if isFatalRCON(err) {
				os.Exit(1)
			}
			continue
		}
		if out != "" {
			fmt.Println(renderOutput(out, color))
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
	}
}

func isFatalRCON(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "auth") || strings.Contains(s, "closed") ||
		strings.Contains(s, "refused") || strings.Contains(s, "broken pipe")
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func isTTY(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// useColor resolves --color=auto|always|never. Auto mode emits ANSI colors
// only when stdout is a TTY, and honors the NO_COLOR convention.
func useColor(mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // auto
		return isTTY(os.Stdout) && os.Getenv("NO_COLOR") == ""
	}
}

// renderOutput formats server output: Minecraft §-codes become ANSI colors
// when color is on, otherwise everything is stripped to plain text (safe
// for pipes, logs, and scripts).
func renderOutput(out string, color bool) string {
	if color {
		return rcon.ToANSI(out)
	}
	return rcon.StripColors(out)
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "mcrcon: "+format+"\n", a...)
	os.Exit(1)
}

func usage() {
	fmt.Fprintf(os.Stderr, `mcrcon %s — Minecraft RCON client (Go + TUI)

Usage:
  mcrcon -H <host> -P <port> -p <password> [command...]
  mcrcon -H <host> -p <password>                 # open the TUI
  mcrcon --no-tui -H <host> -p <password>        # plain REPL (pipe-friendly)

Options:
  -H, --host <host>       RCON host (env MCRCON_HOST) [default 127.0.0.1 in TUI]
  -P, --port <port>       RCON port (env MCRCON_PORT) [default 25575]
  -p, --password <pass>   RCON password (env MCRCON_PASSWORD / RCON_PASSWORD)
      --timeout <secs>    connection & command timeout [default 8]
      --color <mode>      Minecraft colors: auto, always, never [default auto]
                          (auto = colored on TTY, plain when piped; honors NO_COLOR)
  -t, --tui               force TUI mode
      --no-tui            force plain mode (no TUI)
  -h, --help              print this help
  -v, --version           print version

Modes:
  one-shot   pass a command as arguments → execute once, print, exit.
             example: mcrcon -H 127.0.0.1 -p secret "say hello from rcon"
  TUI        no command args + a TTY → fullscreen interface:
             connect form + console (history, tab-completion, /help).
  plain      no TTY / --no-tui → read lines from stdin, write replies to stdout.
             example: echo "list" | mcrcon -H 127.0.0.1 -p secret --no-tui

Local TUI commands (start with "/"):
  /help        help • /history recent commands • /clear clear • /reconnect • /quit quit
  ("/list" is sent as "list" automatically.)

Example server.properties:
  enable-rcon=true / rcon.port=25575 / rcon.password=secret
`, version)
}
