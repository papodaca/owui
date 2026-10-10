package tui

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/papodaca/owui/internal/api"
	"github.com/papodaca/owui/internal/chattree"
	"github.com/papodaca/owui/internal/socket"
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

func (m *App) newChat() tea.Cmd {
	prev := m.chatID
	m.viewToken++
	m.chatID = ""
	m.chatTitle = ""
	m.history = api.History{}
	m.stickToBottom = true
	m.status = ""
	m.errText = ""
	m.confirmDelete = false
	m.syncTranscript()
	return m.markRead(prev)
}

func (m *App) send() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	if handled, cmd := m.slash(text); handled {
		return cmd
	}
	if m.openJob() != nil {
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
	m.ensureJobs()
	first := len(m.jobs) == 0
	job := &genJob{assistant: turn.AssistantID, started: time.Now(), seq: 1}
	if m.chatID == "" {
		job.token = m.viewToken
	} else {
		job.chatID = m.chatID
	}
	m.jobs[job.assistant] = job
	m.input.SetValue("")
	m.syncTranscript()

	messages := chattree.CompletionMessages(m.history)
	req := api.CompletionRequest{
		Model:      m.modelID,
		Messages:   messages,
		Stream:     true,
		ID:         turn.AssistantID,
		SessionID:  sessionID(m.sock),
		Features:   m.effectiveFeatures(),
		ToolIDs:    append([]string{}, m.toolIDs...),
		TerminalID: m.terminalID,
		BackgroundTasks: api.BackgroundTasks{
			TitleGeneration: generateTitle(messages),
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
	if first {
		return tea.Batch(work, m.spinTick())
	}
	return work
}

// generateTitle matches the web client: a name is requested only while the
// completion is still the opening turn, one user message or a system message
// plus that user message. Later rounds leave the title alone.
func generateTitle(msgs []api.CompletionMessage) bool {
	if len(msgs) == 1 {
		return true
	}
	return len(msgs) == 2 && msgs[0].Role == "system" && msgs[1].Role == "user"
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
		m.helpScroll = 0
	case "/new":
		mark := m.newChat()
		m.input.SetValue("")
		return true, mark
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
	index := m.index
	key := m.storeKey()
	prev := m.chatID
	fetch := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rec, err := client.GetChat(ctx, id)
		if api.IsNotFound(err) {
			if index != nil {
				_ = index.Remove(key, id)
			}
			return deletedMsg{id: id, gone: true}
		}
		return loadedMsg{rec: rec, why: "open", err: err}
	}
	if prev != "" && prev != id {
		return tea.Batch(m.markRead(prev), fetch)
	}
	return fetch
}

func (m *App) markRead(id string) tea.Cmd {
	if id == "" || m.sock == nil {
		return nil
	}
	sock := m.sock
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sock.MarkRead(ctx, id)
		return nil
	}
}

func (m *App) applyLoaded(msg loadedMsg) tea.Cmd {
	if msg.err != nil {
		m.errText = msg.err.Error()
		if msg.why == "done" {
			if j := m.jobByChat(msg.rec.ID); j != nil {
				m.dropJob(j.assistant)
			}
			m.status = ""
		}
		return nil
	}
	var cmd tea.Cmd
	if msg.why == "done" {
		if j := m.jobByChat(msg.rec.ID); j != nil {
			m.dropJob(j.assistant)
		}
		m.status = ""
		title := msg.rec.DisplayTitle()
		hist := m.history
		if msg.rec.Chat.History != nil {
			hist = *msg.rec.Chat.History
		}
		if msg.chime {
			cmd = doneChime(title, replySnippet(hist), m.termFocused)
		}
		if msg.why == "done" && msg.rec.ID != "" && msg.rec.ID != m.chatID {
			m.retitleVisible(msg.rec.ID, title)
			return cmd
		}
	}
	m.chatID = msg.rec.ID
	m.chatTitle = msg.rec.DisplayTitle()
	if msg.rec.Chat.History != nil {
		m.history = *msg.rec.Chat.History
	}
	if len(msg.rec.Chat.Models) > 0 && msg.why == "open" {
		m.modelID = msg.rec.Chat.Models[0]
	}
	m.syncTranscript()
	if m.chatID != "" {
		m.retitleVisible(m.chatID, m.chatTitle)
	}
	if msg.why == "open" {
		cmd = tea.Batch(cmd, m.adoptRunning())
	}
	return tea.Batch(cmd, m.markRead(m.chatID))
}

// adoptRunning follows a reply that is still being written but was not started
// here, such as one from the web UI, so the chat keeps updating once opened.
func (m *App) adoptRunning() tea.Cmd {
	cur, ok := m.history.Messages[m.history.CurrentID]
	if m.chatID == "" || !ok || cur.Role != "assistant" || cur.Done == nil || *cur.Done {
		return nil
	}
	if m.jobByChat(m.chatID) != nil {
		return nil
	}
	m.ensureJobs()
	first := len(m.jobs) == 0
	j := &genJob{chatID: m.chatID, assistant: cur.ID, started: time.Now(), seq: 1, adopted: true}
	m.jobs[j.assistant] = j
	if first {
		return tea.Batch(m.schedulePoll(j), m.spinTick())
	}
	return m.schedulePoll(j)
}

func (m *App) schedulePoll(j *genJob) tea.Cmd {
	if j == nil || j.chatID == "" || m.jobs[j.assistant] == nil {
		return nil
	}
	id, asst, seq := j.chatID, j.assistant, j.seq
	return tea.Tick(pollEvery(j), func(time.Time) tea.Msg {
		return pollMsg{id: id, assistant: asst, seq: seq}
	})
}

// pollEvery is the gap between saved-chat checks. The socket streams the
// reply and polling only backs it up, so it is slow while events arrive.
func pollEvery(j *genJob) time.Duration {
	if time.Since(j.lastEvent) < 10*time.Second {
		return 5 * time.Second
	}
	return time.Second
}

func (m *App) poll(msg pollMsg) tea.Cmd {
	j := m.jobs[msg.assistant]
	if j == nil || j.seq != msg.seq || j.chatID != msg.id {
		return nil
	}
	if time.Since(j.started) > 15*time.Minute {
		open := m.openJob() == j
		m.dropJob(j.assistant)
		if open {
			m.errText = "timed out waiting for the reply"
			m.status = ""
		}
		return nil
	}
	client := m.client
	id := j.chatID
	asst := j.assistant
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
	j := m.jobs[msg.assistant]
	if j == nil || j.chatID != msg.id {
		return nil
	}
	if msg.tasksErr == nil {
		j.taskIDs = msg.tasks
	}
	viewing := msg.id == m.chatID
	hist := api.History{}
	if msg.recErr == nil && msg.rec.Chat.History != nil {
		hist = *msg.rec.Chat.History
		if title := msg.rec.DisplayTitle(); title != "" && title != "New Chat" {
			if viewing {
				m.chatTitle = title
			}
			m.retitleVisible(msg.id, title)
		}
		if viewing {
			m.history = mergeHistory(m.history, hist)
			m.syncTranscript()
		}
	} else if viewing {
		hist = m.history
	}
	if hist.Messages != nil {
		if done, ok := assistantDone(hist, j.assistant); ok && done {
			j.chime = true
			return m.finish(j)
		}
	}
	if msg.tasksErr == nil && len(msg.tasks) == 0 {
		if j.adopted {
			// No task on the server: the reply died without being marked done.
			return m.finish(j)
		}
		if msg, ok := hist.Messages[j.assistant]; ok && msg.Content != "" {
			j.chime = true
			return m.finish(j)
		}
	}
	return m.schedulePoll(j)
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
		// The socket can be ahead of what the server has saved so far.
		if ok && outputTextLen(lm.Output) > outputTextLen(rm.Output) {
			rm.Output = lm.Output
		}
		remote.Messages[id] = rm
	}
	if remote.CurrentID == "" {
		remote.CurrentID = local.CurrentID
	}
	return remote
}

func (m *App) finish(j *genJob) tea.Cmd {
	if j == nil || m.jobs[j.assistant] == nil {
		return nil
	}
	chime := j.chime
	id := j.chatID
	m.dropJob(j.assistant)
	if m.client == nil || id == "" {
		if chime {
			return doneChime(m.chatTitle, replySnippet(m.history), m.termFocused)
		}
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rec, err := client.GetChat(ctx, id)
		return loadedMsg{rec: rec, why: "done", err: err, chime: chime}
	}
}

func (m *App) stop() tea.Cmd {
	j := m.openJob()
	if j == nil {
		return nil
	}
	client := m.client
	ids := append([]string{}, j.taskIDs...)
	chatID := j.chatID
	assistant := j.assistant
	m.dropJob(assistant)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if len(ids) == 0 && chatID != "" && client != nil {
			ids, _ = client.ChatTasks(ctx, chatID)
		}
		var first error
		for _, id := range ids {
			if err := client.StopTask(ctx, id); err != nil && first == nil {
				first = err
			}
		}
		return stoppedMsg{id: chatID, assistant: assistant, err: first}
	}
}

func (m *App) onEvent(ev socket.Event) tea.Cmd {
	if ev.Local {
		if ev.Type == "socket:disconnected" {
			m.sockDropped = true
			if ev.Err != "" {
				m.status = "socket down"
			}
		}
		if ev.Type == "socket:connected" {
			if m.status == "socket down" {
				m.status = ""
			}
			if m.sockDropped && m.showAll {
				// Chats made while the socket was down sent no event.
				m.sockDropped = false
				return m.requestChatList()
			}
		}
		return nil
	}
	if m.showAll && chatListChanged(ev) {
		return tea.Batch(m.requestChatList(), m.handleEvent(ev))
	}
	return m.handleEvent(ev)
}

// chatListChanged reports events after which the server's chat list differs:
// a chat was created (chat:list, sent only by newer servers), titled, or got
// a finished reply, which moves it to the top. The server also sends chat:list
// when any client marks a chat read; that one carries last_read_at.
func chatListChanged(ev socket.Event) bool {
	switch ev.Type {
	case "chat:list":
		var data struct {
			LastReadAt *float64 `json:"last_read_at"`
		}
		_ = json.Unmarshal(ev.Data, &data)
		return data.LastReadAt == nil
	case "chat:title":
		return true
	case "replace", "chat:completion":
		var data struct {
			Done bool `json:"done"`
		}
		_ = json.Unmarshal(ev.Data, &data)
		return data.Done
	}
	return false
}

// requestChatList refetches the sidebar for the current scope. A request made
// while one is in flight runs once that one lands, so a burst of events costs
// at most two fetches.
func (m *App) requestChatList() tea.Cmd {
	if m.listFetching {
		m.listDirty = true
		return nil
	}
	var cmd tea.Cmd
	if m.showAll {
		cmd = m.fetchAllChats()
	} else {
		cmd = m.refreshChats()
	}
	m.listFetching = cmd != nil
	return cmd
}

// chatListLanded finishes a fetch and starts the one that was asked for
// while it ran.
func (m *App) chatListLanded() tea.Cmd {
	m.listFetching = false
	if !m.listDirty {
		return nil
	}
	m.listDirty = false
	return m.requestChatList()
}

func (m *App) handleEvent(ev socket.Event) tea.Cmd {
	switch ev.Type {
	case "chat:title":
		var data struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(ev.Data, &data)
		if data.Title == "" {
			_ = json.Unmarshal(ev.Data, &data.Title)
		}
		if data.Title != "" && (ev.ChatID == "" || ev.ChatID == m.chatID || m.jobByChat(ev.ChatID) != nil) {
			id := ev.ChatID
			if id == "" {
				id = m.chatID
			}
			if id == "" || id == m.chatID {
				m.chatTitle = data.Title
			}
			if id != "" {
				m.retitleVisible(id, data.Title)
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
		j := m.jobByChat(ev.ChatID)
		if ev.ChatID == "" {
			j = m.openJob()
		}
		if ev.ChatID != "" && j == nil && ev.ChatID != m.chatID {
			return nil
		}
		if j != nil {
			m.dropJob(j.assistant)
		}
		m.errText = errorText(ev.Data)
		m.status = ""
	case "message":
		m.appendContent(ev, true)
	case "response:completion":
		m.applyResponseEvent(ev)
	case "replace", "chat:completion":
		m.appendContent(ev, false)
		var data struct {
			Done bool `json:"done"`
		}
		_ = json.Unmarshal(ev.Data, &data)
		if data.Done {
			j := m.jobByChat(ev.ChatID)
			if j == nil && ev.ChatID == "" {
				j = m.openJob()
			}
			if j != nil {
				if ev.ChatID != "" {
					j.chatID = ev.ChatID
				}
				j.chime = true
				return m.finish(j)
			}
		}
	}
	return nil
}

// streamTarget is the on-screen message a streamed event writes to. Any event
// for a reply we follow counts as a sign the socket is delivering it.
func (m *App) streamTarget(ev socket.Event) (api.Message, bool) {
	if ev.MessageID == "" {
		return api.Message{}, false
	}
	if j := m.jobs[ev.MessageID]; j != nil {
		j.lastEvent = time.Now()
	}
	if ev.ChatID != "" && m.chatID != "" && ev.ChatID != m.chatID {
		return api.Message{}, false
	}
	msg, ok := m.history.Messages[ev.MessageID]
	return msg, ok
}

// appendContent applies the older text events (message, replace) and
// chat:completion, which carries either text or a snapshot of the output.
func (m *App) appendContent(ev socket.Event, delta bool) {
	msg, ok := m.streamTarget(ev)
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(ev.Data, &raw); err != nil {
		return
	}
	changed := false
	if out, ok := raw["output"]; ok && len(out) > 0 && out[0] == '[' {
		msg.Output = append(json.RawMessage(nil), out...)
		changed = true
	}
	if contentRaw, ok := raw["content"]; ok {
		var text string
		if err := json.Unmarshal(contentRaw, &text); err == nil {
			if delta {
				msg.Content += api.Content(text)
			} else {
				msg.Content = api.Content(text)
			}
			changed = true
		}
	}
	if !changed {
		return
	}
	m.history.Messages[ev.MessageID] = msg
	m.syncTranscript()
}

// applyResponseEvent applies a response:completion event, which servers from
// 0.11 send for each batch of streamed text and each finished output item.
func (m *App) applyResponseEvent(ev socket.Event) {
	msg, ok := m.streamTarget(ev)
	if !ok {
		return
	}
	var d struct {
		Type         string         `json:"type"`
		OutputIndex  *int           `json:"output_index"`
		ContentIndex int            `json:"content_index"`
		ItemID       string         `json:"item_id"`
		Delta        *string        `json:"delta"`
		Item         map[string]any `json:"item"`
	}
	if err := json.Unmarshal(ev.Data, &d); err != nil || d.OutputIndex == nil || *d.OutputIndex < 0 {
		return
	}
	var items []map[string]any
	_ = json.Unmarshal(msg.Output, &items)
	idx := *d.OutputIndex
	switch d.Type {
	case "response.output_item.added", "response.output_item.done":
		if d.Item == nil {
			return
		}
		items = growOutput(items, idx, "")
		items[idx] = d.Item
	case "response.output_text.delta", "response.reasoning_text.delta":
		if d.Delta == nil {
			return
		}
		itemType, partType := "message", "output_text"
		if d.Type == "response.reasoning_text.delta" {
			itemType, partType = "reasoning", "reasoning_text"
		}
		items = growOutput(items, idx, itemType)
		item := items[idx]
		if item["id"] == nil && d.ItemID != "" {
			item["id"] = d.ItemID
		}
		content, _ := item["content"].([]any)
		for len(content) <= d.ContentIndex {
			content = append(content, map[string]any{"type": partType, "text": ""})
		}
		part, ok := content[d.ContentIndex].(map[string]any)
		if !ok {
			part = map[string]any{"type": partType, "text": ""}
		}
		text, _ := part["text"].(string)
		part["text"] = text + *d.Delta
		content[d.ContentIndex] = part
		item["content"] = content
	default:
		return
	}
	out, err := json.Marshal(items)
	if err != nil {
		return
	}
	msg.Output = out
	m.history.Messages[ev.MessageID] = msg
	m.syncTranscript()
}

// growOutput pads the output list so index idx exists. A new last item gets
// itemType; any gap before it is filled with empty items.
func growOutput(items []map[string]any, idx int, itemType string) []map[string]any {
	for len(items) <= idx {
		item := map[string]any{}
		if len(items) == idx && itemType != "" {
			item["type"] = itemType
		}
		items = append(items, item)
	}
	if items[idx] == nil {
		items[idx] = map[string]any{"type": itemType}
	}
	return items
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
	if m.jobByChat(e.ID) != nil {
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
	// gone is set when the chat was deleted somewhere else.
	gone bool
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
