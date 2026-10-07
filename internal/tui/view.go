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
	main := lipgloss.JoinVertical(lipgloss.Left,
		m.headerLine(),
		m.transcriptView(),
		m.statusLine(),
		m.featureBar(),
		m.input.View(),
	)
	if !m.showSidebar {
		return main
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.sidebarView(), main)
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
	if who != "" {
		line += "  " + who
	}
	return m.styles.title.Render(clip(line, m.paneWidth()))
}

func (m App) transcriptView() string {
	if m.overlay != overlayNone {
		return m.overlayView()
	}
	return m.vp.View()
}

func (m App) statusLine() string {
	w := m.paneWidth()
	if m.quitArmed {
		return m.styles.err.Render(clip(quitHint, w))
	}
	if m.confirmDelete {
		return m.styles.err.Render(clip("delete this chat? y or any other key cancels", w))
	}
	if m.errText != "" {
		return m.styles.err.Render(clip(m.errText, w))
	}
	if m.status != "" {
		return m.styles.status.Render(clip(m.status, w))
	}
	return m.styles.help.Render(clip("enter send  alt-enter newline  esc stop  ^n new  ^f find  ^e all/client  ^k chats  ^p model  ^o features  ^t tools  ^g terminal", w))
}

func (m App) featureBar() string {
	eff := m.effectiveFeatures()
	parts := []string{
		m.chip("web", m.features.WebSearch, eff.WebSearch, "web"),
		m.chip("code", m.features.CodeInterpreter, eff.CodeInterpreter, "code"),
		m.chip("image", m.features.ImageGeneration, eff.ImageGeneration, "image"),
		m.chip("mem", m.features.Memory, eff.Memory, "memory"),
	}
	tools := "tools"
	if n := len(m.toolIDs); n > 0 {
		tools = fmt.Sprintf("tools %d", n)
	}
	parts = append(parts, m.styles.off.Render(tools))
	if m.terminalID == "" {
		parts = append(parts, m.styles.off.Render("terminal"))
	} else {
		parts = append(parts, m.styles.on.Render(clip("term "+m.terminalID, 24)))
	}
	name := m.modelName()
	if name == "" {
		name = "no model"
	}
	parts = append(parts, m.styles.dim.Render(clip(name, 28)))
	return strings.Join(parts, "  ")
}

func (m App) chip(label string, wanted, effective bool, feature string) string {
	if _, why := m.featureEnabled(feature); why != "" && !effective {
		return m.styles.disabled.Render(label)
	}
	if effective && wanted {
		return m.styles.on.Render(" " + label + " ")
	}
	if wanted && !effective {
		return m.styles.disabled.Render(label)
	}
	return m.styles.off.Render(label)
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
		line := clip(title, sidebarWidth-2)
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

func (m App) overlayView() string {
	w := m.vp.Width()
	if w < 10 {
		w = m.paneWidth()
	}
	switch m.overlay {
	case overlayHelp:
		return m.helpText(w)
	case overlayFeatures:
		return m.featuresText(w)
	case overlayRename:
		return "rename\n" + m.rename.View() + "\n" + m.styles.dim.Render("enter save, esc cancel")
	case overlayModel:
		return m.pickerText("model", false, w)
	case overlayTools:
		return m.pickerText("tools  space toggles, enter saves", true, w)
	case overlayTerminal:
		return m.pickerText("terminal", false, w)
	case overlaySearch:
		return m.pickerText("find a chat    enter opens it", false, w)
	default:
		return ""
	}
}

func (m App) helpText(w int) string {
	lines := []string{
		"keys",
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
		"A finished reply rings the terminal bell.",
		"Under Herdr it names the chat and plays the done sound.",
		"",
		"Chats started here are stored on the server.",
		"The sidebar lists only those. The web UI shows them too.",
		"",
		"Code interpreter and a terminal cannot both be on.",
		"esc closes this",
	}
	return clipLines(lines, w)
}

func (m App) featuresText(w int) string {
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
	lines := []string{"features  space toggles, esc closes", ""}
	for i, row := range rows {
		mark := "[ ]"
		if row.on {
			mark = "[x]"
		}
		line := mark + " " + row.label
		if ok, why := m.featureEnabled(row.name); !ok {
			line += "  " + why
		}
		line = clip(line, w)
		if i == m.featureCursor {
			line = m.styles.sel.Render(line)
		}
		lines = append(lines, line)
	}
	if m.terminalID != "" {
		lines = append(lines, "", "terminal is on, so code interpreter stays off")
	}
	return strings.Join(lines, "\n")
}

func (m App) pickerText(title string, multi bool, w int) string {
	lines := []string{title}
	if m.pick.filter != "" {
		lines = append(lines, "/"+m.pick.filter)
	}
	lines = append(lines, "")
	items := m.pick.visible()
	if len(items) == 0 {
		lines = append(lines, m.styles.dim.Render("nothing here"))
		return clipLines(lines, w)
	}
	for i, it := range items {
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
			label += "  " + it.note
		}
		label = clip(label, w)
		if i == m.pick.cursor {
			label = m.styles.sel.Render(label)
		}
		lines = append(lines, label)
	}
	return strings.Join(lines, "\n")
}

func clipLines(lines []string, w int) string {
	if w < 8 {
		w = 8
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(clip(line, w))
	}
	return b.String()
}

func clip(s string, n int) string {
	if n < 1 {
		return ""
	}
	// Truncate by terminal columns. Emoji and other wide characters are two
	// columns, so a rune count lets those titles draw through the sidebar border.
	return ansi.Truncate(s, n, "…")
}
