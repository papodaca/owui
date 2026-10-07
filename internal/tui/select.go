package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// selection is a drag inside the transcript. The copied text is the chat
// content, not the screen cells, so the sidebar is not included. A plain drag
// is what both a normal emulator and Herdr deliver while mouse reporting is on.
// Shift-drag is the terminal's own selection of the whole screen, sidebar included.
type selection struct {
	active bool
	anchor cell
	head   cell
}

type cell struct {
	line int
	col  int
}

func (m *App) transcriptCell(px, py int) (cell, bool) {
	ox, oy := 0, 1
	if m.showSidebar {
		ox = sidebarWidth
	}
	h := m.vp.Height()
	if h < 1 || px < ox || py < oy || py >= oy+h {
		return cell{}, false
	}
	return cell{
		line: m.vp.YOffset() + (py - oy),
		col:  px - ox + m.vp.XOffset(),
	}, true
}

func (m *App) clampTranscriptCell(px, py int) cell {
	ox, oy := 0, 1
	if m.showSidebar {
		ox = sidebarWidth
	}
	h := m.vp.Height()
	if h < 1 {
		h = 1
	}
	if py < oy {
		py = oy
	}
	if py >= oy+h {
		py = oy + h - 1
	}
	if px < ox {
		px = ox
	}
	maxX := ox + m.vp.Width() - 1
	if maxX < ox {
		maxX = ox
	}
	if px > maxX {
		px = maxX
	}
	return cell{
		line: m.vp.YOffset() + (py - oy),
		col:  px - ox + m.vp.XOffset(),
	}
}

func (m App) inComposer(y int) bool {
	h := m.vp.Height()
	if h < 1 {
		h = 1
	}
	return y >= 1+h+2
}

func (m *App) showSelection() {
	if m.rendered == "" {
		m.rendered = m.vp.GetContent()
	}
	y := m.vp.YOffset()
	m.vp.SetContent(markSelection(m.rendered, m.sel.anchor, m.sel.head))
	m.vp.SetYOffset(y)
}

func ordered(a, b cell) (cell, cell) {
	if a.line > b.line || (a.line == b.line && a.col > b.col) {
		return b, a
	}
	return a, b
}

func selectedText(content string, a, b cell) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return ""
	}
	a, b = ordered(a, b)
	if a.line < 0 {
		a.line = 0
	}
	if b.line >= len(lines) {
		b.line = len(lines) - 1
	}
	if a.line > b.line {
		return ""
	}
	var out strings.Builder
	for i := a.line; i <= b.line; i++ {
		line := lines[i]
		width := ansi.StringWidth(line)
		start, end := 0, width
		if i == a.line {
			start = min(max(a.col, 0), width)
		}
		if i == b.line {
			end = min(max(b.col, 0), width)
		}
		if end < start {
			end = start
		}
		if i > a.line {
			out.WriteByte('\n')
		}
		out.WriteString(ansi.Strip(ansi.Cut(line, start, end)))
	}
	return out.String()
}

func markSelection(content string, a, b cell) string {
	lines := strings.Split(content, "\n")
	a, b = ordered(a, b)
	rev := lipgloss.NewStyle().Reverse(true).Inline(true)
	for i := range lines {
		if i < a.line || i > b.line {
			continue
		}
		line := lines[i]
		width := ansi.StringWidth(line)
		start, end := 0, width
		if i == a.line {
			start = min(max(a.col, 0), width)
		}
		if i == b.line {
			end = min(max(b.col, 0), width)
		}
		if end <= start {
			continue
		}
		left := ansi.Cut(line, 0, start)
		mid := ansi.Strip(ansi.Cut(line, start, end))
		right := ansi.Cut(line, end, width)
		lines[i] = left + rev.Render(mid) + right
	}
	return strings.Join(lines, "\n")
}

func copyText(s string) tea.Cmd {
	return tea.Batch(
		tea.SetClipboard(s),
		func() tea.Msg {
			_ = clipboard.WriteAll(s)
			return nil
		},
	)
}
