package api

import "encoding/json"

// UnixTime is a unix timestamp. Open WebUI sends seconds for messages and
// sometimes milliseconds for chat rows, both as JSON numbers.
type UnixTime int64

func (t *UnixTime) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || len(b) == 0 {
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*t = UnixTime(n)
	return nil
}

func (t UnixTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(t))
}

// Content is chat text. The server sometimes sends a string and sometimes a
// list of content parts. Non-string JSON is kept raw so it round-trips.
type Content string

func (c *Content) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*c = Content(s)
		return nil
	}
	*c = Content(b)
	return nil
}

func (c Content) MarshalJSON() ([]byte, error) {
	if len(c) > 0 && (c[0] == '[' || c[0] == '{') {
		return []byte(c), nil
	}
	return json.Marshal(string(c))
}

func (c Content) String() string { return string(c) }

// IDList is childrenIds. The server sends null for messages that have none.
type IDList []string

func (l *IDList) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || len(b) == 0 {
		*l = []string{}
		return nil
	}
	var s []string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == nil {
		s = []string{}
	}
	*l = s
	return nil
}

func (l IDList) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(l))
}

// Message is one node in the chat tree the web UI renders.
type Message struct {
	ID          string          `json:"id"`
	Role        string          `json:"role"`
	Content     Content         `json:"content"`
	Timestamp   UnixTime        `json:"timestamp,omitempty"`
	Models      []string        `json:"models,omitempty"`
	ChildrenIDs IDList          `json:"childrenIds"`
	ParentID    string          `json:"parentId,omitempty"`
	Model       string          `json:"model,omitempty"`
	ModelName   string          `json:"modelName,omitempty"`
	ModelIdx    int             `json:"modelIdx,omitempty"`
	Done        *bool           `json:"done,omitempty"`
	Sources     json.RawMessage `json:"sources,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
}

// History is the message tree. currentId is camelCase. The web UI ignores current_id.
type History struct {
	CurrentID string             `json:"currentId"`
	Messages  map[string]Message `json:"messages"`
}

// ChatBody is the JSON object stored on a chat.
type ChatBody struct {
	Title     string    `json:"title,omitempty"`
	Models    []string  `json:"models,omitempty"`
	Messages  []Message `json:"messages,omitempty"`
	History   *History  `json:"history,omitempty"`
	CurrentID string    `json:"currentId,omitempty"`
}

// ChatRecord is a row from /api/v1/chats.
type ChatRecord struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	UserID    string   `json:"user_id"`
	Chat      ChatBody `json:"chat"`
	UpdatedAt UnixTime `json:"updated_at"`
	CreatedAt UnixTime `json:"created_at"`
}

// DisplayTitle prefers the column title, then the title inside the chat JSON.
func (r ChatRecord) DisplayTitle() string {
	if r.Title != "" {
		return r.Title
	}
	if r.Chat.Title != "" {
		return r.Chat.Title
	}
	return "untitled"
}

// User is the account behind the API key or JWT.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	Token string `json:"token"`
}

// Model is one entry from GET /api/models.
type Model struct {
	ID           string
	Name         string
	Capabilities map[string]bool
	ToolIDs      []string
	TerminalID   string
}

// Allows reports whether a capability is on. A missing key means the server
// did not say, so the toggle stays available.
func (m Model) Allows(capability string) bool {
	if m.Capabilities == nil {
		return true
	}
	on, ok := m.Capabilities[capability]
	if !ok {
		return true
	}
	return on
}

// Tool is a workspace tool or an MCP server. MCP ids look like server:mcp:<id>.
type Tool struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Terminal is an Open Terminal connection the user can attach to a chat.
type Terminal struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Contexts json.RawMessage `json:"contexts"`
}

// ChatUsable reports whether this terminal can be selected in a chat.
// contexts.chat false means an admin removed it from chats.
func (t Terminal) ChatUsable() bool {
	if len(t.Contexts) == 0 || string(t.Contexts) == "null" {
		return true
	}
	var wrap struct {
		Chat json.RawMessage `json:"chat"`
	}
	if err := json.Unmarshal(t.Contexts, &wrap); err != nil {
		return true
	}
	if len(wrap.Chat) == 0 || string(wrap.Chat) == "null" {
		return true
	}
	if string(wrap.Chat) == "false" {
		return false
	}
	return true
}

// Features are the four toggles the web composer shows.
type Features struct {
	WebSearch       bool `json:"web_search"`
	CodeInterpreter bool `json:"code_interpreter"`
	ImageGeneration bool `json:"image_generation"`
	Memory          bool `json:"memory"`
}

// BackgroundTasks controls title, tag, and follow-up generation.
type BackgroundTasks struct {
	TitleGeneration    bool `json:"title_generation"`
	TagsGeneration     bool `json:"tags_generation"`
	FollowUpGeneration bool `json:"follow_up_generation"`
}

// CompletionMessage is one turn sent to /api/chat/completions.
type CompletionMessage struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

// CompletionRequest is the body that makes Open WebUI run tools and store the chat.
type CompletionRequest struct {
	Model           string              `json:"model"`
	Messages        []CompletionMessage `json:"messages"`
	Stream          bool                `json:"stream"`
	ChatID          string              `json:"chat_id"`
	ID              string              `json:"id"`
	SessionID       string              `json:"session_id"`
	Features        Features            `json:"features"`
	ToolIDs         []string            `json:"tool_ids,omitempty"`
	TerminalID      string              `json:"terminal_id,omitempty"`
	BackgroundTasks BackgroundTasks     `json:"background_tasks"`
	Variables       map[string]string   `json:"variables,omitempty"`
}

// CompletionAck is what /api/chat/completions returns when session_id is set.
// The answer itself is written into the chat, not this body.
type CompletionAck struct {
	Status  bool     `json:"status"`
	TaskIDs []string `json:"task_ids"`
	ChatID  string   `json:"chat_id"`
}

// ServerFeatures are the admin toggles from GET /api/config.
// A nil pointer means the field was absent, which we treat as available.
type ServerFeatures struct {
	WebSearch       *bool
	ImageGeneration *bool
	CodeInterpreter *bool
}

// Enabled is true when the server did not send the flag or sent true.
func enabled(v *bool) bool { return v == nil || *v }

func (s ServerFeatures) WebOn() bool   { return enabled(s.WebSearch) }
func (s ServerFeatures) ImageOn() bool { return enabled(s.ImageGeneration) }
func (s ServerFeatures) CodeOn() bool  { return enabled(s.CodeInterpreter) }
