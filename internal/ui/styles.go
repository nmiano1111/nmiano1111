// Package ui provides presentation utilities for the CLI and TUI.
// All rendering logic is isolated here; orchestration code never imports ui.
package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Palette defines the colour system. We use a restrained, dark-friendly set
// inspired by Catppuccin Mocha. Adaptive colours degrade gracefully in
// terminals that report a light background.
var (
	// colBlue is the primary accent used for identifiers and prompts.
	colBlue = lipgloss.AdaptiveColor{Dark: "#89B4FA", Light: "#1E66F5"}
	// colGreen signals success.
	colGreen = lipgloss.AdaptiveColor{Dark: "#A6E3A1", Light: "#40A02B"}
	// colRed signals failure or error.
	colRed = lipgloss.AdaptiveColor{Dark: "#F38BA8", Light: "#D20F39"}
	// colYellow signals a warning or pending state.
	colYellow = lipgloss.AdaptiveColor{Dark: "#F9E2AF", Light: "#DF8E1D"}
	// colMauve is used for tool-use events.
	colMauve = lipgloss.AdaptiveColor{Dark: "#CBA6F7", Light: "#8839EF"}
	// colPeach is used for tool-result events.
	colPeach = lipgloss.AdaptiveColor{Dark: "#FAB387", Light: "#FE640B"}
	// colText is the primary text colour.
	colText = lipgloss.AdaptiveColor{Dark: "#CDD6F4", Light: "#4C4F69"}
	// colSubtext is used for secondary / dimmed text.
	colSubtext = lipgloss.AdaptiveColor{Dark: "#6C7086", Light: "#9CA0B0"}
	// colBorder is used for rule lines and box borders.
	colBorder = lipgloss.AdaptiveColor{Dark: "#313244", Light: "#DCE0E8"}
	// colSurface is a slightly elevated background.
	colSurface = lipgloss.AdaptiveColor{Dark: "#1E1E2E", Light: "#EFF1F5"}
)

// Base styles — compose these rather than re-defining colours elsewhere.
var (
	StyleBase = lipgloss.NewStyle().Foreground(colText)
	StyleDim  = lipgloss.NewStyle().Foreground(colSubtext)
	StyleBold = lipgloss.NewStyle().Bold(true).Foreground(colText)

	StyleSuccess = lipgloss.NewStyle().Foreground(colGreen)
	StyleError   = lipgloss.NewStyle().Foreground(colRed)
	StyleWarning = lipgloss.NewStyle().Foreground(colYellow)
	StyleAccent  = lipgloss.NewStyle().Foreground(colBlue)
	StyleMauve   = lipgloss.NewStyle().Foreground(colMauve)
	StylePeach   = lipgloss.NewStyle().Foreground(colPeach)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colBlue).
			PaddingLeft(1).
			PaddingRight(1)

	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBorder)

	StyleRule = lipgloss.NewStyle().
			Foreground(colBorder)

	StyleRunID = lipgloss.NewStyle().
			Foreground(colBlue).
			Bold(true)

	StylePrompt = lipgloss.NewStyle().
			Foreground(colText).
			Italic(true)
)

// Status badges — includes leading space for alignment.
const (
	BadgePending   = "◦ pending"
	BadgeRunning   = "⠸ running"
	BadgeCompleted = "✓ done"
	BadgeFailed    = "✗ failed"
	BadgeCancelled = "⊘ cancelled"
)

// StatusStyle returns the style for a given status badge string.
func StatusStyle(status string) lipgloss.Style {
	switch status {
	case BadgeCompleted:
		return StyleSuccess
	case BadgeFailed:
		return StyleError
	case BadgeRunning:
		return StyleAccent
	case BadgeCancelled:
		return StyleDim
	default:
		return StyleWarning
	}
}

// EventKindLabel maps agent event kinds to display labels and styles.
type EventKindDisplay struct {
	Label string
	Style lipgloss.Style
}

// EventKindDisplays maps event kind strings to display metadata.
var EventKindDisplays = map[string]EventKindDisplay{
	"started":      {Label: "▶ started    ", Style: StyleAccent},
	"text":         {Label: "◆ text       ", Style: StyleBase},
	"tool_use":     {Label: "⚙ tool_use   ", Style: StyleMauve},
	"tool_result":  {Label: "◇ tool_result", Style: StylePeach},
	"completed":    {Label: "✓ completed  ", Style: StyleSuccess},
	"failed":       {Label: "✗ failed     ", Style: StyleError},
	"verification": {Label: "⊕ verify     ", Style: StyleAccent},
}

// HRule returns a horizontal rule string of width w.
func HRule(w int) string {
	if w <= 0 {
		w = 60
	}
	r := ""
	for i := 0; i < w; i++ {
		r += "─"
	}
	return StyleRule.Render(r)
}

// NoColor reports whether color output is disabled.
// Respects NO_COLOR and TERM=dumb.
func NoColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return true
	}
	if os.Getenv("TERM") == "dumb" {
		return true
	}
	return false
}
