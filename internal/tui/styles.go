package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Palette — Minecraft-ish dark theme with creeper-green accent.
var (
	ColorAccent    = lipgloss.Color("#5EBB2B")
	ColorAccentDim = lipgloss.Color("#3E7A1E")
	ColorDanger    = lipgloss.Color("#E5484D")
	ColorWarning   = lipgloss.Color("#E5A90B")
	ColorMuted     = lipgloss.Color("#8B8B8B")
	ColorBorder    = lipgloss.Color("#3A3A3A")
	ColorText      = lipgloss.Color("#E8E8E8")
	ColorDim       = lipgloss.Color("#A0A0A0")
)

var (
	StyleApp = lipgloss.NewStyle().
			Padding(0, 1)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#2D2D2D")).
			Padding(0, 1)

	StyleHeaderAccent = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent)

	StyleConnected = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	StyleDisconnected = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorDanger)

	StyleConnecting = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWarning)

	StyleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	StyleInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorAccent).
				Padding(0, 1)

	StyleInputConnecting = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				Padding(0, 1)

	StyleInputOffline = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorDanger).
				Padding(0, 1)

	StyleInputBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorder).
				Padding(0, 1)

	StyleFooter = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleError = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)

	StyleSystem = lipgloss.NewStyle().
			Foreground(ColorDim).
			Italic(true)

	StyleCmd = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	StyleResponse = lipgloss.NewStyle().
			Foreground(ColorText)

	StyleHelpTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	StyleHelpKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7DD3FC"))

	StyleTimestamp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5A5A5A"))

	StyleSuggest = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9A9A9A")).
			Italic(true)

	StyleNewOutput = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)
)
