package rcon

import (
	"regexp"
	"strings"
)

// ANSI SGR fragments for Minecraft formatting codes.
const (
	ansiReset         = "\x1b[0m"
	ansiBold          = "\x1b[1m"
	ansiStrikethrough = "\x1b[9m"
	ansiUnderline     = "\x1b[4m"
	ansiItalic        = "\x1b[3m"
)

// sectionSign is the Minecraft formatting prefix (U+00A7, 2 bytes in UTF-8).
const sectionSign = "§"

// ansiSGR matches ANSI SGR color/style sequences (safe to strip or keep;
// SGR cannot move the cursor or otherwise manipulate the terminal).
var ansiSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

// mcForeground maps Minecraft color codes (§0-9, §a-f) to ANSI SGR
// foreground sequences (standard + bright, works on any terminal).
var mcForeground = map[byte]string{
	'0': "\x1b[30m", // black
	'1': "\x1b[34m", // dark blue
	'2': "\x1b[32m", // dark green
	'3': "\x1b[36m", // dark aqua
	'4': "\x1b[31m", // dark red
	'5': "\x1b[35m", // dark purple
	'6': "\x1b[33m", // gold
	'7': "\x1b[37m", // gray
	'8': "\x1b[90m", // dark gray
	'9': "\x1b[94m", // blue
	'a': "\x1b[92m", // green
	'b': "\x1b[96m", // aqua
	'c': "\x1b[91m", // red
	'd': "\x1b[95m", // light purple
	'e': "\x1b[93m", // yellow
	'f': "\x1b[97m", // white
}

// splitCode returns the formatting code following a § at s[i:], and how many
// bytes to consume. ok is false for unknown codes, a trailing lone §, or
// when s[i:] doesn't start with § at all.
func splitCode(s string, i int) (code byte, consume int, ok bool) {
	rest := s[i:]
	if !strings.HasPrefix(rest, sectionSign) || len(rest) < len(sectionSign)+1 {
		return 0, 0, false
	}
	c := rest[len(sectionSign)]
	code, ok = normalizeCode(c)
	if !ok {
		return 0, 0, false
	}
	return code, len(sectionSign) + 1, true
}

// normalizeCode lowercases a code char; ok is false for unknown codes.
func normalizeCode(c byte) (byte, bool) {
	if c >= 'A' && c <= 'Z' {
		c += 'a' - 'A'
	}
	switch {
	case c >= '0' && c <= '9',
		c >= 'a' && c <= 'f',
		c == 'k', c == 'l', c == 'm',
		c == 'n', c == 'o', c == 'r':
		return c, true
	}
	return 0, false
}

// StripColors removes Minecraft §-codes and ANSI SGR escape sequences,
// returning plain readable text. Unknown §-sequences are kept literally so
// no user content is ever lost.
func StripColors(s string) string {
	s = ansiSGR.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if _, consume, ok := splitCode(s, i); ok {
			i += consume
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// ToANSI converts Minecraft §-codes to ANSI SGR escape sequences so server
// output (e.g. "§aHello §cWorld") renders in color on a terminal.
// Behavior mirrors the game where practical:
//
//   - a color code (§0-9, §a-f) resets active formatting, then sets the color
//   - formatting codes stack: §l bold, §m strikethrough, §n underline,
//     §o italic (§k obfuscated has no terminal equivalent and is skipped)
//   - §r resets everything
//   - unknown sequences (e.g. §x) and a trailing lone § are kept literally
//   - existing ANSI SGR escapes pass through untouched
//   - a final reset is appended only when styling was emitted, so plain
//     input returns byte-identical output
func ToANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	styled := false
	for i := 0; i < len(s); {
		code, consume, ok := splitCode(s, i)
		if !ok {
			b.WriteByte(s[i])
			i++
			continue
		}
		i += consume
		switch {
		case code == 'r':
			b.WriteString(ansiReset)
			styled = true
		case code == 'k':
			// Obfuscated: no terminal equivalent; skip, keep text readable.
		case code == 'l':
			b.WriteString(ansiBold)
			styled = true
		case code == 'm':
			b.WriteString(ansiStrikethrough)
			styled = true
		case code == 'n':
			b.WriteString(ansiUnderline)
			styled = true
		case code == 'o':
			b.WriteString(ansiItalic)
			styled = true
		default:
			// Color code: reset formatting first, like the game does.
			b.WriteString(ansiReset + mcForeground[code])
			styled = true
		}
	}
	if styled {
		b.WriteString(ansiReset)
	}
	return b.String()
}
