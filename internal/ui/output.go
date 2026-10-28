package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/nmiano/nickbot/internal/agent"
)

// Printer writes formatted output to a writer.
type Printer struct {
	w    io.Writer
	json bool
}

// NewPrinter creates a printer. If jsonMode is true, output is JSONL.
func NewPrinter(w io.Writer, jsonMode bool) *Printer {
	return &Printer{w: w, json: jsonMode}
}

// Println writes a line to the writer.
func (p *Printer) Println(s string) {
	fmt.Fprintln(p.w, s)
}

// PrintRun prints a run summary in human-readable or JSON format.
func (p *Printer) PrintRun(run *agent.Run) {
	if p.json {
		b, _ := json.Marshal(run)
		fmt.Fprintln(p.w, string(b))
		return
	}
	p.Println(FormatRunDetail(run))
}

// PrintRunTable prints a table of runs.
func (p *Printer) PrintRunTable(runs []*agent.Run) {
	if p.json {
		for _, r := range runs {
			b, _ := json.Marshal(r)
			fmt.Fprintln(p.w, string(b))
		}
		return
	}
	p.Println(FormatRunTable(runs))
}

// PrintEvent prints a single event.
func (p *Printer) PrintEvent(e *agent.Event) {
	if p.json {
		b, _ := json.Marshal(e)
		fmt.Fprintln(p.w, string(b))
		return
	}
	p.Println(FormatEvent(e))
}

// PrintError prints an error message.
func (p *Printer) PrintError(msg string, err error) {
	if p.json {
		b, _ := json.Marshal(map[string]string{"error": msg, "detail": err.Error()})
		fmt.Fprintln(p.w, string(b))
		return
	}
	errMsg := StyleError.Render("✗ Error: ") + StyleBase.Render(msg)
	if err != nil {
		errMsg += "\n  " + StyleDim.Render(err.Error())
	}
	fmt.Fprintln(p.w, errMsg)
}

// PrintSuccess prints a success message.
func (p *Printer) PrintSuccess(msg string) {
	if p.json {
		b, _ := json.Marshal(map[string]string{"status": "ok", "message": msg})
		fmt.Fprintln(p.w, string(b))
		return
	}
	fmt.Fprintln(p.w, StyleSuccess.Render("✓ ")+StyleBase.Render(msg))
}

// ── Formatters ────────────────────────────────────────────────────────────────

// FormatRunDetail returns a multi-line detail view of a run.
func FormatRunDetail(run *agent.Run) string {
	var sb strings.Builder

	badge, style := statusBadge(run.Status)
	sb.WriteString("\n")
	sb.WriteString(StyleHeader.Render("Run") + "  " + StyleRunID.Render(ShortID(run.ID)) + "\n")
	sb.WriteString(HRule(60) + "\n\n")

	field := func(k, v string) {
		sb.WriteString(fmt.Sprintf("  %-14s %s\n", StyleDim.Render(k), v))
	}

	field("ID", StyleRunID.Render(run.ID))
	field("Agent", StyleBase.Render(run.AgentID))
	field("Runner", StyleBase.Render(run.RunnerName))
	if run.Model != "" {
		field("Model", StyleBase.Render(run.Model))
	}
	field("Status", style.Render(badge))
	field("Created", StyleBase.Render(humanTime(run.CreatedAt)))
	if run.StartedAt != nil {
		field("Started", StyleBase.Render(humanTime(*run.StartedAt)))
	}
	if run.CompletedAt != nil {
		field("Completed", StyleBase.Render(humanTime(*run.CompletedAt)))
	}
	if run.DurationMS > 0 {
		field("Duration", StyleBase.Render(HumanDuration(run.DurationMS)))
	}
	if run.CostUSD > 0 {
		field("Cost", StyleBase.Render(fmt.Sprintf("$%.4f", run.CostUSD)))
	}
	if run.InputTokens > 0 || run.OutputTokens > 0 {
		field("Tokens", StyleBase.Render(fmt.Sprintf("%d in / %d out", run.InputTokens, run.OutputTokens)))
	}

	sb.WriteString("\n")
	sb.WriteString("  " + StyleDim.Render("Prompt") + "\n")
	prompt := run.Prompt
	if len(prompt) > 200 {
		prompt = prompt[:197] + "..."
	}
	sb.WriteString(wrapIndent(prompt, 2, 72) + "\n")

	if run.Result != "" {
		sb.WriteString("\n")
		sb.WriteString("  " + StyleDim.Render("Result") + "\n")
		result := run.Result
		if len(result) > 500 {
			result = result[:497] + "..."
		}
		sb.WriteString(wrapIndent(result, 2, 72) + "\n")
	}
	if run.Error != "" {
		sb.WriteString("\n")
		sb.WriteString("  " + StyleError.Render("Error") + "\n")
		sb.WriteString(wrapIndent(run.Error, 2, 72) + "\n")
	}

	sb.WriteString("\n" + HRule(60) + "\n")
	return sb.String()
}

// FormatRunTable renders a compact table of runs.
func FormatRunTable(runs []*agent.Run) string {
	if len(runs) == 0 {
		return StyleDim.Render("  No runs found.\n")
	}

	const (
		idW     = 36 // full UUID
		agW     = 14
		stW     = 12
		timeW   = 10
		costW   = 8
		parentW = 10 // short parent ID
	)

	// Show PARENT column only when at least one run has a parent.
	hasParents := false
	for _, r := range runs {
		if r.ParentRunID != "" {
			hasParents = true
			break
		}
	}

	col := func(s string, w int) string { return fmt.Sprintf("%-*s", w, s) }

	cols := []string{
		StyleDim.Render(col("ID", idW)),
		StyleDim.Render(col("AGENT", agW)),
		StyleDim.Render(col("STATUS", stW)),
		StyleDim.Render(col("CREATED", timeW)),
		StyleDim.Render(col("COST", costW)),
	}
	ruleW := idW + agW + stW + timeW + costW + 8
	if hasParents {
		cols = append(cols, StyleDim.Render(col("PARENT", parentW)))
		ruleW += parentW + 2
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  " + strings.Join(cols, "  ") + "\n")
	sb.WriteString("  " + HRule(ruleW) + "\n")

	for _, r := range runs {
		badge, style := statusBadge(r.Status)
		cost := "—"
		if r.CostUSD > 0 {
			cost = fmt.Sprintf("$%.4f", r.CostUSD)
		}
		rowCols := []string{
			StyleRunID.Render(col(r.ID, idW)),
			StyleBase.Render(col(trunc(r.AgentID, agW), agW)),
			style.Render(col(trunc(badge, stW), stW)),
			StyleDim.Render(col(relTime(r.CreatedAt), timeW)),
			StyleDim.Render(col(cost, costW)),
		}
		if hasParents {
			parent := "—"
			if r.ParentRunID != "" {
				parent = "↳ " + ShortID(r.ParentRunID)
			}
			rowCols = append(rowCols, StyleDim.Render(col(parent, parentW)))
		}
		sb.WriteString("  " + strings.Join(rowCols, "  ") + "\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// FormatEvent renders a single event line.
func FormatEvent(e *agent.Event) string {
	ts := e.Timestamp.Format("15:04:05")
	display, ok := EventKindDisplays[string(e.Kind)]
	if !ok {
		display = EventKindDisplay{Label: string(e.Kind), Style: StyleDim}
	}

	detail := eventDetail(e)
	if len(detail) > 80 {
		detail = detail[:77] + "..."
	}

	return fmt.Sprintf("  %s  %s  %s",
		StyleDim.Render(ts),
		display.Style.Render(display.Label),
		StyleBase.Render(detail),
	)
}

func eventDetail(e *agent.Event) string {
	switch e.Kind {
	case agent.EventText:
		var d agent.TextData
		_ = e.DecodeData(&d)
		return strings.TrimSpace(d.Content)
	case agent.EventToolUse:
		var d agent.ToolUseData
		_ = e.DecodeData(&d)
		input := ""
		if len(d.Input) > 0 && string(d.Input) != "null" {
			input = " → " + trunc(string(d.Input), 40)
		}
		return d.Name + input
	case agent.EventToolResult:
		var d agent.ToolResultData
		_ = e.DecodeData(&d)
		s := trunc(strings.TrimSpace(d.Content), 60)
		if d.IsError {
			return StyleError.Render("[error] ") + s
		}
		return s
	case agent.EventCompleted:
		var d agent.CompletedData
		_ = e.DecodeData(&d)
		if d.CostUSD > 0 {
			return fmt.Sprintf("%s  ($%.4f, %d tokens)", trunc(d.Result, 40), d.CostUSD, d.InputTokens+d.OutputTokens)
		}
		return trunc(d.Result, 60)
	case agent.EventFailed:
		var d agent.FailedData
		_ = e.DecodeData(&d)
		return StyleError.Render(d.Error)
	case agent.EventVerification:
		var d agent.VerificationData
		_ = e.DecodeData(&d)
		if d.Passed {
			return StyleSuccess.Render("✓ passed") + "  " + trunc(d.Command, 40)
		}
		return StyleError.Render("✗ failed") + "  " + trunc(d.Command, 40)
	default:
		return ""
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func statusBadge(s agent.RunStatus) (string, lipgloss.Style) {
	switch s {
	case agent.RunStatusCompleted:
		return BadgeCompleted, StyleSuccess
	case agent.RunStatusFailed:
		return BadgeFailed, StyleError
	case agent.RunStatusRunning:
		return BadgeRunning, StyleAccent
	case agent.RunStatusCancelled:
		return BadgeCancelled, StyleDim
	default:
		return BadgePending, StyleWarning
	}
}

// ShortID returns the first 8 characters of id, or id itself if shorter.
func ShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func trunc(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-1]) + "…"
}

func humanTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04:05")
}

func relTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// HumanDuration formats a duration in milliseconds as a human-readable string.
func HumanDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	s := float64(ms) / 1000.0
	if s < 60 {
		return fmt.Sprintf("%.1fs", s)
	}
	m := math.Floor(s / 60)
	rem := s - m*60
	return fmt.Sprintf("%.0fm%.0fs", m, rem)
}

func wrapIndent(s string, indent, width int) string {
	prefix := strings.Repeat(" ", indent)
	words := strings.Fields(s)
	if len(words) == 0 {
		return prefix
	}
	var lines []string
	line := prefix
	for _, w := range words {
		if utf8.RuneCountInString(line)+utf8.RuneCountInString(w)+1 > width && line != prefix {
			lines = append(lines, line)
			line = prefix + w
		} else if line == prefix {
			line += w
		} else {
			line += " " + w
		}
	}
	if line != prefix {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
