package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/papodaca/owui/internal/api"
	"github.com/papodaca/owui/internal/config"
	"github.com/papodaca/owui/internal/socket"
	"github.com/papodaca/owui/internal/store"
)

func TestChatListEventFetchesAndCoalesces(t *testing.T) {
	ev, ok := socket.ParseSocketEvent(`42["events",{"chat_id":"c9","message_id":null,"data":{"type":"chat:list","data":{"chat_id":"c9","folder_id":null}}}]`)
	if !ok || ev.Type != "chat:list" || ev.ChatID != "c9" {
		t.Fatalf("parsed %+v", ev)
	}

	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.client = &api.Client{BaseURL: "http://127.0.0.1:1"}
	app.showAll = false
	if cmd := app.onEvent(ev); cmd != nil || app.listFetching {
		t.Fatal("this-client view should not refetch the server list on events")
	}

	app.showAll = true
	if cmd := app.onEvent(ev); cmd == nil || !app.listFetching {
		t.Fatal("chat:list did not start a fetch")
	}
	if cmd := app.onEvent(ev); cmd != nil || !app.listDirty {
		t.Fatal("an event during a fetch should mark the list dirty, not fetch again")
	}
	next, cmd := app.Update(allChatsMsg{})
	app = next.(App)
	if cmd == nil || !app.listFetching || app.listDirty {
		t.Fatal("the dirty list was not refetched once the first fetch landed")
	}
	next, cmd = app.Update(allChatsMsg{})
	app = next.(App)
	if cmd != nil || app.listFetching {
		t.Fatal("a clean list fetched again")
	}
}

func TestReadMarkerDoesNotRefetch(t *testing.T) {
	ev, ok := socket.ParseSocketEvent(`42["events",{"chat_id":"c9","data":{"type":"chat:list","data":{"chat_id":"c9","last_read_at":1791612933}}}]`)
	if !ok {
		t.Fatal("did not parse")
	}
	if chatListChanged(ev) {
		t.Fatal("a last_read_at chat:list is not a list change")
	}
}

func TestFocusRefetchesChatList(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.client = &api.Client{BaseURL: "http://127.0.0.1:1"}
	app.showAll = true
	next, _ := app.Update(tea.BlurMsg{})
	next, cmd := next.(App).Update(tea.FocusMsg{})
	if cmd == nil || !next.(App).listFetching {
		t.Fatal("regaining focus did not refetch the chat list")
	}
}

func TestDeletedElsewhereDropsChat(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.showAll = true
	app.chats = []store.Entry{{ID: "a"}, {ID: "b"}}
	next, _ := app.Update(deletedMsg{id: "a", gone: true})
	app = next.(App)
	if len(app.chats) != 1 || app.chats[0].ID != "b" {
		t.Fatalf("chats = %+v", app.chats)
	}
	if app.status == "" {
		t.Fatal("no status for a chat deleted elsewhere")
	}
}

func TestReplaceChatsKeepsSelection(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.showAll = true
	app.chats = []store.Entry{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	app.selected = 1
	app.replaceChats([]store.Entry{{ID: "new"}, {ID: "a"}, {ID: "b"}, {ID: "c"}})
	if got := app.visibleChats()[app.selected].ID; got != "b" {
		t.Fatalf("selection moved to %q", got)
	}
}
