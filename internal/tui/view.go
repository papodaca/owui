package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m App) View() tea.View {
	var v tea.View
	switch m.phase {
	case phaseSplash:
		v = tea.NewView(m.splashView())
	case phaseLogin:
		if m.width == 0 || m.height == 0 {
			v = tea.NewView("owui")
		} else {
			v = tea.NewView(m.loginView())
		}
	default:
		if m.width == 0 || m.height == 0 {
			v = tea.NewView("owui")
		} else {
			v = tea.NewView(m.mainView())
		}
	}
	v.AltScreen = true
	v.WindowTitle = "owui"
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = true
	return v
}

func (m App) loginView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", m.styles.title.Render("Open WebUI"))
	fmt.Fprintf(&b, "%s\n%s\n\n", m.styles.dim.Render("server"), m.urlIn.View())
	if m.authMode == authAPIKey {
		fmt.Fprintf(&b, "%s  %s\n", m.styles.accent.Render("api key"), m.styles.dim.Render("f2 password"))
		fmt.Fprintf(&b, "%s\n%s\n", m.styles.dim.Render("key"), m.keyIn.View())
	} else {
		fmt.Fprintf(&b, "%s  %s\n", m.styles.accent.Render("password"), m.styles.dim.Render("f2 api key"))
		fmt.Fprintf(&b, "%s\n%s\n\n", m.styles.dim.Render("email"), m.emailIn.View())
		fmt.Fprintf(&b, "%s\n%s\n", m.styles.dim.Render("password"), m.passIn.View())
	}
	if m.quitArmed {
		fmt.Fprintf(&b, "\n%s\n", m.styles.err.Render(quitHint))
	} else if m.loginErr != "" {
		fmt.Fprintf(&b, "\n%s\n", m.styles.err.Render(m.loginErr))
	}
	fmt.Fprintf(&b, "\n%s\n", m.styles.help.Render("enter sign in    tab next field    ctrl+shift+v paste    ctrl+c twice to quit"))
	return b.String()
}

func (m App) mainView() string {
	screen := lipgloss.JoinVertical(lipgloss.Left,
		m.headerLine(),
		m.vp.View(),
		m.bottomBar(),
		m.input.View(),
	)
	if m.showSidebar {
		screen = lipgloss.JoinHorizontal(lipgloss.Top, m.sidebarView(), screen)
	}
	if m.overlay == overlayNone {
		return screen
	}
	box := m.modalView()
	x := max(0, (m.width-lipgloss.Width(box))/2)
	y := max(0, (m.height-lipgloss.Height(box))/2)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(screen),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}

func (m App) paneWidth() int {
	w := m.width
	if m.showSidebar {
		w -= sidebarWidth
	}
	if w < 1 {
		return 1
	}
	return w
}

func (m App) headerLine() string {
	title := m.chatTitle
	if title == "" {
		title = "new chat"
	}
	who := m.user.Name
	if who == "" {
		who = m.user.Email
	}
	line := title
	if m.openJob() != nil {
		line = m.spin.View() + "  " + line
	}
	if who != "" {
		line += "  " + who
	}
	return m.styles.title.Render(clip(line, m.paneWidth()))
}

// bottomBar shows only what is switched on, plus any status or error.
func (m App) bottomBar() string {
	w := m.paneWidth()
	if m.quitArmed {
		return m.styles.err.Render(clip(quitHint, w))
	}
	if m.confirmDelete {
		return m.styles.err.Render(clip("delete this chat? y or any other key cancels", w))
	}
	var parts []string
	switch {
	case m.errText != "":
		parts = append(parts, m.styles.err.Render(m.errText))
	case m.status != "":
		parts = append(parts, m.styles.status.Render(m.status))
	}
	eff := m.effectiveFeatures()
	for _, c := range []struct {
		label     string
		wanted    bool
		effective bool
	}{
		{"web", m.features.WebSearch, eff.WebSearch},
		{"code", m.features.CodeInterpreter, eff.CodeInterpreter},
		{"image", m.features.ImageGeneration, eff.ImageGeneration},
		{"mem", m.features.Memory, eff.Memory},
	} {
		if c.wanted {
			parts = append(parts, m.chip(c.label, c.effective))
		}
	}
	if n := len(m.toolIDs); n > 0 {
		parts = append(parts, m.styles.on.Render(fmt.Sprintf(" tools %d ", n)))
	}
	if m.terminalID != "" {
		parts = append(parts, m.styles.on.Render(" "+clip("term "+m.terminalID, 24)+" "))
	}
	name := m.modelName()
	if name == "" {
		name = "no model  ^p"
	}
	parts = append(parts, m.styles.dim.Render(clip(name, 28)))
	return clip(strings.Join(parts, "  "), w)
}

// chip draws a feature that is switched on. One the server or model cannot
// use right now is dimmed.
func (m App) chip(label string, effective bool) string {
	if effective {
		return m.styles.on.Render(" " + label + " ")
	}
	return m.styles.disabled.Render(label)
}

func (m App) sidebarView() string {
	var b strings.Builder
	if m.focus == focusSidebar {
		fmt.Fprintf(&b, "%s\n", m.styles.accent.Render("chats"))
	} else {
		fmt.Fprintf(&b, "%s\n", m.styles.title.Render("chats"))
	}
	if m.filtering {
		fmt.Fprintf(&b, "%s\n", m.styles.accent.Render(clip("/"+m.filter, sidebarWidth-3)))
	} else if m.showAll {
		fmt.Fprintf(&b, "%s\n", m.styles.dim.Render("all chats  ^e"))
	} else {
		fmt.Fprintf(&b, "%s\n", m.styles.dim.Render("this client  ^e"))
	}
	items := m.visibleChats()
	start, end, _ := m.sidebarWindow(len(items))
	if len(items) == 0 {
		b.WriteString(m.styles.dim.Render("none yet"))
		b.WriteString("\n")
	}
	for i, e := range items[start:end] {
		idx := start + i
		title := e.Title
		if title == "" {
			title = "untitled"
		}
		limit := sidebarWidth - 2
		prefix := ""
		if m.jobByChat(e.ID) != nil {
			prefix = m.spin.View() + " "
			limit -= ansi.StringWidth(prefix)
			if limit < 1 {
				limit = 1
			}
		}
		line := prefix + clip(title, limit)
		row := lipgloss.NewStyle().Inline(true)
		switch {
		case idx == m.selected && m.focus == focusSidebar:
			row = m.styles.sel.Inline(true)
		case e.ID == m.chatID:
			row = m.styles.accent.Inline(true)
		}
		b.WriteString(row.Render(line))
		b.WriteString("\n")
	}
	style := lipgloss.NewStyle().Width(sidebarWidth-1).Height(m.height).Border(lipgloss.NormalBorder(), false, true, false, false)
	if m.focus == focusSidebar {
		style = style.BorderForeground(lipgloss.Color("6"))
	}
	return style.Render(b.String())
}

// modal is what one overlay box shows. Body rows scroll: around cursor when it
// is set, otherwise from scroll.
type modal struct {
	title  string
	head   []string
	body   []string
	cursor int
	scroll int
	footer string
}

func (m App) modalView() string {
	md := m.modalContent()
	w := m.modalWidth()
	rows := m.modalRows(len(md.head))
	start, end := modalWindow(len(md.body), rows, md.cursor, md.scroll)

	lines := []string{m.styles.title.Render(clip(md.title, w))}
	for _, h := range md.head {
		lines = append(lines, clip(h, w))
	}
	for i := start; i < end; i++ {
		line := clip(md.body[i], w)
		if i == md.cursor {
			plain := ansi.Strip(line)
			line = m.styles.sel.Render(plain + strings.Repeat(" ", max(0, w-ansi.StringWidth(plain))))
		}
		lines = append(lines, line)
	}
	footer := md.footer
	if len(md.body) > rows {
		footer = fmt.Sprintf("%d-%d of %d  %s", start+1, end, len(md.body), footer)
	}
	lines = append(lines, m.styles.dim.Render(clip(footer, w)))

	body := lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(0, 1).
		Render(body)
}

func (m App) modalWidth() int {
	return max(16, min(60, m.width-6))
}

// modalRows is how many body rows fit, after the border, title, head lines,
// footer and a row of margin above and below. Long lists scroll past 12.
func (m App) modalRows(head int) int {
	return max(1, min(12, m.height-6-head))
}

func modalWindow(n, rows, cursor, scroll int) (start, end int) {
	if n <= rows {
		return 0, n
	}
	if cursor >= 0 {
		start = cursor - rows/2
	} else {
		start = scroll
	}
	start = max(0, min(start, n-rows))
	return start, start + rows
}

func (m App) modalContent() modal {
	switch m.overlay {
	case overlayHelp:
		return modal{title: "keys", body: helpLines, cursor: -1, scroll: m.helpScroll, footer: "↑↓ scroll  esc close"}
	case overlayFeatures:
		return m.featuresModal()
	case overlayRename:
		in := m.rename
		in.SetWidth(m.modalWidth() - 3)
		return modal{title: "rename chat", head: []string{in.View()}, cursor: -1, footer: "enter save  esc cancel"}
	case overlayModel:
		return m.pickerModal("model", false, "enter pick  esc close")
	case overlayTools:
		return m.pickerModal("tools", true, "space toggle  enter save  esc cancel")
	case overlayTerminal:
		return m.pickerModal("terminal", false, "enter pick  esc close")
	case overlaySearch:
		return m.pickerModal("find a chat", false, "enter open  esc close")
	default:
		return modal{cursor: -1}
	}
}

// helpScrollMax keeps the help text from scrolling past its last line.
func (m App) helpScrollMax() int {
	return max(0, len(helpLines)-m.modalRows(0))
}

var helpLines = []string{
	"enter          send",
	"alt+enter      newline",
	"esc            stop the reply",
	"ctrl+n         new chat",
	"ctrl+k         chats sidebar",
	"ctrl+f         fuzzy find a chat",
	"ctrl+e         this client or all chats",
	"ctrl+p         model",
	"ctrl+o         web, code, image, memory",
	"ctrl+t         tools and MCP servers",
	"ctrl+g         terminal",
	"tab            move focus",
	"pgup/pgdown   scroll the chat",
	"mouse wheel   scroll the chat",
	"drag          copy the chat text",
	"alt+drag      copy the markdown",
	"",
	"commands",
	"/web /code /image /memory     toggle, or add on/off",
	"/model /tools /terminal",
	"/terminal off",
	"/new /help",
	"",
	"A finished reply rings the bell, or names the chat under Herdr.",
	"It stays quiet while this window has focus.",
	"",
	"Chats started here are stored on the server.",
	"The sidebar lists only those. The web UI shows them too.",
	"",
	"Code interpreter and a terminal cannot both be on.",
}

func (m App) featuresModal() modal {
	rows := []struct {
		label string
		on    bool
		name  string
	}{
		{"web search", m.features.WebSearch, "web"},
		{"code interpreter", m.features.CodeInterpreter, "code"},
		{"image generation", m.features.ImageGeneration, "image"},
		{"memory", m.features.Memory, "memory"},
	}
	md := modal{title: "features", cursor: m.featureCursor, footer: "space toggle  esc close"}
	for _, row := range rows {
		mark := "[ ]"
		if row.on {
			mark = "[x]"
		}
		line := mark + " " + row.label
		if ok, why := m.featureEnabled(row.name); !ok {
			line += "  " + m.styles.dim.Render(why)
		}
		md.body = append(md.body, line)
	}
	if m.terminalID != "" {
		md.body = append(md.body, "", m.styles.dim.Render("terminal is on, so code interpreter stays off"))
	}
	return md
}

func (m App) pickerModal(title string, multi bool, footer string) modal {
	md := modal{title: title, cursor: m.pick.cursor, footer: footer}
	if m.pick.filter != "" {
		md.head = []string{m.styles.accent.Render("/" + m.pick.filter)}
	} else {
		md.head = []string{m.styles.dim.Render("type to filter")}
	}
	items := m.pick.visible()
	if len(items) == 0 {
		md.body = []string{m.styles.dim.Render("nothing here")}
		md.cursor = -1
		return md
	}
	for _, it := range items {
		label := it.label
		if label == "" {
			label = it.id
		}
		if multi {
			box := "[ ]"
			if m.pick.multi[it.id] {
				box = "[x]"
			}
			label = box + " " + label
		}
		if it.note != "" {
			label += "  " + m.styles.dim.Render(it.note)
		}
		md.body = append(md.body, label)
	}
	return md
}

func clip(s string, n int) string {
	if n < 1 {
		return ""
	}
	// Truncate by terminal columns. Emoji and other wide characters are two
	// columns, so a rune count lets those titles draw through the sidebar border.
	return ansi.Truncate(s, n, "…")
}
