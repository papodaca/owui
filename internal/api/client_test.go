package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateAndComplete(t *testing.T) {
	var gotComplete map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("auth = %s", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/api/v1/chats/new":
			var wrap struct {
				Chat ChatBody `json:"chat"`
			}
			if err := json.Unmarshal(body, &wrap); err != nil {
				t.Fatal(err)
			}
			if wrap.Chat.History == nil || wrap.Chat.History.CurrentID == "" {
				t.Fatalf("history = %#v", wrap.Chat.History)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ChatRecord{ID: "chat-1", Chat: wrap.Chat})
		case "/api/chat/completions":
			if err := json.Unmarshal(body, &gotComplete); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(CompletionAck{Status: true, TaskIDs: []string{"task-1"}, ChatID: "chat-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "sk-test")
	turnBody := ChatBody{
		Title:  "Hi",
		Models: []string{"m"},
		History: &History{
			CurrentID: "a",
			Messages: map[string]Message{
				"u": {ID: "u", Role: "user", Content: "Hi", ChildrenIDs: IDList{"a"}},
				"a": {ID: "a", Role: "assistant", ParentID: "u", ChildrenIDs: IDList{}, Done: flag(false)},
			},
		},
		CurrentID: "a",
	}
	rec, err := c.CreateChat(context.Background(), turnBody)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "chat-1" {
		t.Fatalf("id = %s", rec.ID)
	}
	ack, err := c.Complete(context.Background(), CompletionRequest{
		Model:     "m",
		Messages:  []CompletionMessage{{Role: "user", Content: "Hi"}},
		Stream:    true,
		ChatID:    rec.ID,
		ID:        "a",
		SessionID: "sid",
		Features:  Features{WebSearch: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ack.Status || len(ack.TaskIDs) != 1 {
		t.Fatalf("ack = %#v", ack)
	}
	if gotComplete["stream"] != true {
		t.Fatalf("stream = %v", gotComplete["stream"])
	}
	if _, ok := gotComplete["tools"]; ok {
		t.Fatal("tools key opts out of server-side tools")
	}
	if gotComplete["chat_id"] != "chat-1" || gotComplete["session_id"] != "sid" {
		t.Fatalf("body = %#v", gotComplete)
	}
	feats, _ := gotComplete["features"].(map[string]any)
	if feats["web_search"] != true {
		t.Fatalf("features = %#v", feats)
	}
}

func TestListChats(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.RequestURI()
		_, _ = w.Write([]byte(`[{"id":"c1","title":"Hello","updated_at":10}]`))
	}))
	defer srv.Close()
	list, err := New(srv.URL, "k").ListChats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "c1" || list[0].Title != "Hello" {
		t.Fatalf("%#v", list)
	}
	if !strings.Contains(path, "include_pinned=true") || !strings.Contains(path, "include_folders=true") {
		t.Fatalf("path = %s", path)
	}
}

func TestServerFeaturesAbsentMeansOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"features":{"enable_web_search":false}}`))
	}))
	defer srv.Close()
	feats, err := New(srv.URL, "k").ServerFeatures(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if feats.WebOn() {
		t.Fatal("web search should be off")
	}
	if !feats.ImageOn() || !feats.CodeOn() {
		t.Fatal("missing flags should stay available")
	}
}

func TestTerminalChatUsable(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"", true},
		{`{"chat":false}`, false},
		{`{"chat":{"context_id":"chat_id"}}`, true},
		{`{"chat":{}}`, true},
	}
	for _, tc := range cases {
		term := Terminal{ID: "t", Contexts: []byte(tc.raw)}
		if tc.raw == "" {
			term.Contexts = nil
		}
		if got := term.ChatUsable(); got != tc.want {
			t.Errorf("%s: got %v", tc.raw, got)
		}
	}
}

func flag(v bool) *bool { return &v }
