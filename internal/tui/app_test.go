package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/usr1/owui/internal/api"
	"github.com/usr1/owui/internal/config"
	"github.com/usr1/owui/internal/store"
)

func TestLoginRenders(t *testing.T) {
	app := newApp(config.File{URL: "http://127.0.0.1:8080", Path: t.TempDir() + "/config.toml"}, nil)
	next, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := next.(App).View()
	if !strings.Contains(view.Content, "Open WebUI") {
		t.Fatalf("view = %q", view.Content)
	}
	if !view.AltScreen {
		t.Fatal("expected the alternate screen")
	}
}

func TestComposerGrowsWithText(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.focus = focusComposer
	app.input.Focus()
	next, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	app = next.(App)
	if h := app.input.Height(); h != 1 {
		t.Fatalf("empty composer height = %d", h)
	}
	if h := app.vp.Height(); h != 27 {
		t.Fatalf("transcript height = %d", h)
	}
	for _, k := range []tea.KeyPressMsg{
		{Code: 'a', Text: "a"},
		{Code: tea.KeyEnter, Mod: tea.ModAlt},
		{Code: 'b', Text: "b"},
		{Code: tea.KeyEnter, Mod: tea.ModAlt},
		{Code: 'c', Text: "c"},
	} {
		next, _ = app.Update(k)
		app = next.(App)
	}
	if h := app.input.Height(); h != 3 {
		t.Fatalf("three line composer height = %d", h)
	}
	if h := app.vp.Height(); h != 25 {
		t.Fatalf("transcript height = %d", h)
	}
}

func TestSplashInsteadOfLogin(t *testing.T) {
	app := newApp(config.File{
		URL:    "http://127.0.0.1:8080",
		APIKey: "sk-test",
		Path:   t.TempDir() + "/config.toml",
	}, nil)
	if app.phase != phaseSplash {
		t.Fatalf("phase = %d", app.phase)
	}
	next, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	body := next.(App).View().Content
	if !strings.Contains(body, "connecting") {
		t.Fatal("splash missing connecting")
	}
	if strings.Contains(body, "api key") {
		t.Fatal("login form shown during connect")
	}
}

func TestMarkdownRendersTable(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.vp.SetWidth(72)
	src := "#### Closest races\n\n| State | Seat |\n| --- | --- |\n| **Maine** | toss-up |\n\nSee the [Cook Report](https://www.cookpolitical.com/).\n"
	out := app.renderMarkdown("m1", src)
	if strings.Contains(out, "| --- |") || strings.Contains(out, "####") || strings.Contains(out, "**Maine**") {
		t.Fatalf("markdown left raw:\n%s", out)
	}
	if !strings.Contains(out, "Maine") || !strings.Contains(out, "toss-up") || !strings.Contains(out, "Cook Report") {
		t.Fatalf("missing text:\n%s", out)
	}
}

func TestQuitTakesTwoCtrlC(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	key := tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	if key.String() != "ctrl+c" {
		t.Fatalf("key = %q", key.String())
	}
	next, cmd := app.Update(key)
	got := next.(App)
	if !got.quitArmed {
		t.Fatal("first ctrl+c should arm quit")
	}
	if cmd == nil {
		t.Fatal("expected a reset timer")
	}
	next, cmd = got.Update(key)
	if cmd == nil {
		t.Fatal("expected quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("second ctrl+c should quit")
	}
	got = next.(App)
	got.quitArmed = true
	next, _ = got.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if next.(App).quitArmed {
		t.Fatal("another key should cancel the quit")
	}
}

func TestPageUpScrollsTranscript(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.vp.SetWidth(20)
	app.vp.SetHeight(4)
	app.vp.SetContent(strings.Repeat("line\n", 40))
	app.vp.GotoBottom()
	before := app.vp.YOffset()
	if before == 0 {
		t.Fatal("fixture is not taller than the window")
	}
	next, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	scrolled := next.(App)
	if scrolled.vp.YOffset() >= before {
		t.Fatalf("offset stayed at %d", before)
	}
}

func TestWheelScrollsWhilePromptFocused(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.focus = focusComposer
	app.vp.SetWidth(20)
	app.vp.SetHeight(4)
	app.vp.SetContent(strings.Repeat("line\n", 40))
	app.vp.GotoBottom()
	before := app.vp.YOffset()
	next, _ := app.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	got := next.(App)
	scrolled := got.vp.YOffset()
	if scrolled >= before {
		t.Fatalf("wheel did not scroll, offset %d", before)
	}
	if got.stickToBottom {
		t.Fatal("view stayed pinned after scrolling up")
	}
}

func TestPageUpScrollsSidebarNotTranscript(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.focus = focusComposer
	app.showSidebar = true
	app.height = 12
	app.chats = make([]store.Entry, 40)
	for i := range app.chats {
		app.chats[i] = store.Entry{ID: strings.Repeat("x", i+1), Title: "chat"}
	}
	app.selected = 30
	app.layout()
	app.vp.SetContent(strings.Repeat("line\n", 40))
	app.vp.GotoBottom()
	chatAt := app.vp.YOffset()

	next, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	focused := next.(App)
	if focused.focus != focusSidebar {
		t.Fatalf("tab focus = %d", focused.focus)
	}
	next, _ = focused.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	got := next.(App)
	if got.selected >= 30 {
		t.Fatalf("sidebar selection stayed at %d", got.selected)
	}
	if got.vp.YOffset() != chatAt {
		t.Fatalf("page up moved the transcript from %d to %d", chatAt, got.vp.YOffset())
	}
}

func TestWheelScrollsSidebarWhenFocused(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.focus = focusSidebar
	app.height = 12
	app.chats = make([]store.Entry, 40)
	for i := range app.chats {
		app.chats[i] = store.Entry{ID: strings.Repeat("x", i+1), Title: "chat"}
	}
	app.selected = 30
	app.layout()
	app.vp.SetContent(strings.Repeat("line\n", 40))
	app.vp.GotoBottom()
	chatAt := app.vp.YOffset()
	next, _ := app.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	got := next.(App)
	if got.selected >= 30 {
		t.Fatalf("sidebar selection stayed at %d", got.selected)
	}
	if got.vp.YOffset() != chatAt {
		t.Fatal("wheel moved the transcript while the sidebar was focused")
	}
}

func TestTranscriptListsOutputToolCalls(t *testing.T) {
	output := `[
		{"type":"message","content":[{"type":"output_text","text":"I'll look it up."}]},
		{"type":"function_call","name":"tool_web_search_post","status":"completed","call_id":"a"},
		{"type":"function_call","name":"tool_web_search_post","status":"completed","call_id":"b"},
		{"type":"function_call_output","call_id":"a","output":[{"type":"input_text","text":"secret result"}]},
		{"type":"function_call","name":"tool_web_fetch_post","status":"completed","call_id":"c"},
		{"type":"message","content":[{"type":"output_text","text":"Rick and Morty was not canceled."}]}
	]`
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.history = api.History{
		CurrentID: "a",
		Messages: map[string]api.Message{
			"u": {ID: "u", Role: "user", Content: "hi", ChildrenIDs: api.IDList{"a"}},
			"a": {
				ID: "a", Role: "assistant", ParentID: "u", ChildrenIDs: api.IDList{},
				Content: "I'll look it up.\n\nRick and Morty was not canceled.",
				Output:  json.RawMessage(output),
			},
		},
	}
	plain := ansi.Strip(app.transcript())
	search := strings.Count(plain, "tool_web_search_post")
	if search != 2 || !strings.Contains(plain, "tool_web_fetch_post") {
		t.Fatalf("calls:\n%s", plain)
	}
	if strings.Contains(plain, "secret result") {
		t.Fatalf("result leaked:\n%s", plain)
	}
	look := strings.Index(plain, "I'll look it up")
	call := strings.Index(plain, "tool_web_search_post")
	answer := strings.Index(plain, "Rick and Morty")
	if look < 0 || call < look || answer < call {
		t.Fatalf("order:\n%s", plain)
	}
}

func TestTranscriptListsToolCalls(t *testing.T) {
	content := "Before the search.\n\n" +
		"<details type=\"tool_calls\" done=\"true\" id=\"c1\" name=\"tool_web_search_post\" arguments=\"{&#34;q&#34;:&#34;x&#34;}\">\n" +
		"<summary>Tool Executed</summary>\n{\"hits\":12}\n</details>\n\n" +
		"After the search.\n\n" +
		"<details type=\"tool_calls\" done=\"false\" name=\"tool_fetch\" arguments=\"{}\">\npartial"
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.history = api.History{
		CurrentID: "a",
		Messages: map[string]api.Message{
			"u": {ID: "u", Role: "user", Content: "hi", ChildrenIDs: api.IDList{"a"}},
			"a": {ID: "a", Role: "assistant", Content: api.Content(content), ParentID: "u", ChildrenIDs: api.IDList{}},
		},
	}
	out := app.transcript()
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "tool_web_search_post") || !strings.Contains(plain, "calling tool_fetch") {
		t.Fatalf("tools missing:\n%s", plain)
	}
	if strings.Contains(plain, "hits") || strings.Contains(plain, "<details") || strings.Contains(plain, "Tool Executed") || strings.Contains(plain, "partial") {
		t.Fatalf("tool result leaked:\n%s", plain)
	}
	before := strings.Index(plain, "Before the search")
	call := strings.Index(plain, "tool_web_search_post")
	after := strings.Index(plain, "After the search")
	if before < 0 || call < before || after < call {
		t.Fatalf("order:\n%s", plain)
	}
}

func TestAssistantLabelUsesModelName(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.models = []api.Model{{ID: "cur.grok-4.7", Name: "Grok 4.7"}}
	app.history = api.History{
		CurrentID: "a",
		Messages: map[string]api.Message{
			"u": {ID: "u", Role: "user", Content: "hi", ChildrenIDs: api.IDList{"a"}},
			"a": {ID: "a", Role: "assistant", Content: "ok", ParentID: "u", Model: "cur.grok-4.7", ChildrenIDs: api.IDList{}},
		},
	}
	out := app.transcript()
	if strings.Contains(out, "assistant") {
		t.Fatalf("fell back to assistant:\n%s", out)
	}
	if !strings.Contains(out, "Grok 4.7") {
		t.Fatalf("transcript = %q", out)
	}
}

func TestClickTranscriptFocusesPrompt(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.showSidebar = true
	app.width = 100
	app.height = 24
	app.layout()
	app.focus = focusSidebar
	app.input.Blur()
	next, _ := app.Update(tea.MouseClickMsg{X: sidebarWidth + 4, Y: 3, Button: tea.MouseLeft})
	app = next.(App)
	if !app.sel.active {
		t.Fatalf("click did not start a selection, focus %d", app.focus)
	}
	next, cmd := app.Update(tea.MouseReleaseMsg{X: sidebarWidth + 4, Y: 3, Button: tea.MouseLeft})
	got := next.(App)
	if got.focus != focusComposer {
		t.Fatalf("focus = %d", got.focus)
	}
	if cmd == nil {
		t.Fatal("prompt was not focused")
	}
}

func TestSwitchChatWhileGenerating(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.client = api.New("http://127.0.0.1:9", "k")
	app.jobs = map[string]*genJob{"aa": {chatID: "a", assistant: "aa"}}
	app.chatID = "a"
	app.chats = []store.Entry{{ID: "a", Title: "one"}, {ID: "b", Title: "two"}}
	cmd := app.openChatID("b")
	if cmd == nil {
		t.Fatal("click was refused")
	}
	if app.errText != "" {
		t.Fatal(app.errText)
	}
	if app.jobByChat("a") == nil {
		t.Fatal("the reply was cancelled")
	}
}

func TestFinishedReplyDoesNotStealTheOpenChat(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.termFocused = true
	app.chatID = "b"
	app.chatTitle = "other"
	app.jobs = map[string]*genJob{
		"aa": {chatID: "a", assistant: "aa"},
		"bb": {chatID: "b", assistant: "bb"},
	}
	app.history = api.History{CurrentID: "keep", Messages: map[string]api.Message{
		"keep": {ID: "keep", Role: "user", Content: "stay"},
	}}
	_ = app.applyLoaded(loadedMsg{
		why: "done",
		rec: api.ChatRecord{ID: "a", Title: "one"},
	})
	if app.chatID != "b" || app.history.CurrentID != "keep" {
		t.Fatalf("open chat changed to %s %s", app.chatID, app.history.CurrentID)
	}
	if app.jobByChat("a") != nil {
		t.Fatal("finished chat still marked working")
	}
	if app.jobByChat("b") == nil {
		t.Fatal("the other reply was cancelled")
	}
}

func TestSidebarMarksWorkingChat(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.showSidebar = true
	app.height = 20
	app.jobs = map[string]*genJob{"aa": {chatID: "a", assistant: "aa"}}
	app.chatID = "b"
	app.chats = []store.Entry{{ID: "a", Title: "one"}, {ID: "b", Title: "two"}}
	plain := ansi.Strip(app.sidebarView())
	one := strings.Index(plain, "one")
	two := strings.Index(plain, "two")
	mark := strings.Index(plain, "⠋")
	if mark < 0 || one < mark || two < one {
		t.Fatalf("sidebar =\n%s", plain)
	}
	if strings.Count(plain, "⠋") != 1 {
		t.Fatalf("sidebar =\n%s", plain)
	}
}

func TestSendWhileAnotherChatWorks(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.client = api.New("http://127.0.0.1:9", "k")
	app.modelID = "m"
	app.chatID = "b"
	app.history = api.History{
		CurrentID: "u",
		Messages: map[string]api.Message{
			"u": {ID: "u", Role: "user", Content: "hi", ChildrenIDs: api.IDList{}},
		},
	}
	app.jobs = map[string]*genJob{"aa": {chatID: "a", assistant: "aa", seq: 1}}
	app.input.SetValue("next question")
	cmd := app.send()
	if app.errText != "" {
		t.Fatal(app.errText)
	}
	if cmd == nil {
		t.Fatal("did not send")
	}
	if app.jobByChat("a") == nil {
		t.Fatal("the first reply was dropped")
	}
	if app.openJob() == nil || app.openJob().chatID != "b" {
		t.Fatal("the second chat did not start")
	}
}

func TestCodeBlockIsNotIndented(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.vp.SetWidth(80)
	src := "```\nmkdir -p ~/.config/btop/themes\ncp /path/to/gruvbox_dark.theme ~/.config/btop/themes/\n```"
	out := ansi.Strip(app.renderMarkdown("id", src))
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			t.Fatalf("indented:\n%q", out)
		}
	}
}

func TestGenerateTitleOnlyOnFirstTurn(t *testing.T) {
	first := []api.CompletionMessage{{Role: "user", Content: "hi"}}
	if !generateTitle(first) {
		t.Fatal("first turn")
	}
	withSystem := []api.CompletionMessage{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hi"},
	}
	if !generateTitle(withSystem) {
		t.Fatal("system plus user")
	}
	followUp := []api.CompletionMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "more"},
	}
	if generateTitle(followUp) {
		t.Fatal("follow-up should keep the existing title")
	}
}

func TestClickSidebarTitleOpensChat(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.showSidebar = true
	app.height = 20
	app.client = api.New("http://127.0.0.1:9", "k")
	app.chatID = "a"
	app.chats = []store.Entry{{ID: "a", Title: "one"}, {ID: "b", Title: "two"}}
	next, cmd := app.Update(tea.MouseClickMsg{X: 2, Y: 3, Button: tea.MouseLeft})
	got := next.(App)
	if got.focus != focusSidebar || got.selected != 1 {
		t.Fatalf("focus %d selected %d", got.focus, got.selected)
	}
	if cmd == nil {
		t.Fatal("click did not open the chat")
	}
}

func TestSpinnerShowsWhileGenerating(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.chatID = "c"
	app.jobs = map[string]*genJob{"a": {chatID: "c", assistant: "a"}}
	app.history = api.History{
		CurrentID: "a",
		Messages: map[string]api.Message{
			"u": {ID: "u", Role: "user", Content: "hi", ChildrenIDs: api.IDList{"a"}},
			"a": {ID: "a", Role: "assistant", ParentID: "u", ChildrenIDs: api.IDList{}},
		},
	}
	if !strings.Contains(app.transcript(), "⠋") {
		t.Fatalf("transcript = %q", app.transcript())
	}
}

func TestSpaceTogglesTool(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.tools = []api.Tool{{ID: "alpha", Name: "Alpha"}}
	app.openTools()
	next, _ := app.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	got := next.(App)
	if got.pick.filter != "" {
		t.Fatalf("space started a filter %q", got.pick.filter)
	}
	if !got.pick.multi["alpha"] {
		t.Fatal("space did not enable the tool")
	}
}

func TestClipUsesDisplayWidth(t *testing.T) {
	got := clip(strings.Repeat("🍦", 20)+"Paying Amended Tax", 12)
	if w := ansi.StringWidth(got); w > 12 {
		t.Fatalf("width %d, title %q", w, got)
	}
	plain := clip(strings.Repeat("a", 40), 10)
	if w := ansi.StringWidth(plain); w > 10 {
		t.Fatalf("width %d, title %q", w, plain)
	}
}

func TestPasteAPIKey(t *testing.T) {
	app := newApp(config.File{URL: "http://127.0.0.1:8080", Path: t.TempDir() + "/config.toml"}, nil)
	app.authMode = authAPIKey
	app.loginField = 1
	app.keyIn.Focus()
	next, _ := app.Update(tea.PasteMsg{Content: " sk-test-key\n"})
	if got := next.(App).keyIn.Value(); got != "sk-test-key" {
		t.Fatalf("key = %q", got)
	}
}

func TestCodeInterpreterClearsTerminal(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.terminalID = "open-terminal"
	app.modelID = "m"
	handled, _ := app.slash("/code on")
	if !handled {
		t.Fatal("expected /code to be handled")
	}
	if app.terminalID != "" {
		t.Fatal("terminal still set")
	}
	if !app.features.CodeInterpreter {
		t.Fatal("code interpreter not on")
	}
}
