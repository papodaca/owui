package tui

import (
	"strings"
	"testing"

	"github.com/usr1/owui/internal/api"
	"github.com/usr1/owui/internal/config"
)

func TestHerdrNotifyNamesTheChat(t *testing.T) {
	args := herdrNotifyArgs("tables", "Here is the table you asked for.")
	if len(args) < 3 || args[2] != "tables" {
		t.Fatalf("args = %#v", args)
	}
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "--body\x00Here is the table you asked for.") {
		t.Fatalf("args = %#v", args)
	}
	placeholder := herdrNotifyArgs("New Chat", "")
	if len(placeholder) < 3 || placeholder[2] != "Reply ready" {
		t.Fatalf("args = %#v", placeholder)
	}
	bare := herdrNotifyArgs("tables", "  ")
	for _, a := range bare {
		if a == "--body" {
			t.Fatal("empty reply became a body")
		}
	}
}

func TestDoneReplyChimes(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_PANE_ID", "")
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.generating = true
	cmd := app.applyLoaded(loadedMsg{
		why:   "done",
		chime: true,
		rec:   api.ChatRecord{ID: "c", Title: "tables"},
	})
	if cmd == nil {
		t.Fatal("finished reply was silent")
	}
	if app.generating {
		t.Fatal("still generating")
	}
	quiet := app.applyLoaded(loadedMsg{why: "done", rec: api.ChatRecord{ID: "c"}})
	if quiet != nil {
		t.Fatal("a stopped reply chimed")
	}
}

func TestInHerdr(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_PANE_ID", "")
	if inHerdr() {
		t.Fatal("not in herdr")
	}
	t.Setenv("HERDR_ENV", "1")
	if !inHerdr() {
		t.Fatal("HERDR_ENV")
	}
}
