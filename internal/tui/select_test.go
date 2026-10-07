package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/usr1/owui/internal/config"
)

func TestSelectedTextSkipsSidebarColumns(t *testing.T) {
	content := "alpha beta\nsecond line"
	got := selectedText(content, cell{line: 0, col: 0}, cell{line: 0, col: 5})
	if got != "alpha" {
		t.Fatalf("got %q", got)
	}
	got = selectedText(content, cell{line: 0, col: 6}, cell{line: 1, col: 6})
	if got != "beta\nsecond" {
		t.Fatalf("got %q", got)
	}
}

func TestDragInTranscriptCopiesChatText(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.showSidebar = true
	app.width = 100
	app.height = 24
	app.layout()
	app.rendered = "hello sidebar\nmore"
	app.vp.SetContent(app.rendered)
	app.vp.SetYOffset(0)

	x0 := sidebarWidth
	y0 := 1
	next, _ := app.Update(tea.MouseClickMsg{X: x0, Y: y0, Button: tea.MouseLeft})
	app = next.(App)
	next, _ = app.Update(tea.MouseMotionMsg{X: x0 + 5, Y: y0, Button: tea.MouseLeft})
	app = next.(App)
	_, cmd := app.Update(tea.MouseReleaseMsg{X: x0 + 5, Y: y0, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("drag did not copy")
	}
	if strings.TrimSpace(selectedText(app.rendered, cell{0, 0}, cell{0, 5})) != "hello" {
		t.Fatal("selection was not the chat text")
	}
}
