package tui

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/usr1/owui/internal/api"
	"github.com/usr1/owui/internal/chattree"
	"github.com/usr1/owui/internal/socket"
)

func (m *App) submitLogin() tea.Cmd {
	m.cfg.URL = strings.TrimRight(strings.TrimSpace(m.urlIn.Value()), "/")
	if m.cfg.URL == "" {
		m.loginErr = "url is required"
		return nil
	}
	if m.authMode == authAPIKey {
		key := strings.TrimSpace(m.keyIn.Value())
		if key == "" {
			m.loginErr = "api key is required"
			return nil
		}
		m.cfg.APIKey = key
		m.cfg.Token = ""
	} else {
		email := strings.TrimSpace(m.emailIn.Value())
		pass := m.passIn.Value()
		if email == "" || pass == "" {
			m.loginErr = "email and password are required"
			return nil
		}
		base := m.cfg.URL
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			client := api.New(base, "")
			user, err := client.SignIn(ctx, email, pass)
			if err != nil {
				return bootMsg{err: err}
			}
			return signedInMsg{user: user}
		}
	}
	m.loginErr = ""
	return m.startSplash()
}

type signedInMsg struct{ user api.User }

func (m *App) newChat() {
	if m.generating {
		m.errText = "stop the reply before opening another chat"
		return
	}
	m.chatID = ""
	m.chatTitle = ""
	m.history = api.History{}
	m.stickToBottom = true
	m.genAssistant = ""
	m.taskIDs = nil
	m.status = ""
	m.errText = ""
	m.confirmDelete = false
	m.syncTranscript()
}

func (m *App) send() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	if handled, cmd := m.slash(text); handled {
		return cmd
	}
	if m.generating {
		m.errText = "a reply is still running"
		return nil
	}
	if m.modelID == "" {
		m.errText = "pick a model first (^p)"
		return nil
	}
	if m.client == nil {
		m.errText = "not signed in"
		return nil
	}
	m.errText = ""
	m.status = "sending"
	now := time.Now().Unix()
	modelName := m.modelName()
	var (
		turn     chattree.Turn
		created  bool
		body     any
		parentID *string
		chatID   string
	)
	if m.chatID == "" {
		created = true
		var chat api.ChatBody
		turn, chat = chattree.CreateBody(chattree.TitleFromPrompt(text), m.modelID, modelName, text, now)
		body = chat
		m.history = *chat.History
		m.chatTitle = chat.Title
	} else {
		chatID = m.chatID
		prev := m.history.CurrentID
		// The completion request saves this turn. Do not PATCH history here:
		// the server replaces a message object wholesale, so a partial update
		// would wipe the previous reply.
		turn, _ = chattree.AppendPatch(prev, m.modelID, modelName, text, now)
		parentID = &prev
		if msg, ok := m.history.Messages[prev]; ok {
			msg.ChildrenIDs = append(api.IDList{}, msg.ChildrenIDs...)
			msg.ChildrenIDs = append(msg.ChildrenIDs, turn.UserID)
			m.history.Messages[prev] = msg
		}
		if m.history.Messages == nil {
			m.history.Messages = map[string]api.Message{}
		}
		m.history.Messages[turn.UserID] = turn.User
		m.history.Messages[turn.AssistantID] = turn.Assistant
		m.history.CurrentID = turn.AssistantID
	}
	userMessage := turn.User
	m.generating = true
	m.genAssistant = turn.AssistantID
	m.genStart = time.Now()
	m.taskIDs = nil
	m.pollSeq++
	m.input.SetValue("")
	m.syncTranscript()

	req := api.CompletionRequest{
		Model:      m.modelID,
		Messages:   chattree.CompletionMessages(m.history),
		Stream:     true,
		ID:         turn.AssistantID,
		SessionID:  sessionID(m.sock),
		Features:   m.effectiveFeatures(),
		ToolIDs:    append([]string{}, m.toolIDs...),
		TerminalID: m.terminalID,
		BackgroundTasks: api.BackgroundTasks{
			TitleGeneration:    true,
			TagsGeneration:     false,
			FollowUpGeneration: false,
		},
		Variables:   variables(),
		ParentID:    parentID,
		UserMessage: &userMessage,
	}
	if len(req.ToolIDs) == 0 {
		req.ToolIDs = nil
	}
	client := m.client
	work := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		id := chatID
		if created {
			rec, err := client.CreateChat(ctx, body)
			if err != nil {
				return sentMsg{created: true, prompt: text, assistant: turn.AssistantID, err: err}
			}
			id = rec.ID
			if id == "" {
				return sentMsg{created: true, prompt: text, assistant: turn.AssistantID, err: errEmptyID}
			}
		}
		req.ChatID = id
		ack, err := client.Complete(ctx, req)
		return sentMsg{
			id:        id,
			title:     chattree.TitleFromPrompt(text),
			created:   created,
			assistant: turn.AssistantID,
			ack:       ack,
			prompt:    text,
			err:       err,
		}
	}
	return tea.Batch(work, m.spinTick())
}

func (m App) spinTick() tea.Cmd {
	return func() tea.Msg { return m.spin.Tick() }
}

type stringError string

func (e stringError) Error() string { return string(e) }

const errEmptyID stringError = "server did not return a chat id"

func (m *App) slash(text string) (bool, tea.Cmd) {
	if !strings.HasPrefix(text, "/") {
		return false, nil
	}
	fields := strings.Fields(text)
	cmd := fields[0]
	args := fields[1:]
	switch cmd {
	case "/help":
		m.overlay = overlayHelp
	case "/new":
		m.newChat()
	case "/web":
		m.input.SetValue("")
		return true, m.setFeature("web", toggle(args, m.features.WebSearch))
	case "/code":
		m.input.SetValue("")
		return true, m.setFeature("code", toggle(args, m.features.CodeInterpreter))
	case "/image":
		m.input.SetValue("")
		return true, m.setFeature("image", toggle(args, m.features.ImageGeneration))
	case "/memory":
		m.input.SetValue("")
		return true, m.setFeature("memory", toggle(args, m.features.Memory))
	case "/model":
		m.openModels()
	case "/tools":
		m.openTools()
	case "/terminal":
		if len(args) > 0 && (args[0] == "off" || args[0] == "none") {
			m.terminalID = ""
			m.input.SetValue("")
			m.status = "terminal off"
			return true, m.persist()
		}
		m.openTerminals()
	default:
		return false, nil
	}
	m.input.SetValue("")
	return true, nil
}

func toggle(args []string, cur bool) bool {
	if len(args) == 0 {
		return !cur
	}
	switch strings.ToLower(args[0]) {
	case "on", "true", "1":
		return true
	case "off", "false", "0":
		return false
	default:
		return !cur
	}
}

func (m *App) setFeature(name string, on bool) tea.Cmd {
	if on {
		if ok, why := m.featureEnabled(name); !ok && !(name == "code" && m.terminalID != "") {
			// featureEnabled reports code as blocked while a terminal is on.
			// Turning code on should clear the terminal instead of refusing.
			if name != "code" {
				m.errText = why
				return nil
			}
		}
	}
	switch name {
	case "web":
		m.features.WebSearch = on
	case "code":
		m.features.CodeInterpreter = on
		if on {
			m.terminalID = ""
		}
	case "image":
		m.features.ImageGeneration = on
	case "memory":
		m.features.Memory = on
	}
	if on {
		if ok, why := m.featureEnabled(name); !ok {
			switch name {
			case "web":
				m.features.WebSearch = false
			case "code":
				m.features.CodeInterpreter = false
			case "image":
				m.features.ImageGeneration = false
			case "memory":
				m.features.Memory = false
			}
			m.errText = why
			return nil
		}
	}
	m.errText = ""
	m.status = name + " " + onOff(on)
	return m.persist()
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func (m *App) openModels() {
	items := make([]pickerItem, 0, len(m.models))
	cursor := 0
	for i, model := range m.models {
		note := ""
		if model.TerminalID != "" {
			note = "terminal " + model.TerminalID
		}
		items = append(items, pickerItem{id: model.ID, label: model.Name, note: note})
		if model.ID == m.modelID {
			cursor = i
		}
	}
	if len(items) == 0 && m.modelID != "" {
		items = append(items, pickerItem{id: m.modelID, label: m.modelID})
	}
	m.pick = picker{items: items, cursor: cursor}
	m.overlay = overlayModel
}

func (m *App) openTools() {
	items := make([]pickerItem, len(m.tools))
	multi := map[string]bool{}
	for _, id := range m.toolIDs {
		multi[id] = true
	}
	for i, tool := range m.tools {
		label := tool.Name
		if label == "" {
			label = tool.ID
		}
		items[i] = pickerItem{id: tool.ID, label: label}
	}
	m.pick = picker{items: items, multi: multi}
	m.overlay = overlayTools
}

func (m *App) openTerminals() {
	items := []pickerItem{{id: "", label: "None"}}
	cursor := 0
	for _, term := range m.terminals {
		label := term.Name
		if label == "" {
			label = term.ID
		}
		if term.ID == m.terminalID {
			cursor = len(items)
		}
		items = append(items, pickerItem{id: term.ID, label: label})
	}
	m.pick = picker{items: items, cursor: cursor}
	m.overlay = overlayTerminal
}

func (m *App) openFeatures() {
	m.overlay = overlayFeatures
	m.featureCursor = 0
}

func (m *App) acceptPicker() tea.Cmd {
	items := m.pick.visible()
	switch m.overlay {
	case overlayTools:
		var ids []string
		for _, tool := range m.tools {
			if m.pick.multi[tool.ID] {
				ids = append(ids, tool.ID)
			}
		}
		m.toolIDs = ids
		m.overlay = overlayNone
		m.status = "tools updated"
		return m.persist()
	case overlayModel:
		if len(items) == 0 || m.pick.cursor < 0 || m.pick.cursor >= len(items) {
			return nil
		}
		id := items[m.pick.cursor].id
		m.modelID = id
		m.applyModelDefaults(m.modelByID(id))
		m.overlay = overlayNone
		m.status = "model " + m.modelName()
		return m.persist()
	case overlaySearch:
		if len(items) == 0 || m.pick.cursor < 0 || m.pick.cursor >= len(items) {
			return nil
		}
		id := items[m.pick.cursor].id
		m.overlay = overlayNone
		m.focus = focusComposer
		return tea.Batch(m.openChatID(id), m.input.Focus())
	case overlayTerminal:
		if len(items) == 0 || m.pick.cursor < 0 || m.pick.cursor >= len(items) {
			return nil
		}
		m.terminalID = items[m.pick.cursor].id
		if m.terminalID != "" {
			m.features.CodeInterpreter = false
		}
		m.overlay = overlayNone
		if m.terminalID == "" {
			m.status = "terminal off"
		} else {
			m.status = "terminal " + m.terminalID
		}
		return m.persist()
	default:
		m.overlay = overlayNone
		return nil
	}
}

func (m *App) toggleFeatureCursor() tea.Cmd {
	names := []string{"web", "code", "image", "memory"}
	if m.featureCursor < 0 || m.featureCursor >= len(names) {
		return nil
	}
	name := names[m.featureCursor]
	var cur bool
	switch name {
	case "web":
		cur = m.features.WebSearch
	case "code":
		cur = m.features.CodeInterpreter
	case "image":
		cur = m.features.ImageGeneration
	case "memory":
		cur = m.features.Memory
	}
	return m.setFeature(name, !cur)
}

func (m *App) reloadChat() tea.Cmd {
	if m.client == nil || m.chatID == "" {
		return nil
	}
	client := m.client
	id := m.chatID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rec, err := client.GetChat(ctx, id)
		return loadedMsg{rec: rec, why: "reload", err: err}
	}
}

func (m *App) openChatSearch() {
	items := make([]pickerItem, 0, len(m.chats))
	for _, chat := range m.chats {
		label := chat.Title
		if label == "" {
			label = "untitled"
		}
		items = append(items, pickerItem{id: chat.ID, label: label})
	}
	m.pick = picker{items: items, fuzzy: true}
	m.overlay = overlaySearch
	m.input.Blur()
}

func (m *App) openSelected() tea.Cmd {
	items := m.visibleChats()
	m.clampSelected()
	if len(items) == 0 {
		return nil
	}
	return m.openChatID(items[m.selected].ID)
}

func (m *App) openChatID(id string) tea.Cmd {
	if m.generating {
		m.errText = "stop the reply before opening another chat"
		return nil
	}
	if m.client == nil || id == "" {
		return nil
	}
	for i, chat := range m.chats {
		if chat.ID == id {
			m.selected = i
			break
		}
	}
	m.filtering = false
	m.filter = ""
	m.stickToBottom = true
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rec, err := client.GetChat(ctx, id)
		return loadedMsg{rec: rec, why: "open", err: err}
	}
}

func (m *App) applyLoaded(msg loadedMsg) tea.Cmd {
	if msg.err != nil {
		m.errText = msg.err.Error()
		if msg.why == "done" {
			m.generating = false
			m.status = ""
		}
		return nil
	}
	m.chatID = msg.rec.ID
	m.chatTitle = msg.rec.DisplayTitle()
	if msg.rec.Chat.History != nil {
		m.history = *msg.rec.Chat.History
	}
	var cmd tea.Cmd
	if msg.why == "done" {
		m.generating = false
		m.genAssistant = ""
		m.taskIDs = nil
		m.status = ""
		m.pollSeq++
		if msg.chime {
			cmd = doneChime(m.chatTitle, replySnippet(m.history))
		}
	}
	if len(msg.rec.Chat.Models) > 0 && msg.why == "open" {
		m.modelID = msg.rec.Chat.Models[0]
	}
	m.syncTranscript()
	if m.chatID != "" {
		m.retitleVisible(m.chatID, m.chatTitle)
	}
	return cmd
}

func (m *App) schedulePoll() tea.Cmd {
	if !m.generating || m.chatID == "" {
		return nil
	}
	id := m.chatID
	seq := m.pollSeq
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return pollMsg{id: id, seq: seq}
	})
}

func (m *App) poll(msg pollMsg) tea.Cmd {
	if !m.generating || msg.seq != m.pollSeq || msg.id != m.chatID {
		return nil
	}
	if time.Since(m.genStart) > 15*time.Minute {
		m.generating = false
		m.errText = "timed out waiting for the reply"
		m.status = ""
		return nil
	}
	client := m.client
	id := m.chatID
	asst := m.genAssistant
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		tasks, err := client.ChatTasks(ctx, id)
		rec, recErr := client.GetChat(ctx, id)
		return pollResult{id: id, tasks: tasks, rec: rec, tasksErr: err, recErr: recErr, assistant: asst}
	}
}

type pollResult struct {
	id        string
	assistant string
	tasks     []string
	rec       api.ChatRecord
	tasksErr  error
	recErr    error
}

func (m *App) applyPoll(msg pollResult) tea.Cmd {
	if !m.generating || msg.id != m.chatID {
		return m.schedulePoll()
	}
	if msg.tasksErr == nil {
		m.taskIDs = msg.tasks
	}
	if msg.recErr == nil && msg.rec.Chat.History != nil {
		m.history = mergeHistory(m.history, *msg.rec.Chat.History)
		if title := msg.rec.DisplayTitle(); title != "" && title != "New Chat" {
			m.chatTitle = title
		}
		m.syncTranscript()
	}
	if done, ok := assistantDone(m.history, m.genAssistant); ok && done {
		m.pendingChime = true
		return m.finish()
	}
	if msg.tasksErr == nil && len(msg.tasks) == 0 {
		if msg, ok := m.history.Messages[m.genAssistant]; ok && msg.Content != "" {
			m.pendingChime = true
			return m.finish()
		}
	}
	return m.schedulePoll()
}

func assistantDone(h api.History, id string) (bool, bool) {
	msg, ok := h.Messages[id]
	if !ok || msg.Done == nil {
		return false, false
	}
	return *msg.Done, true
}

func mergeHistory(local, remote api.History) api.History {
	if local.Messages == nil {
		return remote
	}
	for id, rm := range remote.Messages {
		lm, ok := local.Messages[id]
		if ok && len(lm.Content) > len(rm.Content) {
			rm.Content = lm.Content
		}
		remote.Messages[id] = rm
	}
	if remote.CurrentID == "" {
		remote.CurrentID = local.CurrentID
	}
	return remote
}

func (m *App) finish() tea.Cmd {
	chime := m.pendingChime
	m.pendingChime = false
	if m.client == nil || m.chatID == "" {
		m.generating = false
		if chime {
			return doneChime(m.chatTitle, replySnippet(m.history))
		}
		return nil
	}
	m.pollSeq++
	client := m.client
	id := m.chatID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rec, err := client.GetChat(ctx, id)
		return loadedMsg{rec: rec, why: "done", err: err, chime: chime}
	}
}

func (m *App) stop() tea.Cmd {
	if !m.generating {
		return nil
	}
	client := m.client
	ids := append([]string{}, m.taskIDs...)
	chatID := m.chatID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if len(ids) == 0 && chatID != "" {
			ids, _ = client.ChatTasks(ctx, chatID)
		}
		var first error
		for _, id := range ids {
			if err := client.StopTask(ctx, id); err != nil && first == nil {
				first = err
			}
		}
		return stoppedMsg{err: first}
	}
}

func (m *App) onEvent(ev socket.Event) tea.Cmd {
	if ev.Local {
		if ev.Type == "socket:disconnected" && ev.Err != "" {
			m.status = "socket down"
		}
		if ev.Type == "socket:connected" && m.status == "socket down" {
			m.status = ""
		}
		return nil
	}
	switch ev.Type {
	case "chat:title":
		var data struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(ev.Data, &data)
		if data.Title == "" {
			_ = json.Unmarshal(ev.Data, &data.Title)
		}
		if data.Title != "" && (ev.ChatID == "" || ev.ChatID == m.chatID) {
			m.chatTitle = data.Title
			if m.chatID != "" {
				m.retitleVisible(m.chatID, data.Title)
			}
		}
	case "status":
		var data struct {
			Description string `json:"description"`
			Done        bool   `json:"done"`
		}
		_ = json.Unmarshal(ev.Data, &data)
		if ev.ChatID == "" || ev.ChatID == m.chatID {
			if data.Done {
				if m.status == data.Description {
					m.status = ""
				}
			} else if data.Description != "" {
				m.status = data.Description
			}
		}
	case "chat:message:error":
		m.errText = errorText(ev.Data)
		m.generating = false
		m.status = ""
		m.pollSeq++
	case "message":
		m.appendContent(ev, true)
	case "replace", "chat:completion":
		m.appendContent(ev, false)
		var data struct {
			Done bool `json:"done"`
		}
		_ = json.Unmarshal(ev.Data, &data)
		if data.Done && (ev.ChatID == "" || ev.ChatID == m.chatID) {
			m.pendingChime = true
			return m.finish()
		}
	}
	return nil
}

func (m *App) appendContent(ev socket.Event, delta bool) {
	if ev.MessageID == "" {
		return
	}
	if ev.ChatID != "" && m.chatID != "" && ev.ChatID != m.chatID {
		return
	}
	msg, ok := m.history.Messages[ev.MessageID]
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(ev.Data, &raw); err != nil {
		return
	}
	contentRaw, ok := raw["content"]
	if !ok {
		return
	}
	var text string
	if err := json.Unmarshal(contentRaw, &text); err != nil {
		return
	}
	if delta {
		msg.Content += api.Content(text)
	} else {
		msg.Content = api.Content(text)
	}
	if m.history.Messages == nil {
		m.history.Messages = map[string]api.Message{}
	}
	m.history.Messages[ev.MessageID] = msg
	m.syncTranscript()
}

func errorText(raw json.RawMessage) string {
	var wrap struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil || wrap.Error == nil {
		if len(raw) == 0 {
			return "chat error"
		}
		return string(raw)
	}
	switch v := wrap.Error.(type) {
	case string:
		return v
	case map[string]any:
		if s, ok := v["content"].(string); ok && s != "" {
			return s
		}
		b, _ := json.Marshal(v)
		return string(b)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func (m *App) deleteSelected() tea.Cmd {
	items := m.visibleChats()
	m.clampSelected()
	if len(items) == 0 || m.client == nil {
		m.confirmDelete = false
		return nil
	}
	e := items[m.selected]
	if e.ID == m.chatID && m.generating {
		m.errText = "stop the reply before deleting this chat"
		m.confirmDelete = false
		return nil
	}
	client := m.client
	index := m.index
	key := m.storeKey()
	id := e.ID
	current := m.chatID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := client.DeleteChat(ctx, id)
		if err == nil && index != nil {
			_ = index.Remove(key, id)
		}
		return deletedMsg{id: id, current: current, err: err}
	}
}

type deletedMsg struct {
	id      string
	current string
	err     error
}

func (m *App) renameSelected() tea.Cmd {
	title := strings.TrimSpace(m.rename.Value())
	m.overlay = overlayNone
	items := m.visibleChats()
	m.clampSelected()
	if title == "" || len(items) == 0 || m.client == nil {
		return nil
	}
	id := items[m.selected].ID
	client := m.client
	index := m.index
	key := m.storeKey()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, err := client.UpdateChat(ctx, id, map[string]string{"title": title})
		if err == nil && index != nil {
			_ = index.Rename(key, id, title)
		}
		return renamedMsg{id: id, title: title, err: err}
	}
}

type renamedMsg struct {
	id    string
	title string
	err   error
}
