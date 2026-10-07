// Package chattree builds the message tree Open WebUI's web UI renders.
// Message ids are caller-generated. currentId is camelCase. Every message
// carries childrenIds, and every message except the root carries parentId.
package chattree

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"github.com/usr1/owui/internal/api"
)

func newID() string { return uuid.NewString() }

func boolPtr(v bool) *bool { return &v }

// Turn is a user message plus the empty assistant placeholder the completion writes into.
type Turn struct {
	UserID      string
	AssistantID string
	User        api.Message
	Assistant   api.Message
}

func newTurn(modelID, modelName, prompt, parentID string, now int64) Turn {
	userID, asstID := newID(), newID()
	user := api.Message{
		ID:          userID,
		Role:        "user",
		Content:     api.Content(prompt),
		Timestamp:   api.UnixTime(now),
		Models:      []string{modelID},
		ChildrenIDs: api.IDList{asstID},
		ParentID:    parentID,
	}
	asst := api.Message{
		ID:          asstID,
		Role:        "assistant",
		Content:     "",
		ParentID:    userID,
		ChildrenIDs: api.IDList{},
		Model:       modelID,
		ModelName:   modelName,
		ModelIdx:    0,
		Done:        boolPtr(false),
		Timestamp:   api.UnixTime(now + 1),
	}
	return Turn{UserID: userID, AssistantID: asstID, User: user, Assistant: asst}
}

// CreateBody is the payload for POST /api/v1/chats/new.
// The messages array and history.messages both contain the same two nodes.
// The web UI walks history. The messages array is what older clients read.
func CreateBody(title, modelID, modelName, prompt string, now int64) (Turn, api.ChatBody) {
	if now == 0 {
		now = time.Now().Unix()
	}
	if title == "" {
		title = "New Chat"
	}
	turn := newTurn(modelID, modelName, prompt, "", now)
	hist := &api.History{
		CurrentID: turn.AssistantID,
		Messages: map[string]api.Message{
			turn.UserID:      turn.User,
			turn.AssistantID: turn.Assistant,
		},
	}
	body := api.ChatBody{
		Title:     title,
		Models:    []string{modelID},
		Messages:  []api.Message{turn.User, turn.Assistant},
		History:   hist,
		CurrentID: turn.AssistantID,
	}
	return turn, body
}

// AppendPatch is the payload for POST /api/v1/chats/{id} on a later turn.
// The previous assistant message is sent as a partial object with only
// childrenIds, so a deep merge keeps the reply the server already stored.
func AppendPatch(prevCurrentID, modelID, modelName, prompt string, now int64) (Turn, map[string]any) {
	if now == 0 {
		now = time.Now().Unix()
	}
	turn := newTurn(modelID, modelName, prompt, prevCurrentID, now)
	patch := map[string]any{
		"currentId": turn.AssistantID,
		"history": map[string]any{
			"currentId": turn.AssistantID,
			"messages": map[string]any{
				prevCurrentID: map[string]any{
					"childrenIds": []string{turn.UserID},
				},
				turn.UserID:      turn.User,
				turn.AssistantID: turn.Assistant,
			},
		},
	}
	return turn, patch
}

// Chain walks parentId from currentId back to the root and returns the thread
// in display order.
func Chain(h api.History) []api.Message {
	if h.CurrentID == "" || h.Messages == nil {
		return nil
	}
	var rev []api.Message
	seen := map[string]bool{}
	id := h.CurrentID
	for id != "" && !seen[id] {
		seen[id] = true
		msg, ok := h.Messages[id]
		if !ok {
			break
		}
		rev = append(rev, msg)
		id = msg.ParentID
	}
	out := make([]api.Message, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// CompletionMessages is the messages array for /api/chat/completions.
// The empty assistant placeholder at the end is left out. The server fills it.
func CompletionMessages(h api.History) []api.CompletionMessage {
	chain := Chain(h)
	if len(chain) == 0 {
		return nil
	}
	last := chain[len(chain)-1]
	if last.Role == "assistant" && last.Content == "" {
		chain = chain[:len(chain)-1]
	}
	out := make([]api.CompletionMessage, 0, len(chain))
	for _, msg := range chain {
		if msg.Role == "" {
			continue
		}
		out = append(out, api.CompletionMessage{Role: msg.Role, Content: msg.Content})
	}
	return out
}

// TitleFromPrompt is a one-line stand-in until the server generates a title.
// It has to fit the sidebar column, which is about 30 cells wide.
func TitleFromPrompt(prompt string) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	return ansi.Truncate(prompt, 28, "…")
}
