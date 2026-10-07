package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/usr1/owui/internal/socket"
	"github.com/usr1/owui/internal/store"
)

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		if m.phase == phaseMain {
			m.syncTranscript()
		}
		return m, nil
	case quitResetMsg:
		m.quitArmed = false
		return m, nil
	case tea.MouseClickMsg:
		if m.phase == phaseMain && m.overlay == overlayNone && m.showSidebar && msg.Button == tea.MouseLeft && msg.X < sidebarWidth {
			m.focusSidebarAt(msg.Y)
			return m, nil
		}
		return m, nil
	case tea.MouseWheelMsg:
		if m.phase != phaseMain || m.overlay != overlayNone {
			return m, nil
		}
		if m.focus == focusSidebar {
			m.scrollSidebar(msg)
			return m, nil
		}
		m.scrollTranscript(msg)
		return m, nil
	case tea.PasteMsg:
		return m, m.paste(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, m.requestQuit()
		}
		m.quitArmed = false
		if m.phase == phaseSplash {
			return m, nil
		}
		if m.phase == phaseLogin {
			return m, m.updateLogin(msg)
		}
		return m, m.updateMain(msg)
	case splashFrameMsg:
		if m.phase != phaseSplash {
			return m, nil
		}
		m.frame++
		return m, spin()
	case splashHoldMsg:
		m.splashReady = true
		return m, m.releaseSplash()
	case bootMsg:
		m.bootHeld = &msg
		return m, m.releaseSplash()
	case signedInMsg:
		if msg.user.Token == "" {
			m.phase = phaseLogin
			m.loginErr = "sign-in did not return a token"
			return m, m.focusLogin()
		}
		m.cfg.Token = msg.user.Token
		m.cfg.APIKey = ""
		m.cfg.UserID = msg.user.ID
		m.loginErr = ""
		return m, m.startSplash()
	case chatsMsg:
		if !m.showAll {
			m.chats = msg.entries
			m.clampSelected()
		}
		if msg.err != nil && m.errText == "" {
			m.errText = msg.err.Error()
		}
		return m, nil
	case allChatsMsg:
		if m.showAll {
			if msg.err != nil {
				m.errText = msg.err.Error()
			} else {
				m.chats = msg.entries
				m.clampSelected()
			}
		}
		return m, nil
	case sentMsg:
		return m, m.applySent(msg)
	case loadedMsg:
		m.applyLoaded(msg)
		return m, nil
	case pollMsg:
		return m, m.poll(msg)
	case pollResult:
		return m, m.applyPoll(msg)
	case stoppedMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
		}
		m.status = "stopped"
		return m, m.finish()
	case deletedMsg:
		m.confirmDelete = false
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		if msg.id == m.chatID {
			m.chatID = ""
			m.chatTitle = ""
			m.history.Messages = nil
			m.history.CurrentID = ""
			m.syncTranscript()
		}
		m.dropVisible(msg.id)
		m.clampSelected()
		return m, nil
	case renamedMsg:
		if msg.err != nil {
			m.errText = msg.err.Error()
			return m, nil
		}
		if msg.id == m.chatID {
			m.chatTitle = msg.title
		}
		m.retitleVisible(msg.id, msg.title)
		return m, nil
	case savedMsg:
		if msg.err != nil {
			m.errText = "saving config: " + msg.err.Error()
		}
		return m, nil
	case socket.Event:
		cmd := m.onEvent(msg)
		return m, tea.Batch(cmd, waitEvent(m.sock.Events))
	}
	return m, nil
}

func (m *App) applySent(msg sentMsg) tea.Cmd {
	if msg.err != nil {
		m.generating = false
		m.pollSeq++
		m.status = ""
		m.errText = msg.err.Error()
		if msg.created {
			m.chatID = ""
			m.chatTitle = ""
			m.history.Messages = nil
			m.history.CurrentID = ""
			m.genAssistant = ""
			m.input.SetValue(msg.prompt)
			m.syncTranscript()
			return nil
		}
		if m.client != nil && m.chatID != "" {
			return m.reloadChat()
		}
		return nil
	}
	m.chatID = msg.id
	if msg.ack.ChatID != "" {
		m.chatID = msg.ack.ChatID
	}
	m.taskIDs = msg.ack.TaskIDs
	m.chatTitle = msg.title
	m.status = "working"
	if m.index != nil {
		_ = m.index.Put(m.storeKey(), store.Entry{ID: m.chatID, Title: m.chatTitle, UpdatedAt: time.Now().Unix()})
		m.upsertVisible(store.Entry{ID: m.chatID, Title: m.chatTitle, UpdatedAt: time.Now().Unix()})
	}
	return m.schedulePoll()
}

func (m *App) paste(msg tea.PasteMsg) tea.Cmd {
	if m.phase == phaseSplash {
		return nil
	}
	if m.phase == phaseLogin || m.overlay == overlayRename {
		msg.Content = strings.TrimSpace(msg.Content)
		if msg.Content == "" {
			return nil
		}
	}
	if m.phase == phaseLogin {
		return m.updateLoginInput(msg)
	}
	if m.overlay == overlayRename {
		var cmd tea.Cmd
		m.rename, cmd = m.rename.Update(msg)
		return cmd
	}
	if m.overlay == overlayNone && m.focus == focusComposer {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return cmd
	}
	return nil
}

func (m *App) updateLogin(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "f2":
		if m.authMode == authAPIKey {
			m.authMode = authPassword
		} else {
			m.authMode = authAPIKey
		}
		if m.loginField >= m.loginFields() {
			m.loginField = m.loginFields() - 1
		}
		return m.focusLogin()
	case "tab", "down":
		m.loginField = (m.loginField + 1) % m.loginFields()
		return m.focusLogin()
	case "shift+tab", "up":
		m.loginField = (m.loginField - 1 + m.loginFields()) % m.loginFields()
		return m.focusLogin()
	case "enter":
		return m.submitLogin()
	}
	return m.updateLoginInput(msg)
}

func (m App) loginFields() int {
	if m.authMode == authAPIKey {
		return 2
	}
	return 3
}

func (m *App) focusLogin() tea.Cmd {
	m.urlIn.Blur()
	m.keyIn.Blur()
	m.emailIn.Blur()
	m.passIn.Blur()
	switch m.loginField {
	case 0:
		return m.urlIn.Focus()
	case 1:
		if m.authMode == authAPIKey {
			return m.keyIn.Focus()
		}
		return m.emailIn.Focus()
	default:
		return m.passIn.Focus()
	}
}

func (m *App) updateLoginInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch m.loginField {
	case 0:
		m.urlIn, cmd = m.urlIn.Update(msg)
	case 1:
		if m.authMode == authAPIKey {
			m.keyIn, cmd = m.keyIn.Update(msg)
		} else {
			m.emailIn, cmd = m.emailIn.Update(msg)
		}
	default:
		m.passIn, cmd = m.passIn.Update(msg)
	}
	return cmd
}

func (m *App) updateMain(msg tea.KeyPressMsg) tea.Cmd {
	if m.overlay != overlayNone {
		return m.updateOverlay(msg)
	}
	if m.confirmDelete {
		switch msg.String() {
		case "y":
			return m.deleteSelected()
		default:
			m.confirmDelete = false
			return nil
		}
	}
	switch msg.String() {
	case "ctrl+n":
		m.newChat()
		return nil
	case "ctrl+k":
		// First press shows and focuses the list. The next press hides it.
		if m.focus == focusSidebar && m.showSidebar {
			m.showSidebar = false
			m.focus = focusComposer
			m.layout()
			m.syncTranscript()
			return m.input.Focus()
		}
		m.showSidebar = true
		m.focus = focusSidebar
		m.input.Blur()
		m.layout()
		m.syncTranscript()
		return nil
	case "ctrl+p":
		m.openModels()
		return nil
	case "ctrl+t":
		m.openTools()
		return nil
	case "ctrl+g":
		m.openTerminals()
		return nil
	case "ctrl+o":
		m.openFeatures()
		return nil
	case "ctrl+f":
		m.openChatSearch()
		return nil
	case "ctrl+e":
		return m.toggleChatScope()
	case "pgup", "pgdown", "ctrl+up", "ctrl+down":
		return m.pageScroll(msg)
	case "esc":
		if m.generating {
			return m.stop()
		}
		if m.focus != focusComposer {
			m.focus = focusComposer
			return m.input.Focus()
		}
		return nil
	case "tab":
		return m.cycleFocus(1)
	case "shift+tab":
		return m.cycleFocus(-1)
	}

	switch m.focus {
	case focusSidebar:
		return m.updateSidebar(msg)
	case focusChat:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return cmd
	default:
		if msg.String() == "enter" {
			m.stickToBottom = true
			return m.send()
		}
		if m.promptScroll(msg) {
			m.scrollTranscript(msg)
			return nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return cmd
	}
}

// promptScroll reports whether this key should move the transcript instead of
// the cursor. The prompt keeps its own line motion until the cursor is on the
// first or last line.
func (m App) promptScroll(msg tea.KeyPressMsg) bool {
	switch msg.Code {
	case tea.KeyPgUp, tea.KeyPgDown:
		return true
	}
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+up", "ctrl+down", "ctrl+u", "ctrl+d":
		return true
	case "up":
		return m.input.Line() == 0
	case "down":
		lines := strings.Count(m.input.Value(), "\n")
		return m.input.Line() >= lines && !m.vp.AtBottom()
	default:
		return false
	}
}

func (m *App) pageScroll(msg tea.KeyPressMsg) tea.Cmd {
	if m.focus == focusSidebar {
		m.scrollSidebar(msg)
		return nil
	}
	m.scrollTranscript(msg)
	return nil
}

func (m *App) focusSidebarAt(y int) {
	m.focus = focusSidebar
	m.input.Blur()
	items := m.visibleChats()
	start, _, _ := m.sidebarWindow(len(items))
	idx := start + (y - 2)
	if y >= 2 && idx >= 0 && idx < len(items) {
		m.selected = idx
	}
}

func (m *App) cycleFocus(dir int) tea.Cmd {
	if m.focus == focusChat {
		m.focus = focusComposer
	}
	order := []int{focusComposer}
	if m.showSidebar {
		order = []int{focusComposer, focusSidebar}
	}
	idx := 0
	for i, f := range order {
		if f == m.focus {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(order)) % len(order)
	m.focus = order[idx]
	if m.focus == focusComposer {
		return m.input.Focus()
	}
	m.input.Blur()
	return nil
}

func (m *App) updateSidebar(msg tea.KeyPressMsg) tea.Cmd {
	if m.filtering {
		switch msg.String() {
		case "esc":
			m.filtering = false
			m.filter = ""
			m.clampSelected()
			return nil
		case "backspace":
			r := []rune(m.filter)
			if len(r) > 0 {
				m.filter = string(r[:len(r)-1])
			}
			m.clampSelected()
			return nil
		case "up":
			m.selected--
			m.clampSelected()
			return nil
		case "down":
			m.selected++
			m.clampSelected()
			return nil
		case "enter":
			return m.openSelected()
		}
		if text, ok := printable(msg); ok {
			m.filter += text
			m.selected = 0
			return nil
		}
		return nil
	}
	switch msg.String() {
	case "up", "k":
		m.selected--
		m.clampSelected()
	case "down", "j":
		m.selected++
		m.clampSelected()
	case "enter":
		return m.openSelected()
	case "n":
		m.newChat()
		m.focus = focusComposer
		return m.input.Focus()
	case "d":
		if len(m.visibleChats()) > 0 {
			m.confirmDelete = true
		}
	case "r":
		items := m.visibleChats()
		m.clampSelected()
		if len(items) == 0 {
			return nil
		}
		m.rename.SetValue(items[m.selected].Title)
		m.rename.Blur()
		m.overlay = overlayRename
		return m.rename.Focus()
	case "/":
		m.filtering = true
		m.filter = ""
		m.status = "type to filter, esc clears"
	}
	return nil
}

func printable(msg tea.KeyPressMsg) (string, bool) {
	if msg.Mod != 0 || msg.Text == "" {
		return "", false
	}
	return msg.Text, true
}

func (m *App) updateOverlay(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "esc" {
		m.overlay = overlayNone
		m.focus = focusComposer
		return m.input.Focus()
	}
	switch m.overlay {
	case overlayHelp:
		if msg.String() == "enter" || msg.String() == "q" {
			m.overlay = overlayNone
			return m.input.Focus()
		}
		return nil
	case overlayFeatures:
		switch msg.String() {
		case "up", "k":
			if m.featureCursor > 0 {
				m.featureCursor--
			}
		case "down", "j":
			if m.featureCursor < 3 {
				m.featureCursor++
			}
		case " ", "enter":
			return m.toggleFeatureCursor()
		}
		return nil
	case overlayRename:
		if msg.String() == "enter" {
			return m.renameSelected()
		}
		var cmd tea.Cmd
		m.rename, cmd = m.rename.Update(msg)
		return cmd
	default:
		return m.updatePicker(msg)
	}
}

func (m *App) updatePicker(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "up", "ctrl+p":
		m.movePicker(-1)
		return nil
	case "down", "ctrl+n":
		m.movePicker(1)
		return nil
	case "backspace":
		r := []rune(m.pick.filter)
		if len(r) > 0 {
			m.pick.filter = string(r[:len(r)-1])
		}
		m.pick.cursor = 0
		return nil
	case " ":
		if m.overlay == overlayTools {
			items := m.pick.visible()
			if m.pick.cursor >= 0 && m.pick.cursor < len(items) {
				id := items[m.pick.cursor].id
				if m.pick.multi == nil {
					m.pick.multi = map[string]bool{}
				}
				m.pick.multi[id] = !m.pick.multi[id]
			}
			return nil
		}
	case "enter":
		return m.acceptPicker()
	}
	if text, ok := printable(msg); ok {
		m.pick.filter += text
		m.pick.cursor = 0
	}
	return nil
}
