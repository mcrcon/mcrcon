package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcrcon/mcrcon/internal/rcon"
)

const testPw = "secret"

// recorder is a concurrency-safe collector for commands the fake server saw.
type recorder struct {
	mu sync.Mutex
	cm []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cm = append(r.cm, s)
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.cm...)
}

// fakeServer is a minimal loopback RCON server. handler maps each command to
// its response. When handler returns drop=true the connection is closed
// instead of answering (simulates a transport error mid-script).
type fakeServer struct {
	addr    string
	handler func(cmd string) (resp string, drop bool)
}

func startFakeServer(t *testing.T, handler func(cmd string) (string, bool)) fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				for {
					pkt, err := rcon.ReadPacket(c)
					if err != nil {
						return
					}
					if pkt.Type == rcon.TypeAuth {
						if pkt.Body == testPw {
							_ = rcon.Packet{ID: pkt.ID, Type: rcon.TypeAuthResponse, Body: ""}.Write(c)
						} else {
							_ = rcon.Packet{ID: -1, Type: rcon.TypeAuthResponse, Body: ""}.Write(c)
							return
						}
						continue
					}
					resp, drop := handler(pkt.Body)
					if drop {
						return
					}
					_ = rcon.Packet{ID: pkt.ID, Type: rcon.TypeResponseValue, Body: resp}.Write(c)
				}
			}(conn)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return fakeServer{addr: ln.Addr().String()}
}

func echoHandler(cmd string) (string, bool) { return "ok: " + cmd, false }

func cliOptsFor(s fakeServer, mut ...func(*cliOpts)) cliOpts {
	o := cliOpts{
		host:      "127.0.0.1",
		port:      mustPort(s.addr),
		password:  testPw,
		timeout:   3 * time.Second,
		colorMode: "never",
		command:   "list",
	}
	for _, m := range mut {
		m(&o)
	}
	return o
}

func mustPort(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return p
}

func TestRunOneShotSuccess(t *testing.T) {
	s := startFakeServer(t, echoHandler)
	var out, errB bytes.Buffer
	rc := runOneShot(cliOptsFor(s), &out, &errB)
	if rc != exitOK {
		t.Fatalf("exit=%d stderr=%q", rc, errB.String())
	}
	if got, want := out.String(), "ok: list\n"; got != want {
		t.Fatalf("stdout=%q want %q", got, want)
	}
}

func TestRunOneShotAuthFailure(t *testing.T) {
	s := startFakeServer(t, echoHandler)
	o := cliOptsFor(s, func(o *cliOpts) { o.password = "wrong" })
	var out, errB bytes.Buffer
	rc := runOneShot(o, &out, &errB)
	if rc != exitError {
		t.Fatalf("exit=%d want 1, stderr=%q", rc, errB.String())
	}
	if !strings.Contains(errB.String(), "auth") {
		t.Fatalf("expected auth error, got %q", errB.String())
	}
}

func TestRunOneShotMissingHostIsUsageError(t *testing.T) {
	s := startFakeServer(t, echoHandler)
	o := cliOptsFor(s, func(o *cliOpts) { o.host = "" })
	var out, errB bytes.Buffer
	if rc := runOneShot(o, &out, &errB); rc != exitUsage {
		t.Fatalf("exit=%d want 2, stderr=%q", rc, errB.String())
	}
}

func TestRunOneShotRendersColor(t *testing.T) {
	s := startFakeServer(t, func(cmd string) (string, bool) {
		return "hello §aWORLD", false
	})
	o := cliOptsFor(s, func(o *cliOpts) { o.colorMode = "always" })
	var out, errB bytes.Buffer
	if rc := runOneShot(o, &out, &errB); rc != exitOK {
		t.Fatalf("exit=%d stderr=%q", rc, errB.String())
	}
	if !strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected ANSI colors in output %q", out.String())
	}
}

func TestReadCommandFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ops.txt")
	content := strings.Join([]string{
		"# maintenance window",
		"",
		"list",
		"say hello from rcon",
		"/time set day",
		"  broadcast done ",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds, err := readCommandFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"list", "say hello from rcon", "time set day", "broadcast done"}
	if strings.Join(cmds, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q want %q", cmds, want)
	}
}

func TestReadCommandFileMissing(t *testing.T) {
	if _, err := readCommandFile(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRunCommandFileRunsAll(t *testing.T) {
	var rec recorder
	s := startFakeServer(t, func(cmd string) (string, bool) {
		rec.add(cmd)
		return "ok: " + cmd, false
	})
	path := filepath.Join(t.TempDir(), "c.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errB bytes.Buffer
	rc := runCommandFile(cliOptsFor(s), path, &out, &errB)
	if rc != exitOK {
		t.Fatalf("exit=%d stderr=%q", rc, errB.String())
	}
	if strings.Join(rec.all(), ",") != "one,two,three" {
		t.Fatalf("executed %q", rec.all())
	}
	if strings.Join(strings.Fields(out.String()), " ") != "ok: one ok: two ok: three" {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestRunCommandFileContinuesOnError(t *testing.T) {
	var rec recorder
	s := startFakeServer(t, func(cmd string) (string, bool) {
		rec.add(cmd)
		if cmd == "boom" {
			return "", true // drop connection mid-script
		}
		return "ok: " + cmd, false
	})
	o := cliOptsFor(s, func(o *cliOpts) {
		// Server refuses new connections after the first drop; reuse the
		// same client like runCommandFile does makes this deterministic.
	})
	// Note: subsequent commands on a closed conn error immediately; the
	// point here is that we still attempt them and end non-zero.
	path := filepath.Join(t.TempDir(), "c.txt")
	if err := os.WriteFile(path, []byte("one\nboom\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errB bytes.Buffer
	rc := runCommandFile(o, path, &out, &errB)
	if rc != exitError {
		t.Fatalf("exit=%d want 1, stderr=%q", rc, errB.String())
	}
	if !strings.Contains(out.String(), "ok: one") {
		t.Fatalf("expected first command output, got %q", out.String())
	}
}

func TestRunCommandFileBatchStopsAtFirstError(t *testing.T) {
	var rec recorder
	s := startFakeServer(t, func(cmd string) (string, bool) {
		rec.add(cmd)
		if cmd == "boom" {
			return "", true
		}
		return "ok: " + cmd, false
	})
	o := cliOptsFor(s, func(o *cliOpts) { o.batch = true })
	path := filepath.Join(t.TempDir(), "c.txt")
	if err := os.WriteFile(path, []byte("one\nboom\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errB bytes.Buffer
	rc := runCommandFile(o, path, &out, &errB)
	if rc != exitError {
		t.Fatalf("exit=%d want 1, stderr=%q", rc, errB.String())
	}
	if strings.Join(rec.all(), ",") != "one,boom" {
		t.Fatalf("batch must stop after boom, executed %q", rec.all())
	}
}

func TestRunPlainPipesCommands(t *testing.T) {
	s := startFakeServer(t, echoHandler)
	var stdin bytes.Buffer
	stdin.WriteString("list\nsay hello\n")
	var out, errB bytes.Buffer
	rc := runPlain(cliOptsFor(s), &stdin, &out, &errB)
	if rc != exitOK {
		t.Fatalf("exit=%d stderr=%q", rc, errB.String())
	}
	if !strings.Contains(out.String(), "ok: list") || !strings.Contains(out.String(), "ok: say hello") {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestRunPlainFailOnError(t *testing.T) {
	s := startFakeServer(t, func(cmd string) (string, bool) {
		if cmd == "boom" {
			return "", true
		}
		return "ok: " + cmd, false
	})
	var stdin bytes.Buffer
	stdin.WriteString("okcmd\nboom\n")
	var out, errB bytes.Buffer
	o := cliOptsFor(s, func(o *cliOpts) { o.failOnErr = true })
	rc := runPlain(o, &stdin, &out, &errB)
	if rc != exitError {
		t.Fatalf("exit=%d want 1, stderr=%q", rc, errB.String())
	}
	if !strings.Contains(errB.String(), "boom") {
		t.Fatalf("expected error mentioning the failing command, got %q", errB.String())
	}
}

func TestRenderOutputColorSwitch(t *testing.T) {
	s := startFakeServer(t, echoHandler)
	_ = s
	if plain := renderOutput("§aX", false); plain != "X" {
		t.Fatalf("plain render got %q", plain)
	}
	if colored := renderOutput("§aX", true); !strings.Contains(colored, "\x1b[") {
		t.Fatalf("colored render got %q", colored)
	}
}

func TestColorEnabledModes(t *testing.T) {
	if !colorEnabled("always") {
		t.Fatal("always must be true")
	}
	if colorEnabled("never") {
		t.Fatal("never must be false")
	}
	t.Setenv("NO_COLOR", "1")
	if colorEnabled("auto") {
		t.Fatal("auto must be false when NO_COLOR is set (stdout not a TTY in tests)")
	}
}

func TestPrintResultAddsMissingNewline(t *testing.T) {
	var b bytes.Buffer
	printResult(&b, "no-newline")
	if got := b.String(); got != "no-newline\n" {
		t.Fatalf("got %q", got)
	}
	b.Reset()
	printResult(&b, "")
	if b.Len() != 0 {
		t.Fatalf("empty output should print nothing, got %q", b.String())
	}
}
