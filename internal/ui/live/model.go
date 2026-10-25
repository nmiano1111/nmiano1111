// Package live provides a Bubble Tea model for live-streaming agent execution.
// It renders a real-time execution view: header, spinner, event stream.
package live

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nmiano/nickbot/internal/agent"
	"github.com/nmiano/nickbot/internal/ui"
)

// ── Messages ──────────────────────────────────────────────────────────────────

// EventMsg carries a received agent event.
type EventMsg agent.Event

// DoneMsg signals that the event channel has closed.
type DoneMsg struct{}

// errMsg carries a fatal error from the TUI.
type errMsg struct{ err error }

// tickMsg is emitted periodically to refresh the spinner.
type tickMsg time.Time

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the Bubble Tea model for live execution display.
type Model struct {
	// Run is the initial run info (ID, agent, model, etc.).
	Run *agent.Run
	// EventCh is the channel of incoming events.
	EventCh <-chan agent.Event

	spinner      spinner.Model
	events       []renderedEvent
	done         bool          // channel has closed; TUI should quit
	terminalKind agent.EventKind // kind of last terminal event (completed/failed/verification)
	width        int
	height       int
	startedAt    time.Time
}

type renderedEvent struct {
	line  string
	isErr bool
}

// New creates a live model for the given run and event channel.
func New(run *agent.Run, ch <-chan agent.Event) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = ui.StyleAccent

	return Model{
		Run:       run,
		EventCh:   ch,
		spinner:   sp,
		startedAt: time.Now(),
		width:     80,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitEvent(m.EventCh),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case EventMsg:
		e := agent.Event(msg)
		m.events = append(m.events, renderEvent(e))
		switch e.Kind {
		case agent.EventCompleted, agent.EventFailed, agent.EventVerification:
			m.terminalKind = e.Kind
		}
		// Always keep reading; quit when the channel closes (DoneMsg).
		return m, waitEvent(m.EventCh)

	case DoneMsg:
		m.done = true
		return m, tea.Quit

	case errMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	var sb strings.Builder

	// ── Header ────────────────────────────────────────────────────────────────
	appName := ui.StyleAccent.Bold(true).Render("agentctl")
	runIDStr := ui.StyleRunID.Render(ui.ShortID(m.Run.ID))
	headerRight := ui.StyleDim.Render(m.Run.RunnerName)
	headerLine := fmt.Sprintf("%s  ·  Run %s  ·  %s", appName, runIDStr, headerRight)
	sb.WriteString("\n" + headerLine + "\n")
	sb.WriteString(ui.HRule(w-2) + "\n\n")

	// ── Meta ──────────────────────────────────────────────────────────────────
	ds := ui.DeriveDisplayStatus(m.Run)
	if m.terminalKind != "" {
		// Derive status from the last terminal event, including verification.
		switch m.terminalKind {
		case agent.EventFailed:
			ds = ui.DisplayFailed
		case agent.EventVerification:
			// Check whether the verification passed by inspecting the last event.
			if len(m.events) > 0 && m.events[len(m.events)-1].isErr {
				ds = ui.DisplayFailed
			} else {
				ds = ui.DisplayCompleted
			}
		default:
			ds = ui.DisplayCompleted
		}
	}
	badge, badgeStyle := ui.DisplayStatusBadge(ds)
	var statusStr string
	if m.done {
		statusStr = badgeStyle.Render(badge)
	} else {
		statusStr = m.spinner.View() + " " + ui.StyleAccent.Render("running")
	}

	field := func(k, v string) string {
		return fmt.Sprintf("  %-12s %s", ui.StyleDim.Render(k), v)
	}
	sb.WriteString(field("Agent", ui.StyleBase.Render(m.Run.AgentID)) + "\n")
	if m.Run.Model != "" {
		sb.WriteString(field("Model", ui.StyleBase.Render(m.Run.Model)) + "\n")
	}
	sb.WriteString(field("Status", statusStr) + "\n")
	sb.WriteString("\n")
	sb.WriteString(ui.HRule(w-2) + "\n\n")

	// ── Events ────────────────────────────────────────────────────────────────
	sb.WriteString("  " + ui.StyleDim.Render("Events") + "\n\n")

	// Show the last N events that fit in the terminal.
	maxEvents := m.height - 16
	if maxEvents < 5 {
		maxEvents = 5
	}
	start := 0
	if len(m.events) > maxEvents {
		start = len(m.events) - maxEvents
	}
	for _, ev := range m.events[start:] {
		sb.WriteString(ev.line + "\n")
	}

	if !m.done {
		sb.WriteString("\n")
		// Footer help
		sb.WriteString(ui.StyleDim.Render("  q  quit") + "\n")
	}

	return sb.String()
}

// FinalView returns a compact completion summary for printing after the TUI exits.
func (m Model) FinalView() string {
	if len(m.events) == 0 {
		return ""
	}
	var sb strings.Builder
	// Find the last terminal event for a summary line.
	last := m.events[len(m.events)-1]
	sb.WriteString("\n")
	sb.WriteString(last.line + "\n")
	sb.WriteString("\n")
	return sb.String()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func waitEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return DoneMsg{}
		}
		return EventMsg(e)
	}
}

// outputPattern maps a line prefix to a display style for text event content.
type outputPattern struct {
	prefix string
	style  lipgloss.Style
}

// claudeCodePatterns highlights common Claude Code output prefixes in the TUI.
var claudeCodePatterns = []outputPattern{
	{"Reading ",  ui.StyleDim},
	{"Writing ",  ui.StyleSuccess},
	{"Editing ",  ui.StyleSuccess},
	{"Creating ", ui.StyleSuccess},
	{"Deleting ", ui.StyleWarning},
	{"Running ",  ui.StyleAccent},
	{"Bash(",     ui.StyleAccent},
	{"Error:",    ui.StyleError},
	{"✓ ",        ui.StyleSuccess},
	{"✗ ",        ui.StyleError},
}

// applyOutputPattern returns the style for a text content line, based on prefix matching.
func applyOutputPattern(s string) lipgloss.Style {
	for _, p := range claudeCodePatterns {
		if strings.HasPrefix(s, p.prefix) {
			return p.style
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "#CDD6F4", Light: "#4C4F69"})
}

func renderEvent(e agent.Event) renderedEvent {
	ts := e.Timestamp.Format("15:04:05")
	display, ok := ui.EventKindDisplays[string(e.Kind)]
	if !ok {
		display = ui.EventKindDisplay{Label: string(e.Kind), Style: ui.StyleDim}
	}

	detail := eventDetail(e)
	if len([]rune(detail)) > 70 {
		detail = string([]rune(detail)[:69]) + "…"
	}

	// Apply output pattern styling to text event details.
	detailStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "#CDD6F4", Light: "#4C4F69"})
	if e.Kind == agent.EventText {
		detailStyle = applyOutputPattern(detail)
	}

	line := fmt.Sprintf("  %s  %s  %s",
		ui.StyleDim.Render(ts),
		display.Style.Render(display.Label),
		detailStyle.Render(detail),
	)
	isErr := e.Kind == agent.EventFailed
	if e.Kind == agent.EventVerification {
		var d agent.VerificationData
		if e.DecodeData(&d) == nil {
			isErr = !d.Passed
		}
	}
	return renderedEvent{
		line:  line,
		isErr: isErr,
	}
}

func eventDetail(e agent.Event) string {
	switch e.Kind {
	case agent.EventText:
		var d agent.TextData
		_ = e.DecodeData(&d)
		s := strings.TrimSpace(d.Content)
		s = strings.ReplaceAll(s, "\n", " ")
		return s
	case agent.EventToolUse:
		var d agent.ToolUseData
		_ = e.DecodeData(&d)
		input := ""
		if len(d.Input) > 0 && string(d.Input) != "null" {
			raw := strings.TrimSpace(string(d.Input))
			if len(raw) > 40 {
				raw = raw[:37] + "..."
			}
			input = " → " + raw
		}
		return d.Name + input
	case agent.EventToolResult:
		var d agent.ToolResultData
		_ = e.DecodeData(&d)
		s := strings.TrimSpace(strings.ReplaceAll(d.Content, "\n", " "))
		if d.IsError {
			return "[error] " + s
		}
		return s
	case agent.EventCompleted:
		var d agent.CompletedData
		_ = e.DecodeData(&d)
		if d.CostUSD > 0 {
			return fmt.Sprintf("%s  ($%.4f · %d tokens · %s)",
				truncStr(d.Result, 30), d.CostUSD,
				d.InputTokens+d.OutputTokens,
				ui.HumanDuration(d.DurationMS))
		}
		return truncStr(d.Result, 60)
	case agent.EventFailed:
		var d agent.FailedData
		_ = e.DecodeData(&d)
		return d.Error
	case agent.EventVerification:
		var d agent.VerificationData
		_ = e.DecodeData(&d)
		if d.Passed {
			return "✓ passed  " + truncStr(d.Command, 40)
		}
		return "✗ failed  " + truncStr(d.Command, 40)
	default:
		return ""
	}
}

func truncStr(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

