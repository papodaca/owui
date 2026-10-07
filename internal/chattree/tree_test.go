package chattree

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/usr1/owui/internal/api"
)

func TestCreateBodyTree(t *testing.T) {
	turn, body := CreateBody("Hello there", "m1", "Model", "Hi", 100)
	if body.History.CurrentID != turn.AssistantID {
		t.Fatalf("currentId = %s", body.History.CurrentID)
	}
	user := body.History.Messages[turn.UserID]
	asst := body.History.Messages[turn.AssistantID]
	if len(user.ChildrenIDs) != 1 || user.ChildrenIDs[0] != turn.AssistantID {
		t.Fatalf("user children = %#v", user.ChildrenIDs)
	}
	if asst.ParentID != turn.UserID || len(asst.ChildrenIDs) != 0 {
		t.Fatalf("assistant parent=%s children=%#v", asst.ParentID, asst.ChildrenIDs)
	}
	if user.ParentID != "" {
		t.Fatalf("root parent = %q", user.ParentID)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatal("invalid json")
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	hist := decoded["history"].(map[string]any)
	if _, ok := hist["currentId"]; !ok {
		t.Fatal("currentId missing")
	}
	if _, ok := hist["current_id"]; ok {
		t.Fatal("current_id must not be sent")
	}
	msgs := CompletionMessages(*body.History)
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "Hi" {
		t.Fatalf("completion messages = %#v", msgs)
	}
}

func TestAppendPatchLinksPrevious(t *testing.T) {
	turn, patch := AppendPatch("asst-1", "m1", "Model", "more", 200)
	hist := patch["history"].(map[string]any)
	if hist["currentId"] != turn.AssistantID {
		t.Fatalf("currentId = %v", hist["currentId"])
	}
	messages := hist["messages"].(map[string]any)
	prev := messages["asst-1"].(map[string]any)
	kids := prev["childrenIds"].([]string)
	if len(kids) != 1 || kids[0] != turn.UserID {
		t.Fatalf("prev children = %#v", kids)
	}
	if _, ok := prev["content"]; ok {
		t.Fatal("partial update must not resend content")
	}
	user := messages[turn.UserID].(api.Message)
	if user.ParentID != "asst-1" {
		t.Fatalf("parent = %s", user.ParentID)
	}

	h := api.History{
		CurrentID: turn.AssistantID,
		Messages: map[string]api.Message{
			"user-0":         {ID: "user-0", Role: "user", Content: "Hi", ChildrenIDs: api.IDList{"asst-1"}},
			"asst-1":         {ID: "asst-1", Role: "assistant", Content: "Hello", ParentID: "user-0", ChildrenIDs: api.IDList{turn.UserID}},
			turn.UserID:      turn.User,
			turn.AssistantID: turn.Assistant,
		},
	}
	msgs := CompletionMessages(h)
	if len(msgs) != 3 {
		t.Fatalf("len = %d %#v", len(msgs), msgs)
	}
	if msgs[2].Content != "more" {
		t.Fatalf("last = %s", msgs[2].Content)
	}
}

func TestTitleFromPromptStaysOneSidebarLine(t *testing.T) {
	got := TitleFromPrompt("using web search write me a greeting for the team\nplease")
	if strings.Contains(got, "\n") {
		t.Fatalf("title has a newline: %q", got)
	}
	if w := ansi.StringWidth(got); w > 28 {
		t.Fatalf("width %d, title %q", w, got)
	}
}

func TestChainStopsOnCycle(t *testing.T) {
	h := api.History{
		CurrentID: "a",
		Messages: map[string]api.Message{
			"a": {ID: "a", Role: "user", ParentID: "b", Content: "a"},
			"b": {ID: "b", Role: "assistant", ParentID: "a", Content: "b"},
		},
	}
	got := Chain(h)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
}
