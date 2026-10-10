package tui

import (
	"testing"

	"github.com/papodaca/owui/internal/api"
	"github.com/papodaca/owui/internal/config"
)

func openedChat(done bool) loadedMsg {
	d := done
	return loadedMsg{why: "open", rec: api.ChatRecord{
		ID: "c1",
		Chat: api.ChatBody{History: &api.History{
			CurrentID: "a1",
			Messages: map[string]api.Message{
				"u1": {ID: "u1", Role: "user", Content: "hi"},
				"a1": {ID: "a1", Role: "assistant", ParentID: "u1", Content: "partial", Done: &d},
			},
		}},
	}}
}

func TestOpeningRunningChatFollowsIt(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	if cmd := app.applyLoaded(openedChat(false)); cmd == nil {
		t.Fatal("no poll scheduled")
	}
	j := app.jobByChat("c1")
	if j == nil || !j.adopted || j.assistant != "a1" {
		t.Fatalf("job = %+v", j)
	}

	app.applyPoll(pollResult{id: "c1", assistant: "a1", tasks: []string{}, rec: openedChat(false).rec})
	if app.jobByChat("c1") != nil {
		t.Fatal("a reply with no server task kept being followed")
	}
}

func TestOpeningFinishedChatAdoptsNothing(t *testing.T) {
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.applyLoaded(openedChat(true))
	if len(app.jobs) != 0 {
		t.Fatalf("jobs = %+v", app.jobs)
	}
}
