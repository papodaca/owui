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
	alt    bool
}

func altHeld(mod tea.KeyMod) bool {
	return mod&tea.ModAlt != 0
}

func (m App) selectionText() string {
	shown := selectedText(m.renderedText(), m.sel.anchor, m.sel.head)
	if !m.sel.alt {
		return shown
	}
	src := sourceText(m.renderedText(), m.copyBlocks, m.lineBlock, m.sel.anchor, m.sel.head)
	if strings.TrimSpace(src) == "" {
		return shown
	}
	return src
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

// cutEnd is the exclusive column just after the cell under the pointer.
// ansi.Cut stops before its right argument, so the cell itself has to be included.
func cutEnd(col, width int) int {
	if col < 0 {
		col = 0
	}
	if col >= width {
		return width
	}
	return col + 1
}

// dropScreenPadding removes spaces the renderer adds so a wrapped line fills
// the screen. The document ends at the newline, not at that padding.
func dropScreenPadding(line string, start, end int) string {
	return strings.TrimRight(ansi.Strip(ansi.Cut(line, start, end)), " ")
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
			end = cutEnd(b.col, width)
		}
		if end < start {
			end = start
		}
		piece := dropScreenPadding(line, start, end)
		if i > a.line {
			out.WriteByte('\n')
		}
		out.WriteString(piece)
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
			end = cutEnd(b.col, width)
		}
		if end <= start {
			continue
		}
		end = start + ansi.StringWidth(dropScreenPadding(line, start, end))
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

// sourceText copies the markdown blocks touched by the selection. A rendered
// table is several screen lines and one source block, so any of those lines
// yields the original table. Lines with no source, such as the sender name,
// stay as the text on screen.
func sourceText(content string, blocks []string, lineBlock []int, a, b cell) string {
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
	var parts []string
	seen := map[int]bool{}
	var plain strings.Builder
	flush := func() {
		s := strings.Trim(plain.String(), "\n")
		plain.Reset()
		if strings.TrimSpace(s) == "" {
			return
		}
		parts = append(parts, s)
	}
	for i := a.line; i <= b.line; i++ {
		id := -1
		if i < len(lineBlock) {
			id = lineBlock[i]
		}
		if id >= 0 && id < len(blocks) {
			flush()
			if !seen[id] {
				seen[id] = true
				parts = append(parts, blocks[id])
			}
			continue
		}
		line := lines[i]
		width := ansi.StringWidth(line)
		start, end := 0, width
		if i == a.line {
			start = min(max(a.col, 0), width)
		}
		if i == b.line {
			end = cutEnd(b.col, width)
		}
		if end < start {
			end = start
		}
		if plain.Len() > 0 {
			plain.WriteByte('\n')
		}
		plain.WriteString(dropScreenPadding(line, start, end))
	}
	flush()
	return strings.Join(parts, "\n\n")
}

func (m *App) renderMapped(id, body string) (string, []int) {
	out := m.renderMarkdown(id, body)
	if out == "" {
		return "", nil
	}
	lines := strings.Split(out, "\n")
	ids := make([]int, len(lines))
	for i := range ids {
		ids[i] = -1
	}
	if body == "" || body == "…" || m.md == nil {
		return out, ids
	}
	blocks := splitMarkdown(body)
	if len(blocks) == 0 || !m.placeBlocks(lines, blocks, ids) {
		m.markAll(lines, strings.Trim(body, "\n"), ids)
	}
	return out, ids
}

func (m *App) placeBlocks(full []string, blocks []string, ids []int) bool {
	cursor := 0
	for _, block := range blocks {
		rendered, err := m.md.Render(block)
		if err != nil {
			return false
		}
		needle := contentLines(strings.TrimRight(rendered, "\n"))
		if len(needle) == 0 {
			continue
		}
		start := findLines(full, needle, cursor)
		if start < 0 {
			return false
		}
		bid := len(m.copyBlocks)
		m.copyBlocks = append(m.copyBlocks, block)
		for j := range needle {
			ids[start+j] = bid
		}
		cursor = start + len(needle)
	}
	return true
}

func (m *App) markAll(lines []string, src string, ids []int) {
	if strings.TrimSpace(src) == "" {
		return
	}
	bid := len(m.copyBlocks)
	m.copyBlocks = append(m.copyBlocks, src)
	for i, ln := range lines {
		if !blankLine(ln) {
			ids[i] = bid
		}
	}
}

func contentLines(rendered string) []string {
	lines := strings.Split(rendered, "\n")
	start, end := 0, len(lines)
	for start < end && blankLine(lines[start]) {
		start++
	}
	for end > start && blankLine(lines[end-1]) {
		end--
	}
	if start >= end {
		return nil
	}
	return lines[start:end]
}

func blankLine(s string) bool {
	return strings.TrimSpace(ansi.Strip(s)) == ""
}

func findLines(full, needle []string, from int) int {
	if len(needle) == 0 || from < 0 {
		return -1
	}
	for i := from; i+len(needle) <= len(full); i++ {
		ok := true
		for j := range needle {
			if strings.TrimSpace(ansi.Strip(full[i+j])) != strings.TrimSpace(ansi.Strip(needle[j])) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func splitMarkdown(src string) []string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	var blocks []string
	var cur []string
	flush := func() {
		s := strings.Trim(strings.Join(cur, "\n"), "\n")
		cur = nil
		if strings.TrimSpace(s) == "" {
			return
		}
		blocks = append(blocks, s)
	}
	inFence := false
	var fence string
	for _, line := range strings.Split(src, "\n") {
		if !inFence {
			if mark, ok := openingFence(line); ok {
				flush()
				inFence = true
				fence = mark
				cur = append(cur, line)
				continue
			}
			if strings.TrimSpace(line) == "" {
				flush()
				continue
			}
			cur = append(cur, line)
			continue
		}
		cur = append(cur, line)
		if closingFence(line, fence) {
			inFence = false
			flush()
		}
	}
	flush()
	return blocks
}

func openingFence(line string) (string, bool) {
	trim := strings.TrimSpace(line)
	if len(trim) < 3 {
		return "", false
	}
	mark := trim[0]
	if mark != '`' && mark != '~' {
		return "", false
	}
	n := 0
	for n < len(trim) && trim[n] == mark {
		n++
	}
	if n < 3 {
		return "", false
	}
	return strings.Repeat(string(mark), n), true
}

func closingFence(line, fence string) bool {
	trim := strings.TrimSpace(line)
	if fence == "" || len(trim) < len(fence) {
		return false
	}
	for _, r := range trim {
		if r != rune(fence[0]) {
			return false
		}
	}
	return true
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
