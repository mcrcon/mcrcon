package rcon

import (
	"strings"
	"testing"
)

func TestToANSIPlainUnchanged(t *testing.T) {
	in := "There are 2 players online: Steve, Alex"
	if got := ToANSI(in); got != in {
		t.Fatalf("plain input must be byte-identical, got %q", got)
	}
}

func TestToANSIColors(t *testing.T) {
	got := ToANSI("§aHello §cWorld")
	want := "\x1b[0m\x1b[92mHello \x1b[0m\x1b[91mWorld\x1b[0m"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestToANSIUppercaseAndReset(t *testing.T) {
	got := ToANSI("§AX§rY")
	want := "\x1b[0m\x1b[92mX\x1b[0mY\x1b[0m"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestToANSIStackedFormatting(t *testing.T) {
	got := ToANSI("§l§nBold+Underline")
	if !strings.HasPrefix(got, "\x1b[1m\x1b[4m") {
		t.Fatalf("expected stacked bold+underline, got %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("expected trailing reset, got %q", got)
	}
}

func TestToANSIColorResetsFormatting(t *testing.T) {
	// Like the game: a new color clears earlier bold.
	got := ToANSI("§lBold §aGreen")
	if !strings.Contains(got, "\x1b[0m\x1b[92mGreen") {
		t.Fatalf("expected color to reset bold first, got %q", got)
	}
}

func TestToANSIObfuscatedSkipped(t *testing.T) {
	got := ToANSI("§ksecret")
	if got != "secret" {
		t.Fatalf("obfuscated text should stay readable, got %q", got)
	}
}

func TestToANSIUnknownKeptLiteral(t *testing.T) {
	in := "100§% sure §x"
	got := ToANSI(in)
	if got != in {
		t.Fatalf("unknown sequences must be literal, got %q want %q", got, in)
	}
}

func TestToANSITrailingSectionKept(t *testing.T) {
	if got := ToANSI("oops§"); got != "oops§" {
		t.Fatalf("got %q", got)
	}
}

func TestToANSIPassthroughExistingANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m"
	if got := ToANSI(in); got != in {
		t.Fatalf("existing ANSI must pass through, got %q", got)
	}
}

func TestStripColors(t *testing.T) {
	tests := map[string]string{
		"§aHello §cWorld":      "Hello World",
		"§lBold§r plain":       "Bold plain",
		"\x1b[31mred\x1b[0m":   "red",
		"§ksecret":             "secret",
		"no codes here":        "no codes here",
		"100§% sure":           "100§% sure", // unknown kept
		"trailing§":            "trailing§",
		"§Amixed §Bcase":       "mixed case",
		"multi\nline §bsecond": "multi\nline second",
	}
	for in, want := range tests {
		if got := StripColors(in); got != want {
			t.Errorf("StripColors(%q) = %q, want %q", in, got, want)
		}
	}
}
