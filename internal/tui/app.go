package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	gstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"

	"github.com/usr1/owui/internal/api"
	"github.com/usr1/owui/internal/chattree"
	"github.com/usr1/owui/internal/config"
	"github.com/usr1/owui/internal/socket"
	"github.com/usr1/owui/internal/store"
)

const (
	phaseSplash = iota
	phaseLogin
	phaseMain
)

const (
	focusComposer = iota
	focusSidebar
	focusChat
)

const (
	overlayNone = iota
	overlayModel
	overlayTools
	overlayTerminal
	overlayFeatures
	overlayRename
	overlayHelp
	overlaySearch
)

const (
	authAPIKey = iota
	authPassword
)

const sidebarWidth = 32

// App is the Open WebUI TUI.
type App struct {
	cfg    config.File
	client *api.Client
	index  *store.Index
	sock   *socket.Client
	sockOn bool

	phase  int
	width  int
	height int
	styles styles

	user      api.User
	models    []api.Model
	tools     []api.Tool
	terminals []api.Terminal
	server    api.ServerFeatures

	modelID    string
	features   api.Features
	toolIDs    []string
	terminalID string

	chats       []store.Entry
	showAll     bool
	filter      string
	filtering   bool
	selected    int
	focus       int
	showSidebar bool

	chatID    string
	chatTitle string
	history   api.History

	stickToBottom bool
	termFocused   bool
	viewToken     int
	jobs          map[string]*genJob
	status        string
	errText       string

	input      textarea.Model
	vp         viewport.Model
	spin       spinner.Model
	md         *glamour.TermRenderer
	mdWidth    int
	mdCache    map[string]mdEntry
	rendered   string
	sel        selection
	copyBlocks []string
	lineBlock  []int

	overlay       int
	pick          picker
	featureCursor int
	rename        textinput.Model
	confirmDelete bool

	bootHeld    *bootMsg
	splashReady bool
	frame       int
	quitArmed   bool

	authMode   int
	loginField int
	urlIn      textinput.Model
	keyIn      textinput.Model
	emailIn    textinput.Model
	passIn     textinput.Model
	loginErr   string
}

type mdEntry struct {
	src string
	out string
}

type pickerItem struct {
	id    string
	label string
	note  string
}

type picker struct {
	items  []pickerItem
	filter string
	cursor int
	multi  map[string]bool
	fuzzy  bool
}

func Run(cfg config.File, index *store.Index) error {
	p := tea.NewProgram(newApp(cfg, index))
	_, err := p.Run()
	return err
}

func newApp(cfg config.File, index *store.Index) App {
	a := App{
		cfg:         cfg,
		index:       index,
		phase:       phaseLogin,
		styles:      newStyles(),
		showSidebar: true,
		focus:       focusComposer,
		features: api.Features{
			WebSearch:       cfg.WebSearch,
			CodeInterpreter: cfg.CodeInterpreter,
			ImageGeneration: cfg.ImageGeneration,
			Memory:          cfg.Memory,
		},
		toolIDs:       append([]string{}, cfg.ToolIDs...),
		terminalID:    cfg.TerminalID,
		modelID:       cfg.Model,
		showAll:       cfg.ShowAll,
		stickToBottom: true,
		termFocused:   true,
		selected:      0,
		input:         newComposer(),
		vp:            viewport.New(),
		rename:        newSingleLine("title"),
		urlIn:         newSingleLine("http://localhost:8080"),
		keyIn:         newSingleLine("sk-..."),
		emailIn:       newSingleLine("you@example.com"),
		passIn:        newSingleLine(""),
	}
	a.passIn.EchoMode = textinput.EchoPassword
	a.urlIn.SetValue(cfg.URL)
	if cfg.Credential() == "" {
		a.urlIn.Focus()
	} else {
		a.phase = phaseSplash
	}
	if cfg.APIKey != "" {
		a.keyIn.SetValue(cfg.APIKey)
		a.authMode = authAPIKey
	} else if cfg.Token != "" {
		a.authMode = authPassword
	}
	a.vp.MouseWheelEnabled = true
	a.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot))
	a.spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	return a
}

func newComposer() textarea.Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Placeholder = "message, or /help"
	ta.Prompt = "> "
	ta.CharLimit = 0
	ta.MaxHeight = 8
	ta.SetHeight(4)
	km := ta.KeyMap
	km.InsertNewline = key.NewBinding(key.WithKeys("alt+enter"))
	km.LineNext = key.NewBinding(key.WithKeys("down"))
	km.LinePrevious = key.NewBinding(key.WithKeys("up"))
	km.TransposeCharacterBackward = key.NewBinding(key.WithKeys("alt+t"))
	km.Paste = key.NewBinding(key.WithKeys("ctrl+v", "ctrl+shift+v", "shift+insert"))
	ta.KeyMap = km
	return ta
}

func newSingleLine(placeholder string) textinput.Model {
	in := textinput.New()
	in.Placeholder = placeholder
	in.CharLimit = 0
	km := in.KeyMap
	km.Paste = key.NewBinding(key.WithKeys("ctrl+v", "ctrl+shift+v", "shift+insert"))
	in.KeyMap = km
	return in
}

func (m App) Init() tea.Cmd {
	if m.phase == phaseSplash {
		return m.startSplash()
	}
	return m.urlIn.Focus()
}

func (m *App) startSplash() tea.Cmd {
	m.phase = phaseSplash
	m.splashReady = false
	m.frame = 0
	m.bootHeld = nil
	return tea.Batch(m.bootstrap(), spin(), holdSplash())
}

func spin() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg {
		return splashFrameMsg{}
	})
}

func holdSplash() tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg {
		return splashHoldMsg{}
	})
}

func quit() tea.Cmd {
	return func() tea.Msg { return tea.Quit() }
}

const quitHint = "press ctrl+c again to exit"

type quitResetMsg struct{}

func (m *App) requestQuit() tea.Cmd {
	if m.quitArmed {
		return quit()
	}
	m.quitArmed = true
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return quitResetMsg{}
	})
}

func waitEvent(ch <-chan socket.Event) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}

type bootMsg struct {
	user   api.User
	models []api.Model
	tools  []api.Tool
	terms  []api.Terminal
	feats  api.ServerFeatures
	err    error
}

type chatsMsg struct {
	entries []store.Entry
	err     error
}

type allChatsMsg struct {
	entries []store.Entry
	err     error
}

type sentMsg struct {
	id        string
	title     string
	created   bool
	assistant string
	ack       api.CompletionAck
	prompt    string
	err       error
}

type loadedMsg struct {
	rec   api.ChatRecord
	why   string
	err   error
	chime bool
}

type savedMsg struct{ err error }

type pollMsg struct {
	id        string
	assistant string
	seq       int
}

type stoppedMsg struct {
	id        string
	assistant string
	err       error
}

func (m *App) bootstrap() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		client := api.New(cfg.URL, cfg.Credential())
		user, err := client.Me(ctx)
		if err != nil {
			return bootMsg{err: err}
		}
		models, modelsErr := client.Models(ctx)
		tools, _ := client.Tools(ctx)
		terms, _ := client.Terminals(ctx)
		feats, _ := client.ServerFeatures(ctx)
		if modelsErr != nil && len(models) == 0 {
			return bootMsg{user: user, feats: feats, err: modelsErr}
		}
		return bootMsg{user: user, models: models, tools: tools, terms: terms, feats: feats}
	}
}

type splashFrameMsg struct{}
type splashHoldMsg struct{}

func (m *App) releaseSplash() tea.Cmd {
	if m.bootHeld == nil {
		return nil
	}
	if m.phase == phaseSplash && !m.splashReady {
		return nil
	}
	msg := *m.bootHeld
	m.bootHeld = nil
	return m.applyBoot(msg)
}

func (m *App) applyBoot(msg bootMsg) tea.Cmd {
	if msg.err != nil && msg.user.ID == "" {
		m.phase = phaseLogin
		m.loginErr = msg.err.Error()
		return m.urlIn.Focus()
	}
	m.phase = phaseMain
	m.user = msg.user
	m.models = msg.models
	m.tools = msg.tools
	m.terminals = msg.terms
	m.server = msg.feats
	m.cfg.UserID = msg.user.ID
	m.client = api.New(m.cfg.URL, m.cfg.Credential())
	m.pickModel(m.modelID)
	if msg.err != nil {
		m.errText = msg.err.Error()
	}
	if m.width > 0 {
		m.layout()
	}
	m.ensureSocket()
	var cmds []tea.Cmd
	cmds = append(cmds, m.input.Focus(), waitEvent(m.sock.Events), m.refreshChats(), m.persist())
	if m.showAll {
		cmds = append(cmds, m.fetchAllChats())
	}
	return tea.Batch(cmds...)
}

func (m *App) ensureSocket() {
	if m.sockOn || m.client == nil {
		return
	}
	m.sock = socket.New(m.client.BaseURL, m.client.Token)
	go m.sock.Run(context.Background())
	m.sockOn = true
}

func (m *App) storeKey() string {
	return store.Key(m.cfg.URL, m.user.ID)
}

func (m *App) refreshChats() tea.Cmd {
	if m.client == nil || m.index == nil || m.user.ID == "" {
		return nil
	}
	client := m.client
	index := m.index
	key := m.storeKey()
	return func() tea.Msg {
		entries := index.List(key)
		keep := make([]store.Entry, 0, len(entries))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, e := range entries {
			rec, err := client.GetChat(ctx, e.ID)
			if api.IsNotFound(err) {
				continue
			}
			if err != nil {
				keep = append(keep, e)
				continue
			}
			e.Title = rec.DisplayTitle()
			keep = append(keep, e)
		}
		if err := index.Replace(key, keep); err != nil {
			return chatsMsg{entries: keep, err: err}
		}
		return chatsMsg{entries: keep}
	}
}

func (m *App) fetchAllChats() tea.Cmd {
	if m.client == nil {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		list, err := client.ListChats(ctx)
		entries := make([]store.Entry, 0, len(list))
		for _, chat := range list {
			title := chat.Title
			if title == "" {
				title = "untitled"
			}
			entries = append(entries, store.Entry{
				ID:        chat.ID,
				Title:     title,
				UpdatedAt: int64(chat.UpdatedAt),
			})
		}
		return allChatsMsg{entries: entries, err: err}
	}
}

func (m *App) toggleChatScope() tea.Cmd {
	m.showAll = !m.showAll
	m.cfg.ShowAll = m.showAll
	m.filtering = false
	m.filter = ""
	m.selected = 0
	if m.showAll {
		m.status = "all chats"
		return tea.Batch(m.fetchAllChats(), m.persist())
	}
	if m.index != nil {
		m.chats = m.index.List(m.storeKey())
	} else {
		m.chats = nil
	}
	m.status = "this client"
	return m.persist()
}

func (m *App) showClientList() {
	if m.index == nil {
		return
	}
	m.chats = m.index.List(m.storeKey())
}

func (m *App) upsertVisible(e store.Entry) {
	if !m.showAll {
		m.showClientList()
		return
	}
	next := make([]store.Entry, 0, len(m.chats)+1)
	next = append(next, e)
	for _, old := range m.chats {
		if old.ID != e.ID {
			next = append(next, old)
		}
	}
	m.chats = next
}

func (m *App) retitleVisible(id, title string) {
	inIndex := false
	if m.index != nil {
		for _, e := range m.index.List(m.storeKey()) {
			if e.ID == id {
				inIndex = true
				break
			}
		}
		if inIndex {
			_ = m.index.Rename(m.storeKey(), id, title)
		}
	}
	if !m.showAll {
		m.showClientList()
		return
	}
	for i := range m.chats {
		if m.chats[i].ID == id {
			m.chats[i].Title = title
			return
		}
	}
}

func (m *App) dropVisible(id string) {
	next := make([]store.Entry, 0, len(m.chats))
	for _, e := range m.chats {
		if e.ID != id {
			next = append(next, e)
		}
	}
	m.chats = next
}

func (m *App) pickModel(id string) {
	if id != "" {
		for _, model := range m.models {
			if model.ID == id {
				m.modelID = id
				return
			}
		}
		m.modelID = id
		return
	}
	if len(m.models) > 0 {
		m.modelID = m.models[0].ID
		m.applyModelDefaults(m.models[0])
	}
}

func (m *App) applyModelDefaults(model api.Model) {
	if m.terminalID == "" && model.TerminalID != "" {
		m.terminalID = model.TerminalID
		m.features.CodeInterpreter = false
	}
	if len(m.toolIDs) == 0 && len(model.ToolIDs) > 0 {
		m.toolIDs = append([]string{}, model.ToolIDs...)
	}
}

func (m *App) modelByID(id string) api.Model {
	for _, model := range m.models {
		if model.ID == id {
			return model
		}
	}
	if id == "" {
		return api.Model{}
	}
	return api.Model{ID: id, Name: id}
}

func (m *App) modelName() string {
	model := m.modelByID(m.modelID)
	if model.Name != "" {
		return model.Name
	}
	return m.modelID
}

func (m App) featureEnabled(name string) (bool, string) {
	model := m.modelByID(m.modelID)
	switch name {
	case "web":
		if !m.server.WebOn() {
			return false, "web search is off on this server"
		}
		if !model.Allows("web_search") {
			return false, "this model has web search off"
		}
	case "code":
		if m.terminalID != "" {
			return false, "turn the terminal off before using code interpreter"
		}
		if !m.server.CodeOn() {
			return false, "code interpreter is off on this server"
		}
		if !model.Allows("code_interpreter") {
			return false, "this model has code interpreter off"
		}
	case "image":
		if !m.server.ImageOn() {
			return false, "image generation is off on this server"
		}
		if !model.Allows("image_generation") {
			return false, "this model has image generation off"
		}
	case "memory":
		if !model.Allows("memory") {
			return false, "this model has memory off"
		}
	}
	return true, ""
}

func (m App) effectiveFeatures() api.Features {
	f := m.features
	if ok, _ := m.featureEnabled("web"); !ok {
		f.WebSearch = false
	}
	if ok, _ := m.featureEnabled("code"); !ok {
		f.CodeInterpreter = false
	}
	if ok, _ := m.featureEnabled("image"); !ok {
		f.ImageGeneration = false
	}
	if ok, _ := m.featureEnabled("memory"); !ok {
		f.Memory = false
	}
	return f
}

func (m *App) persist() tea.Cmd {
	f := m.cfg
	f.Model = m.modelID
	f.WebSearch = m.features.WebSearch
	f.CodeInterpreter = m.features.CodeInterpreter
	f.ImageGeneration = m.features.ImageGeneration
	f.Memory = m.features.Memory
	f.ToolIDs = append([]string{}, m.toolIDs...)
	f.TerminalID = m.terminalID
	f.ShowAll = m.showAll
	if m.client != nil && m.client.BaseURL != "" {
		f.URL = m.client.BaseURL
	}
	return func() tea.Msg {
		err := f.Save()
		return savedMsg{err: err}
	}
}

func (m *App) layout() {
	side := 0
	if m.showSidebar && m.phase == phaseMain {
		side = sidebarWidth
	}
	mainW := m.width - side
	if mainW < 20 {
		mainW = 20
	}
	composerH := 4
	// header, status, feature bar, composer
	transcriptH := m.height - composerH - 3
	if transcriptH < 1 {
		transcriptH = 1
	}
	m.vp.SetWidth(mainW)
	m.vp.SetHeight(transcriptH)
	m.input.SetWidth(mainW - 1)
	m.input.SetHeight(composerH)
	w := mainW - 4
	if w < 20 {
		w = 20
	}
	m.urlIn.SetWidth(w)
	m.keyIn.SetWidth(w)
	m.emailIn.SetWidth(w)
	m.passIn.SetWidth(w)
	m.rename.SetWidth(w)
}

// lineWriter records which markdown block each transcript line came from.
// A trailing newline leaves the extra empty Split element with no block.
type lineWriter struct {
	b     strings.Builder
	ids   []int
	curID int
	open  bool
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.WriteString(string(p))
	return len(p), nil
}

func (w *lineWriter) WriteString(s string) {
	for _, r := range s {
		if !w.open {
			w.ids = append(w.ids, w.curID)
			w.open = true
		}
		w.b.WriteRune(r)
		if r == '\n' {
			w.open = false
		}
	}
}

func (w *lineWriter) writeMapped(text string, ids []int) {
	if text == "" {
		return
	}
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		id := -1
		if i < len(ids) {
			id = ids[i]
		}
		if i > 0 {
			w.curID = -1
			w.WriteString("\n")
		}
		w.curID = id
		w.WriteString(ln)
	}
}

func (w *lineWriter) finish() []int {
	ids := w.ids
	if strings.HasSuffix(w.b.String(), "\n") {
		ids = append(ids, -1)
	}
	return ids
}

func (m *App) syncTranscript() {
	m.rendered = m.transcript()
	m.vp.SetContent(m.rendered)
	if m.stickToBottom {
		m.vp.GotoBottom()
	}
}

// scrollTranscript moves the chat. Scrolling up stops pinning the view to the
// newest line, so a later token does not yank it back down.
func (m App) sidebarWindow(n int) (start, end, limit int) {
	limit = m.height - 4
	if limit < 1 {
		limit = 1
	}
	if m.selected >= limit {
		start = m.selected - limit + 1
	}
	if start > n {
		start = n
	}
	end = start + limit
	if end > n {
		end = n
	}
	return start, end, limit
}

func (m *App) scrollSidebar(msg tea.Msg) {
	if len(m.visibleChats()) == 0 {
		return
	}
	page := m.height - 4
	if page < 1 {
		page = 1
	}
	delta := 0
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			delta = -3
		case tea.MouseWheelDown:
			delta = 3
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "pgup", "ctrl+up":
			delta = -page
		case "pgdown", "ctrl+down":
			delta = page
		}
		if msg.Code == tea.KeyPgUp {
			delta = -page
		}
		if msg.Code == tea.KeyPgDown {
			delta = page
		}
	}
	if delta == 0 {
		return
	}
	m.selected += delta
	m.clampSelected()
}

func (m *App) scrollTranscript(msg tea.Msg) {
	m.vp, _ = m.vp.Update(msg)
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			m.stickToBottom = false
			return
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "pgup", "up", "ctrl+up", "ctrl+u":
			m.stickToBottom = false
			return
		}
		if msg.Code == tea.KeyPgUp || msg.Code == tea.KeyUp {
			m.stickToBottom = false
			return
		}
	}
	m.stickToBottom = m.vp.AtBottom()
}

func (m *App) transcript() string {
	m.copyBlocks = m.copyBlocks[:0]
	chain := chattree.Chain(m.history)
	if len(chain) == 0 {
		m.lineBlock = nil
		return m.styles.dim.Render("No chat open. Write a message to start one. It is stored on the server, so the web UI can open it too.")
	}
	var w lineWriter
	for _, msg := range chain {
		name := "assistant"
		style := m.styles.assist
		if msg.Role == "user" {
			name = "you"
			style = m.styles.user
		} else if label := m.assistantLabel(msg); label != "" {
			name = label
		}
		w.curID = -1
		fmt.Fprintf(&w, "%s\n", style.Render(name))
		parts := messageParts(msg.Content.String(), msg.Output)
		wrote := false
		for _, part := range parts {
			if part.tool != "" {
				if wrote {
					w.curID = -1
					w.WriteString("\n")
				}
				label := part.tool
				if part.running {
					label = "calling " + part.tool
				}
				w.curID = -1
				w.WriteString(m.styles.accent.Render(label))
				wrote = true
				continue
			}
			body, bodyIDs := m.renderMapped(msg.ID, part.text)
			if body == "" {
				continue
			}
			if wrote {
				w.curID = -1
				w.WriteString("\n")
			}
			w.writeMapped(body, bodyIDs)
			wrote = true
		}
		working := false
		if j := m.openJob(); j != nil && j.assistant == msg.ID {
			working = true
		}
		if working {
			if wrote {
				w.curID = -1
				w.WriteString("\n")
			}
			w.curID = -1
			w.WriteString(m.spin.View())
		}
		w.curID = -1
		w.WriteString("\n")
		if src := sourceBlock(msg.Sources); src != "" {
			w.WriteString(m.styles.dim.Render(src))
			w.WriteString("\n")
		}
		w.WriteString("\n")
	}
	m.lineBlock = w.finish()
	return w.b.String()
}

func (m App) assistantLabel(msg api.Message) string {
	if msg.Model != "" {
		if known := m.modelByID(msg.Model); known.Name != "" {
			return known.Name
		}
		return msg.Model
	}
	if msg.ModelName != "" {
		return msg.ModelName
	}
	return ""
}

func (m *App) renderMarkdown(id, body string) string {
	if body == "" || body == "…" {
		return body
	}
	width := m.vp.Width() - 4
	if width < 24 {
		width = 24
	}
	if m.md == nil || m.mdWidth != width {
		r, err := glamour.NewTermRenderer(
			glamour.WithStyles(markdownStyle()),
			glamour.WithWordWrap(width),
			glamour.WithTableWrap(true),
		)
		if err != nil {
			return body
		}
		m.md = r
		m.mdWidth = width
		m.mdCache = nil
	}
	if m.mdCache == nil {
		m.mdCache = map[string]mdEntry{}
	}
	if e, ok := m.mdCache[id]; ok && e.src == body {
		return e.out
	}
	out, err := m.md.Render(body)
	if err != nil {
		return body
	}
	out = strings.TrimRight(out, "\n")
	m.mdCache[id] = mdEntry{src: body, out: out}
	return out
}

func markdownStyle() ansi.StyleConfig {
	style := gstyles.DarkStyleConfig
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""
	return style
}

func sourceBlock(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("sources\n")
	n := 0
	for _, it := range items {
		label := sourceLabel(it)
		if label == "" {
			continue
		}
		n++
		fmt.Fprintf(&b, "  %d. %s\n", n, label)
	}
	if n == 0 {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

func sourceLabel(it map[string]any) string {
	if src, ok := it["source"].(map[string]any); ok {
		if name, ok := src["name"].(string); ok && name != "" {
			return name
		}
		if url, ok := src["url"].(string); ok && url != "" {
			return url
		}
	}
	for _, key := range []string{"name", "title", "url"} {
		if s, ok := it[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func (m *App) visibleChats() []store.Entry {
	if m.filter == "" {
		return m.chats
	}
	q := strings.ToLower(m.filter)
	var out []store.Entry
	for _, e := range m.chats {
		if strings.Contains(strings.ToLower(e.Title), q) || strings.Contains(strings.ToLower(e.ID), q) {
			out = append(out, e)
		}
	}
	return out
}

func (m *App) clampSelected() {
	n := len(m.visibleChats())
	if n == 0 {
		m.selected = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= n {
		m.selected = n - 1
	}
}

func (p picker) visible() []pickerItem {
	if p.filter == "" {
		return p.items
	}
	if p.fuzzy {
		return fuzzyItems(p.items, p.filter)
	}
	q := strings.ToLower(p.filter)
	var out []pickerItem
	for _, it := range p.items {
		if strings.Contains(strings.ToLower(it.label), q) || strings.Contains(strings.ToLower(it.id), q) || strings.Contains(strings.ToLower(it.note), q) {
			out = append(out, it)
		}
	}
	return out
}

func (m *App) movePicker(delta int) {
	items := m.pick.visible()
	if len(items) == 0 {
		m.pick.cursor = 0
		return
	}
	m.pick.cursor += delta
	if m.pick.cursor < 0 {
		m.pick.cursor = 0
	}
	if m.pick.cursor >= len(items) {
		m.pick.cursor = len(items) - 1
	}
}

func sessionID(sock *socket.Client) string {
	if sock != nil {
		if id := sock.SessionID(); id != "" {
			return id
		}
	}
	return "api-" + uuid.NewString()
}

func variables() map[string]string {
	return map[string]string{
		"{{USER_LANGUAGE}}":    "en-US",
		"{{CURRENT_DATETIME}}": time.Now().Format(time.RFC3339),
		"{{CURRENT_TIMEZONE}}": time.Now().Location().String(),
	}
}
