package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/usr1/owui/internal/api"
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

func TestSplitMarkdownKeepsTableAndFence(t *testing.T) {
	src := "Intro.\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n```\nline\n\nstill\n```\n"
	got := splitMarkdown(src)
	if len(got) != 3 {
		t.Fatalf("blocks = %#v", got)
	}
	if !strings.Contains(got[1], "| 1 | 2 |") {
		t.Fatalf("table = %q", got[1])
	}
	if !strings.Contains(got[2], "still") {
		t.Fatalf("fence = %q", got[2])
	}
}

func TestAltDragCopiesTableMarkdown(t *testing.T) {
	const table = "| Name | Qty |\n| --- | --- |\n| apple | 2 |\n| pear | 9 |"
	src := "Intro line about the data.\n\n" + table + "\n\nAfter the table."
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.showSidebar = true
	app.width = 100
	app.height = 40
	app.layout()
	app.history = api.History{
		CurrentID: "u",
		Messages: map[string]api.Message{
			"u": {ID: "u", Role: "user", Content: api.Content(src), ChildrenIDs: api.IDList{}},
		},
	}
	app.syncTranscript()
	lines := strings.Split(app.rendered, "\n")
	if len(app.lineBlock) != len(lines) {
		t.Fatalf("line map %d, rendered lines %d", len(app.lineBlock), len(lines))
	}
	start, end := -1, -1
	for i, id := range app.lineBlock {
		if id < 0 || !strings.Contains(app.copyBlocks[id], "| apple |") {
			continue
		}
		if start < 0 {
			start = i
		}
		end = i
	}
	if start < 0 {
		t.Fatal("table lines were not mapped to markdown")
	}
	got := sourceText(app.rendered, app.copyBlocks, app.lineBlock, cell{start, 0}, cell{end, 8})
	if got != table {
		t.Fatalf("markdown = %q", got)
	}
	if strings.Contains(got, "Intro") || strings.Contains(got, "│") {
		t.Fatalf("copied the rendered chat: %q", got)
	}

	y0 := 1 + start
	y1 := 1 + end
	x0 := sidebarWidth
	next, _ := app.Update(tea.MouseClickMsg{X: x0, Y: y0, Button: tea.MouseLeft})
	app = next.(App)
	next, _ = app.Update(tea.MouseMotionMsg{X: x0 + 10, Y: y1, Button: tea.MouseLeft})
	app = next.(App)
	next, cmd := app.Update(tea.MouseReleaseMsg{X: x0 + 10, Y: y1, Button: tea.MouseLeft, Mod: tea.ModAlt})
	gotApp := next.(App)
	if cmd == nil {
		t.Fatal("alt drag did not copy")
	}
	if gotApp.status != "copied markdown" {
		t.Fatalf("status = %q", gotApp.status)
	}
}
