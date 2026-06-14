package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// NodeStatus represents the execution status of a DAG node.
type NodeStatus string

const (
	StatusPending   NodeStatus = "pending"
	StatusRunning   NodeStatus = "running"
	StatusCompleted NodeStatus = "completed"
	StatusFailed    NodeStatus = "failed"
)

// DAGNode represents a single node in the execution DAG.
type DAGNode struct {
	ID        string
	Label     string
	Status    NodeStatus
	Detail    string
	StartTime time.Time
	EndTime   time.Time
	Children  []*DAGNode
}

// DAGView tracks multiple DAGNodes and renders a visual representation
// of multi-agent DAG execution progress.
type DAGView struct {
	title      string
	roots      []*DAGNode
	nodeIndex  map[string]*DAGNode
	tokenCount int
}

// NewDAGView creates a new DAGView with the given title.
func NewDAGView(title string) *DAGView {
	return &DAGView{
		title:     title,
		roots:     make([]*DAGNode, 0),
		nodeIndex: make(map[string]*DAGNode),
	}
}

// AddNode adds a top-level node to the DAG view.
func (d *DAGView) AddNode(id, label string) {
	node := &DAGNode{
		ID:       id,
		Label:    label,
		Status:   StatusPending,
		Children: make([]*DAGNode, 0),
	}
	d.roots = append(d.roots, node)
	d.nodeIndex[id] = node
}

// AddChildNode adds a child node under the specified parent.
func (d *DAGView) AddChildNode(parentID, id, label string) {
	child := &DAGNode{
		ID:       id,
		Label:    label,
		Status:   StatusPending,
		Children: make([]*DAGNode, 0),
	}
	if parent, ok := d.nodeIndex[parentID]; ok {
		parent.Children = append(parent.Children, child)
	} else {
		d.roots = append(d.roots, child)
	}
	d.nodeIndex[id] = child
}

// UpdateNode updates the status and detail of a node by ID.
func (d *DAGView) UpdateNode(id string, status NodeStatus, detail string) {
	node, ok := d.nodeIndex[id]
	if !ok {
		return
	}
	node.Status = status
	node.Detail = detail
	now := time.Now()
	if status == StatusRunning && node.StartTime.IsZero() {
		node.StartTime = now
	}
	if (status == StatusCompleted || status == StatusFailed) && node.EndTime.IsZero() {
		node.EndTime = now
	}
}

// SetTokenCount sets the cumulative token count displayed in the status line.
func (d *DAGView) SetTokenCount(count int) {
	d.tokenCount = count
}

// Progress returns the overall completion percentage as a float64 between 0 and 1.
func (d *DAGView) Progress() float64 {
	total := 0
	completed := 0
	d.walkNodes(func(n *DAGNode) {
		total++
		if n.Status == StatusCompleted {
			completed++
		}
	})
	if total == 0 {
		return 0
	}
	return float64(completed) / float64(total)
}

// Render produces a string representation of the DAG view that fits within
// the given width, using box-drawing characters and a progress bar.
func (d *DAGView) Render(width int) string {
	if width < 20 {
		width = 20
	}

	innerWidth := width - 4 // subtract box borders + padding

	// Styles
	completedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	runningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))    // yellow
	pendingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))    // dim gray
	failedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))     // red
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // cyan

	var sb strings.Builder

	// Top border
	sb.WriteString("┌" + strings.Repeat("─", width-2) + "┌\n")

	// Title line
	titleText := titleStyle.Render("🔄 " + d.title)
	sb.WriteString("│  " + padRight(titleText, innerWidth) + "│\n")

	// Blank line
	sb.WriteString("│" + strings.Repeat(" ", width-2) + "│\n")

	// Render each root node and its children
	for i, root := range d.roots {
		isLastRoot := i == len(d.roots)-1
		sb.WriteString(renderNodeLine(root, "", isLastRoot, innerWidth,
			completedStyle, runningStyle, pendingStyle, failedStyle))

		for j, child := range root.Children {
			isLastChild := j == len(root.Children)-1
			prefix := "  ├── "
			if isLastRoot && isLastChild {
				prefix = "  └── "
			} else if isLastChild {
				prefix = "  └── "
			}
			sb.WriteString(renderNodeLine(child, prefix, isLastRoot, innerWidth,
				completedStyle, runningStyle, pendingStyle, failedStyle))
		}
	}

	// Blank line
	sb.WriteString("│" + strings.Repeat(" ", width-2) + "│\n")

	// Progress bar line
	progressLine := renderProgressLine(d.Progress(), d.tokenCount, innerWidth)
	sb.WriteString("│  " + padRight(progressLine, innerWidth) + "│\n")

	// Bottom border
	sb.WriteString("└" + strings.Repeat("─", width-2) + "└\n")

	return sb.String()
}

// renderNodeLine renders a single node line with status icon and detail.
func renderNodeLine(node *DAGNode, prefix string, _ bool, innerWidth int,
	completedStyle, runningStyle, pendingStyle, failedStyle lipgloss.Style) string {

	icon, styledLabel := nodeStyleAndIcon(node, completedStyle, runningStyle, pendingStyle, failedStyle)

	line := prefix + "[" + icon + "] " + styledLabel
	if node.Detail != "" {
		line += "  ─ " + node.Detail
	}

	// Strip ANSI codes for width measurement, then pad
	visibleLen := lipgloss.Width(line)
	if visibleLen > innerWidth {
		// Truncate visible content — just cut the detail
		line = prefix + "[" + icon + "] " + styledLabel
		visibleLen = lipgloss.Width(line)
		detailSpace := innerWidth - visibleLen - 5 // "  ─ " + some room
		if detailSpace > 3 && node.Detail != "" {
			truncDetail := node.Detail
			if len(truncDetail) > detailSpace {
				truncDetail = truncDetail[:detailSpace-1] + "…"
			}
			line += "  ─ " + truncDetail
		}
	}

	return "│" + padRight(line, innerWidth+1) + "│\n" // +1 for the leading space after │
}

// nodeStyleAndIcon returns the status icon and styled label for a node.
func nodeStyleAndIcon(node *DAGNode, completedStyle, runningStyle, pendingStyle, failedStyle lipgloss.Style) (string, string) {
	switch node.Status {
	case StatusCompleted:
		return completedStyle.Render("✓"), completedStyle.Render(node.Label)
	case StatusRunning:
		return runningStyle.Render("⟳"), runningStyle.Render(node.Label)
	case StatusFailed:
		return failedStyle.Render("✗"), failedStyle.Render(node.Label)
	default: // StatusPending
		return pendingStyle.Render("⏳"), pendingStyle.Render(node.Label)
	}
}

// renderProgressLine creates the progress bar and token count string.
func renderProgressLine(progress float64, tokenCount int, width int) string {
	pct := int(progress * 100)

	// Progress bar
	barMaxWidth := 20
	if width < 50 {
		barMaxWidth = 10
	}
	filled := int(float64(barMaxWidth) * progress)
	if filled > barMaxWidth {
		filled = barMaxWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barMaxWidth-filled)

	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan
	styledBar := barStyle.Render(bar)

	// Token count formatting
	tokenStr := formatTokenCount(tokenCount)

	line := fmt.Sprintf("Progress: %s %d%%  Tokens: %s", styledBar, pct, tokenStr)
	return line
}

// formatTokenCount formats a token count as a human-readable string (e.g. "8.2K").
func formatTokenCount(count int) string {
	if count >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(count)/1_000_000)
	}
	if count >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(count)/1_000)
	}
	return fmt.Sprintf("%d", count)
}

// padRight pads a string (accounting for ANSI escape sequences) to the given
// visible width by appending spaces.
func padRight(s string, visibleWidth int) string {
	current := lipgloss.Width(s)
	if current >= visibleWidth {
		return s
	}
	return s + strings.Repeat(" ", visibleWidth-current)
}

// walkNodes visits every node in the tree (roots and their children).
func (d *DAGView) walkNodes(fn func(*DAGNode)) {
	for _, root := range d.roots {
		fn(root)
		for _, child := range root.Children {
			fn(child)
		}
	}
}
