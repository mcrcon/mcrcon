// Command mcrcon is a Minecraft RCON client with TUI, one-shot, batch-file
// and plain modes.
//
// Examples:
//
//	mcrcon -H 127.0.0.1 -P 25575 -p secret "list"
//	mcrcon -H play.example.com -p secret            # open the TUI
//	echo "list" | mcrcon -H 127.0.0.1 -p secret     # pipe / plain mode
//	mcrcon -H 127.0.0.1 -p secret -F commands.txt   # run a list of commands
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/mcrcon/mcrcon/internal/rcon"
	"github.com/mcrcon/mcrcon/internal/tui"
)

// Exit codes are part of the scripting contract:
//
//	0  success
//	1  runtime error (dial/auth/exec/connection)
//	2  usage error (bad flags, missing args, unreadable command file)
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// version is overridden at release time via:
//
//	go build -ldflags "-X main.version=v1.2.3"
var version = "dev"

// cliOpts carries resolved options shared by the non-TUI modes.
type cliOpts struct {
	host           string
	port           int
	password       string
	timeoutSeconds int
	timeout        time.Duration
	command        string // one-shot command (positional args joined)
	colorMode      string
	batch          bool // --batch: stop at the first command error
	failOnErr      bool // --fail-on-error: exit non-zero if any command errored
}

func main() {
	var (
		filePath string
		noColor  bool
		forceTUI bool
		noTUI    bool
		showVer  bool
		showHelp bool
	)
	opts := cliOpts{colorMode: "auto"}

	// Short flags (classic mcrcon style) + long flags.
	flag.StringVar(&opts.host, "H", "", "RCON host (env MCRCON_HOST)")
	flag.StringVar(&opts.host, "host", "", "RCON host (env MCRCON_HOST)")
	flag.IntVar(&opts.port, "P", 0, "RCON port (default 25575, env MCRCON_PORT)")
	flag.IntVar(&opts.port, "port", 0, "RCON port (default 25575, env MCRCON_PORT)")
	flag.StringVar(&opts.password, "p", "", "RCON password (env MCRCON_PASSWORD)")
	flag.StringVar(&opts.password, "password", "", "RCON password (env MCRCON_PASSWORD)")
	flag.IntVar(&opts.timeoutSeconds, "timeout", 8, "connection & command timeout in seconds")
	flag.StringVar(&filePath, "F", "", "run commands from a file, one per line")
	flag.StringVar(&filePath, "command-file", "", "run commands from a file, one per line")
	flag.StringVar(&opts.colorMode, "color", "auto", "Minecraft colors: auto, always, never")
	flag.BoolVar(&noColor, "no-color", false, "alias for --color never (wins if both are set)")
	flag.BoolVar(&opts.batch, "batch", false, "stop at the first failed command (fail-fast)")
	flag.BoolVar(&opts.failOnErr, "fail-on-error", false, "exit non-zero if any command failed")
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
		os.Exit(exitOK)
	}
	if showVer {
		fmt.Printf("mcrcon %s (go TUI)\n", version)
		os.Exit(exitOK)
	}

	// --no-color is shorthand for --color never; it wins when both are set.
	if noColor {
		opts.colorMode = "never"
	}

	// Env fallback (flag > env > default).
	if opts.host == "" {
		opts.host = firstEnv("MCRCON_HOST", "RCON_HOST", "MINECRAFT_HOST")
	}
	if opts.port == 0 {
		if s := firstEnv("MCRCON_PORT", "RCON_PORT"); s != "" {
			if p, err := strconv.Atoi(s); err == nil {
				opts.port = p
			}
		}
		if opts.port == 0 {
			opts.port = 25575
		}
	}
	if opts.password == "" {
		opts.password = firstEnv("MCRCON_PASSWORD", "RCON_PASSWORD", "MCRCON_PASS")
	}
	opts.timeout = time.Duration(opts.timeoutSeconds) * time.Second
	if opts.timeout <= 0 {
		opts.timeout = 8 * time.Second
	}
	switch opts.colorMode {
	case "auto", "always", "never":
	default:
		errc(os.Stderr, "invalid --color %q: use auto, always, or never", opts.colorMode)
		os.Exit(exitUsage)
	}

	args := flag.Args()

	// --- command-file mode: mcrcon -F file ---
	if filePath != "" {
		if len(args) > 0 {
			errc(os.Stderr, "cannot combine a positional command with --command-file")
			os.Exit(exitUsage)
		}
		os.Exit(runCommandFile(opts, filePath, os.Stdout, os.Stderr))
	}

	// --- one-shot mode: mcrcon -H host -p pass "list" ---
	if len(args) > 0 {
		opts.command = strings.Join(args, " ")
		os.Exit(runOneShot(opts, os.Stdout, os.Stderr))
	}

	// --- no command args: TUI vs plain ---
	useTUI := forceTUI || (!noTUI && isTTY(os.Stdin) && isTTY(os.Stdout))
	if !useTUI {
		os.Exit(runPlain(opts, os.Stdin, os.Stdout, os.Stderr))
	}

	cfg := tui.Config{
		Host:     opts.host,
		Port:     opts.port,
		Password: opts.password,
		Timeout:  opts.timeout,
	}
	prog := tea.NewProgram(tui.New(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := prog.Run(); err != nil {
		fatal("TUI error: %v", err)
	}
	os.Exit(exitOK)
}

// runOneShot executes a single command and prints its response.
func runOneShot(o cliOpts, stdout, stderr io.Writer) int {
	if o.host == "" {
		errc(stderr, "missing host: use -H <host> or env MCRCON_HOST")
		return exitUsage
	}
	if o.password == "" {
		errc(stderr, "missing password: use -p <password> or env MCRCON_PASSWORD")
		return exitUsage
	}
	c, err := rcon.Dial(addr(o), o.password, o.timeout)
	if err != nil {
		errc(stderr, "connect failed: %v", err)
		return exitError
	}
	defer c.Close()

	out, err := c.Execute(o.command)
	if err != nil {
		errc(stderr, "command failed: %v", err)
		return exitError
	}
	printResult(stdout, renderOutput(out, colorEnabled(o.colorMode)))
	return exitOK
}

// runCommandFile reads commands from a file (one per line) and executes them
// in order. With --batch it stops at the first failure; otherwise it runs
// them all and reports each failure. The exit code is non-zero if any
// command failed.
func runCommandFile(o cliOpts, path string, stdout, stderr io.Writer) int {
	if o.host == "" {
		errc(stderr, "missing host: use -H <host> or env MCRCON_HOST")
		return exitUsage
	}
	if o.password == "" {
		errc(stderr, "missing password: use -p <password> or env MCRCON_PASSWORD")
		return exitUsage
	}
	cmds, err := readCommandFile(path)
	if err != nil {
		errc(stderr, "%v", err)
		return exitUsage
	}

	c, err := rcon.Dial(addr(o), o.password, o.timeout)
	if err != nil {
		errc(stderr, "connect failed: %v", err)
		return exitError
	}
	defer c.Close()

	failed := false
	for _, cmd := range cmds {
		out, err := c.Execute(cmd)
		if err != nil {
			errc(stderr, "%s: %v", cmd, err)
			failed = true
			if o.batch {
				return exitError
			}
			continue
		}
		printResult(stdout, renderOutput(out, colorEnabled(o.colorMode)))
	}
	if failed {
		return exitError
	}
	return exitOK
}

// runPlain is a dumb-terminal REPL: read lines from stdin, exec, print.
// Used for pipes/scripts and when no TTY is available. With --batch it
// stops at the first error; with --fail-on-error it exits non-zero on EOF
// if any command failed (default exit is 0).
func runPlain(o cliOpts, stdin io.Reader, stdout, stderr io.Writer) int {
	if o.host == "" {
		errc(stderr, "missing host: use -H <host> or env MCRCON_HOST")
		return exitUsage
	}
	if o.password == "" {
		errc(stderr, "missing password: use -p <password> or env MCRCON_PASSWORD")
		return exitUsage
	}
	c, err := rcon.Dial(addr(o), o.password, o.timeout)
	if err != nil {
		errc(stderr, "connect failed: %v", err)
		return exitError
	}
	defer c.Close()

	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	interactive := isTerminalReader(stdin)
	failed := false
	for {
		if interactive {
			fmt.Fprint(stdout, "> ")
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
				if failed && o.failOnErr {
					return exitError
				}
				return exitOK
			}
		case "/clear":
			continue
		}
		cmd := strings.TrimPrefix(line, "/")
		out, err := c.Execute(cmd)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s: %v\n", cmd, err)
			failed = true
			// Exit on hard failures (auth/connection); otherwise keep the
			// session alive unless --batch asked for fail-fast.
			if isFatalRCON(err) || o.batch {
				return exitError
			}
			continue
		}
		if out != "" {
			fmt.Fprintln(stdout, renderOutput(out, colorEnabled(o.colorMode)))
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(stderr, "error reading stdin: %v\n", err)
	}
	if failed && o.failOnErr {
		return exitError
	}
	return exitOK
}

// readCommandFile parses a command file: one command per line. Blank lines
// and lines starting with "#" are skipped, a single leading "/" is stripped,
// and trailing whitespace is trimmed.
func readCommandFile(path string) ([]string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read command file: %w", err)
	}
	var cmds []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cmds = append(cmds, strings.TrimPrefix(line, "/"))
	}
	return cmds, nil
}

func addr(o cliOpts) string {
	return fmt.Sprintf("%s:%d", o.host, o.port)
}

// printResult writes command output, ensuring a trailing newline so console
// output is never glued to the shell prompt.
func printResult(w io.Writer, out string) {
	if out == "" {
		return
	}
	fmt.Fprint(w, out)
	if !strings.HasSuffix(out, "\n") {
		fmt.Fprintln(w)
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

// colorEnabled resolves --color=auto|always|never. Auto mode emits ANSI
// colors only when stdout is a TTY, and honors the NO_COLOR convention.
func colorEnabled(mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // auto
		return isTTY(os.Stdout) && os.Getenv("NO_COLOR") == ""
	}
}

// isTerminalReader reports whether r is an interactive terminal (used to
// decide whether to print the "> " prompt in plain mode).
func isTerminalReader(r io.Reader) bool {
	if f, ok := r.(*os.File); ok {
		return isTTY(f)
	}
	return false
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

func errc(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, "mcrcon: "+format+"\n", a...)
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "mcrcon: "+format+"\n", a...)
	os.Exit(exitError)
}

func usage() {
	fmt.Fprintf(os.Stderr, `mcrcon %s — Minecraft RCON client (Go + TUI)

Usage:
  mcrcon -H <host> -P <port> -p <password> [command...]
  mcrcon -H <host> -p <password> -F <file>          # run commands from a file
  mcrcon -H <host> -p <password>                    # open the TUI
  mcrcon --no-tui -H <host> -p <password>           # plain REPL (pipe-friendly)

Options:
  -H, --host <host>       RCON host (env MCRCON_HOST) [default 127.0.0.1 in TUI]
  -P, --port <port>       RCON port (env MCRCON_PORT) [default 25575]
  -p, --password <pass>   RCON password (env MCRCON_PASSWORD / RCON_PASSWORD)
      --timeout <secs>    connection & command timeout [default 8]
  -F, --command-file <f>  run commands from a file, one per line
                          ("#" comments and blank lines are skipped)
      --batch             stop at the first failed command (fail-fast)
      --fail-on-error     exit non-zero if any command failed (plain mode)
      --color <mode>      Minecraft colors: auto, always, never [default auto]
                          (auto = colored on TTY, plain when piped; honors NO_COLOR)
      --no-color          alias for --color never
  -t, --tui               force TUI mode
      --no-tui            force plain mode (no TUI)
  -h, --help              print this help
  -v, --version           print version

Modes:
  one-shot     pass a command as arguments → execute once, print, exit.
               example: mcrcon -H 127.0.0.1 -p secret "say hello from rcon"
  command-file run every line of a file as a separate command, in order.
               example: mcrcon -H 127.0.0.1 -p secret -F ops.txt
  TUI          no command args + a TTY → fullscreen interface:
               connect form + console (history, tab-completion, /help).
  plain        no TTY / --no-tui → read lines from stdin, write replies to stdout.
               example: echo "list" | mcrcon -H 127.0.0.1 -p secret --no-tui

Exit codes:
  0  success                 1  runtime error   2  usage error

Local TUI commands (start with "/"):
  /help        help • /history recent commands • /clear clear • /reconnect • /quit quit
  ("/list" is sent as "list" automatically.)

Example server.properties:
  enable-rcon=true / rcon.port=25575 / rcon.password=secret
`, version)
}
