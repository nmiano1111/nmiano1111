package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/nmiano/nickbot/internal/agent"
)

// DisplayStatus is a presentation-layer classification of run state.
// It is computed from the run record and never stored or transmitted.
// The interface must never assert state the runtime cannot prove.
type DisplayStatus int

const (
	DisplayPending   DisplayStatus = iota
	DisplayRunning
	DisplayCompleted
	DisplayFailed
	DisplayCancelled
)

// DeriveDisplayStatus computes the display status from a run record.
// This is the single source of display logic; the TUI never branches on
// agent.RunStatus directly.
func DeriveDisplayStatus(run *agent.Run) DisplayStatus {
	switch run.Status {
	case agent.RunStatusCompleted:
		return DisplayCompleted
	case agent.RunStatusFailed:
		return DisplayFailed
	case agent.RunStatusCancelled:
		return DisplayCancelled
	case agent.RunStatusRunning:
		return DisplayRunning
	default:
		return DisplayPending
	}
}

// DisplayStatusBadge returns the badge string and Lipgloss style for a display status.
func DisplayStatusBadge(ds DisplayStatus) (badge string, style lipgloss.Style) {
	switch ds {
	case DisplayCompleted:
		return BadgeCompleted, StyleSuccess
	case DisplayFailed:
		return BadgeFailed, StyleError
	case DisplayRunning:
		return BadgeRunning, StyleAccent
	case DisplayCancelled:
		return BadgeCancelled, StyleDim
	default:
		return BadgePending, StyleWarning
	}
}
