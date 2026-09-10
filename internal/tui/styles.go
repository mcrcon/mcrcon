package tui

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// palette holds every color used by the UI so the light and dark themes can
// be switched wholesale. Adaptive mode is selected at startup (env
// MCRCON_THEME=auto|dark|light, default auto) by probing the terminal
// background with lipgloss.HasDarkBackground.
type palette struct {
	accent      lipgloss.Color
	accentHi    lipgloss.Color // bright end of the accent gradient
	accentDim   lipgloss.Color
	danger      lipgloss.Color
	warning     lipgloss.Color
	muted       lipgloss.Color
	border      lipgloss.Color
	text        lipgloss.Color
	dim         lipgloss.Color
	helpKey     lipgloss.Color
	timestamp   lipgloss.Color
	suggest     lipgloss.Color
	headerFg    lipgloss.Color
	headerBg    lipgloss.Color
	surface     lipgloss.Color // sidebar/panel fill
	surfaceText lipgloss.Color
}

var darkPalette = palette{
	accent:      lipgloss.Color("#5EBB2B"),
	accentHi:    lipgloss.Color("#A6F270"),
	accentDim:   lipgloss.Color("#3E7A1E"),
	danger:      lipgloss.Color("#E5484D"),
	warning:     lipgloss.Color("#E5A90B"),
	muted:       lipgloss.Color("#8B8B8B"),
	border:      lipgloss.Color("#3A3A3A"),
	text:        lipgloss.Color("#E8E8E8"),
	dim:         lipgloss.Color("#9AA0A6"),
	helpKey:     lipgloss.Color("#7DD3FC"),
	timestamp:   lipgloss.Color("#5A5A5A"),
	suggest:     lipgloss.Color("#9A9A9A"),
	headerFg:    lipgloss.Color("#FFFFFF"),
	headerBg:    lipgloss.Color("#24292E"),
	surface:     lipgloss.Color("#1C1F24"),
	surfaceText: lipgloss.Color("#C9C9C9"),
}

var lightPalette = palette{
	accent:      lipgloss.Color("#1F8A44"),
	accentHi:    lipgloss.Color("#52D97F"),
	accentDim:   lipgloss.Color("#5BB574"),
	danger:      lipgloss.Color("#C1121F"),
	warning:     lipgloss.Color("#B45309"),
	muted:       lipgloss.Color("#5B6472"),
	border:      lipgloss.Color("#CBD2DC"),
	text:        lipgloss.Color("#1F2937"),
	dim:         lipgloss.Color("#4B5563"),
	helpKey:     lipgloss.Color("#0E7490"),
	timestamp:   lipgloss.Color("#99A1AF"),
	suggest:     lipgloss.Color("#6B7280"),
	headerFg:    lipgloss.Color("#FFFFFF"),
	headerBg:    lipgloss.Color("#1F2937"),
	surface:     lipgloss.Color("#F7FAFC"),
	surfaceText: lipgloss.Color("#374151"),
}

func detectTheme() palette {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("MCRCON_THEME")))
	switch mode {
	case "light":
		return lightPalette
	case "dark":
		return darkPalette
	}
	if mode == "" || mode == "auto" {
		if lipgloss.HasDarkBackground() {
			return darkPalette
		}
		return lightPalette
	}
	return darkPalette
}

var theme = detectTheme()

var (
	ColorAccent    = theme.accent
	ColorAccentDim = theme.accentDim
	ColorDanger    = theme.danger
	ColorWarning   = theme.warning
	ColorMuted     = theme.muted
	ColorBorder    = theme.border
	ColorText      = theme.text
	ColorDim       = theme.dim
)

var (
	StyleApp = lipgloss.NewStyle().
			Padding(0, 1)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.headerFg).
			Background(theme.headerBg).
			Padding(0, 1)

	StyleHeaderAccent = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.accent)

	StyleConnected = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.accent)

	StyleDisconnected = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.danger)

	StyleConnecting = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.warning)

	StyleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.border).
			Padding(0, 1)

	StyleInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.accent).
				Padding(0, 1)

	StyleInputConnecting = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.warning).
				Padding(0, 1)

	StyleInputOffline = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.danger).
				Padding(0, 1)

	StyleInputBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.border).
				Padding(0, 1)

	StyleFooter = lipgloss.NewStyle().
			Foreground(theme.muted)

	StyleError = lipgloss.NewStyle().
			Foreground(theme.danger).
			Bold(true)

	StyleSystem = lipgloss.NewStyle().
			Foreground(theme.dim).
			Italic(true)

	StyleCmd = lipgloss.NewStyle().
			Foreground(theme.accent).
			Bold(true)

	StyleResponse = lipgloss.NewStyle().
			Foreground(theme.text)

	StyleSidebar = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.border).
			Background(theme.surface).
			Padding(0, 1)

	StyleSidebarTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.accent)

	StyleSidebarLabel = lipgloss.NewStyle().
				Foreground(theme.muted)

	StyleSidebarValue = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.surfaceText)

	StyleSidebarDim = lipgloss.NewStyle().
			Foreground(theme.dim)

	StyleHelpTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.accent)

	StyleHelpKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.helpKey)

	StyleTimestamp = lipgloss.NewStyle().
			Foreground(theme.timestamp)

	StyleSuggest = lipgloss.NewStyle().
			Foreground(theme.suggest).
			Italic(true)

	StyleNewOutput = lipgloss.NewStyle().
			Foreground(theme.warning).
			Bold(true)

	// styleBrand is the gradient applied to the product name.
	styleBrand = gradText("mcrcon", theme.accent, theme.accentHi)
)

// gradText renders each rune of s with a color interpolated between c1 and
// c2 (linear RGB), producing a smooth horizontal gradient.
func gradText(s string, c1, c2 lipgloss.Color) string {
	runes := []rune(s)
	if len(runes) <= 1 {
		return lipgloss.NewStyle().Foreground(c1).Render(s)
	}
	var b strings.Builder
	for i, r := range runes {
		t := float64(i) / float64(len(runes)-1)
		b.WriteString(lipgloss.NewStyle().Foreground(lerpColor(c1, c2, t)).Render(string(r)))
	}
	return b.String()
}

// lerpColor linearly interpolates between two hex colors at position t [0,1].
func lerpColor(c1, c2 lipgloss.Color, t float64) lipgloss.Color {
	r1, g1, b1 := hexRGB(string(c1))
	r2, g2, b2 := hexRGB(string(c2))
	l := func(a, b int) int {
		return a + int(float64(b-a)*t)
	}
	return lipgloss.Color(rgbHex(l(r1, r2), l(g1, g2), l(b1, b2)))
}

func hexRGB(s string) (int, int, int) {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		r, g, bb := s[0], s[1], s[2]
		s = string([]byte{r, r, g, g, bb, bb})
	}
	if len(s) != 6 {
		return 0, 0, 0
	}
	v := func(a, b byte) int {
		n := 0
		for _, c := range []byte{a, b} {
			n <<= 4
			switch {
			case c >= '0' && c <= '9':
				n += int(c - '0')
			case c >= 'a' && c <= 'f':
				n += int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				n += int(c-'A') + 10
			}
		}
		return n
	}
	return v(s[0], s[1]), v(s[2], s[3]), v(s[4], s[5])
}

func rgbHex(r, g, b int) string {
	const hex = "0123456789ABCDEF"
	h := func(n int) string {
		if n < 0 {
			n = 0
		}
		if n > 255 {
			n = 255
		}
		return string([]byte{hex[n>>4], hex[n&0xF]})
	}
	return "#" + h(r) + h(g) + h(b)
}

// visibleWidth counts the terminal columns of s, skipping ANSI escape
// sequences so styled text participates in width math like plain text.
func visibleWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		w++
		i += size
	}
	return w
}

// padVisible right-pads s with spaces until it spans exactly w columns,
// regardless of any ANSI styling inside.
func padVisible(s string, w int) string {
	for dw := w - visibleWidth(s); dw > 0; dw-- {
		s += " "
	}
	return s
}

// truncate shortens s to at most w runes, appending an ellipsis.
func truncate(s string, w int) string {
	if w < 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= w {
		return s
	}
	return string(runes[:max(w-1, 0)]) + "…"
}

// splitNoTrailing splits s on newlines without the trailing empty element a
// final newline would produce.
func splitNoTrailing(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// sparkline renders values as a compact unicode bar chart (▁…█). Missing or
// flat data degrade gracefully to a dotted placeholder.
func sparkline(vals []float64, width int) string {
	if width < 1 {
		return ""
	}
	if len(vals) == 0 {
		return strings.Repeat("░", width)
	}
	maxv := 0.0
	for _, v := range vals {
		if v > maxv {
			maxv = v
		}
	}
	if maxv <= 0 {
		return strings.Repeat("░", width)
	}
	blockRunes := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for i := 0; i < width; i++ {
		idx := len(vals) - width + i
		if idx < 0 {
			b.WriteString(" ")
			continue
		}
		level := int(7 * vals[idx] / maxv)
		if level > 7 {
			level = 7
		}
		b.WriteString(string(blockRunes[level]))
	}
	return b.String()
}
