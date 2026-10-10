package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/usr1/owui/internal/api"
	"github.com/usr1/owui/internal/config"
	"github.com/usr1/owui/internal/socket"
)

func streamingApp(t *testing.T) App {
	t.Helper()
	app := newApp(config.File{Path: t.TempDir() + "/config.toml"}, nil)
	app.phase = phaseMain
	app.chatID = "c1"
	app.vp.SetWidth(80)
	app.history = api.History{
		CurrentID: "a1",
		Messages: map[string]api.Message{
			"u1": {ID: "u1", Role: "user", Content: "hi", ChildrenIDs: api.IDList{"a1"}},
			"a1": {ID: "a1", Role: "assistant", ParentID: "u1", ChildrenIDs: api.IDList{}},
		},
	}
	app.jobs = map[string]*genJob{"a1": {chatID: "c1", assistant: "a1", seq: 1}}
	return app
}

func feed(t *testing.T, app *App, frame string) {
	t.Helper()
	ev, ok := socket.ParseSocketEvent(frame)
	if !ok {
		t.Fatalf("did not parse %s", frame)
	}
	app.onEvent(ev)
}

// Open WebUI 0.11 streams text as response:completion deltas.
func TestResponseDeltasStream(t *testing.T) {
	app := streamingApp(t)
	feed(t, &app, `42["events",{"chat_id":"c1","message_id":"a1","data":{"type":"response:completion","data":{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hello"}}}]`)
	feed(t, &app, `42["events",{"chat_id":"c1","message_id":"a1","data":{"type":"response:completion","data":{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":" there"}}}]`)

	plain := ansi.Strip(app.transcript())
	if !strings.Contains(plain, "Hello there") {
		t.Fatalf("transcript:\n%s", plain)
	}
	if app.jobs["a1"].lastEvent.IsZero() {
		t.Fatal("streamed event did not mark the job as fed by the socket")
	}
}

func TestOutputSnapshotReplaces(t *testing.T) {
	app := streamingApp(t)
	feed(t, &app, `42["events",{"chat_id":"c1","message_id":"a1","data":{"type":"response:completion","data":{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"draft"}}}]`)
	feed(t, &app, `42["events",{"chat_id":"c1","message_id":"a1","data":{"type":"chat:completion","data":{"output":[{"type":"message","content":[{"type":"output_text","text":"final answer"}]}],"flush":true}}}]`)

	plain := ansi.Strip(app.transcript())
	if !strings.Contains(plain, "final answer") || strings.Contains(plain, "draft") {
		t.Fatalf("transcript:\n%s", plain)
	}
}

func TestPollDoesNotRewindStream(t *testing.T) {
	ahead := `[{"type":"message","content":[{"type":"output_text","text":"Hello there friend"}]}]`
	behind := `[{"type":"message","content":[{"type":"output_text","text":"Hello"}]}]`
	local := api.History{Messages: map[string]api.Message{"a1": {ID: "a1", Output: []byte(ahead)}}}
	remote := api.History{Messages: map[string]api.Message{"a1": {ID: "a1", Output: []byte(behind)}}}
	got := mergeHistory(local, remote)
	if string(got.Messages["a1"].Output) != ahead {
		t.Fatalf("output = %s", got.Messages["a1"].Output)
	}
}

func TestPollSlowsWhileSocketStreams(t *testing.T) {
	j := &genJob{}
	if got := pollEvery(j); got != time.Second {
		t.Fatalf("silent socket polls every %v", got)
	}
	j.lastEvent = time.Now()
	if got := pollEvery(j); got != 5*time.Second {
		t.Fatalf("streaming socket polls every %v", got)
	}
}
