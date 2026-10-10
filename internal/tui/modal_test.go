package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/usr1/owui/internal/config"
)

func TestModalDrawsOverChat(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.showSidebar = false
	next, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	app = next.(App)
	app.vp.SetContent(strings.Repeat("chat line\n", 40))
	items := make([]pickerItem, 30)
	for i := range items {
		items[i] = pickerItem{id: fmt.Sprint(i), label: fmt.Sprintf("tool %d", i)}
	}
	app.pick = picker{items: items, cursor: 20}
	app.overlay = overlayTools

	view := ansi.Strip(app.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) != 30 {
		t.Fatalf("view has %d lines", len(lines))
	}
	if !strings.Contains(lines[1], "chat line") {
		t.Fatalf("chat is not drawn behind the modal: %q", lines[1])
	}
	if !strings.Contains(view, "╭") || !strings.Contains(view, "tool 20") {
		t.Fatalf("modal is missing the cursor row:\n%s", view)
	}
	if strings.Contains(view, "tool 0 ") {
		t.Fatalf("modal shows rows outside its window:\n%s", view)
	}
}
