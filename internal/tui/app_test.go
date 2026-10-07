package tui

import (
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
	app.vp.SetWidth(20)
	app.vp.SetHeight(4)
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
	app.vp.SetHeight(4)
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

func TestSpinnerShowsWhileGenerating(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.generating = true
	app.genAssistant = "a"
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
