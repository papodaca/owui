package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

func (c *Client) SignIn(ctx context.Context, email, password string) (User, error) {
	var u User
	err := c.do(ctx, http.MethodPost, "/api/v1/auths/signin", map[string]string{
		"email":    email,
		"password": password,
	}, &u)
	return u, err
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	err := c.do(ctx, http.MethodGet, "/api/v1/auths/", nil, &u)
	return u, err
}

func (c *Client) ServerFeatures(ctx context.Context) (ServerFeatures, error) {
	var raw map[string]json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/config", nil, &raw); err != nil {
		return ServerFeatures{}, err
	}
	var features map[string]json.RawMessage
	if b, ok := raw["features"]; ok {
		_ = json.Unmarshal(b, &features)
	}
	return ServerFeatures{
		WebSearch:       boolPtr(features, "enable_web_search"),
		ImageGeneration: boolPtr(features, "enable_image_generation"),
		CodeInterpreter: firstBool(features, "enable_code_interpreter", "enable_code_execution"),
	}, nil
}

func boolPtr(m map[string]json.RawMessage, key string) *bool {
	if m == nil {
		return nil
	}
	b, ok := m[key]
	if !ok {
		return nil
	}
	var v bool
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	return &v
}

func firstBool(m map[string]json.RawMessage, keys ...string) *bool {
	for _, k := range keys {
		if v := boolPtr(m, k); v != nil {
			return v
		}
	}
	return nil
}

func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var wrap struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/models", nil, &wrap); err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(wrap.Data))
	for _, raw := range wrap.Data {
		m, ok := parseModel(raw)
		if ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func parseModel(raw json.RawMessage) (Model, bool) {
	var row struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Info struct {
			Meta struct {
				Capabilities map[string]bool `json:"capabilities"`
				ToolIDs      []string        `json:"toolIds"`
				TerminalID   string          `json:"terminalId"`
			} `json:"meta"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &row); err != nil || row.ID == "" {
		return Model{}, false
	}
	name := row.Name
	if name == "" {
		name = row.ID
	}
	return Model{
		ID:           row.ID,
		Name:         name,
		Capabilities: row.Info.Meta.Capabilities,
		ToolIDs:      row.Info.Meta.ToolIDs,
		TerminalID:   row.Info.Meta.TerminalID,
	}, true
}

func (c *Client) Tools(ctx context.Context) ([]Tool, error) {
	var tools []Tool
	err := c.do(ctx, http.MethodGet, "/api/v1/tools/", nil, &tools)
	if err != nil {
		return nil, err
	}
	if tools == nil {
		tools = []Tool{}
	}
	return tools, nil
}

func (c *Client) Terminals(ctx context.Context) ([]Terminal, error) {
	var terms []Terminal
	err := c.do(ctx, http.MethodGet, "/api/v1/terminals/", nil, &terms)
	if err != nil {
		return nil, err
	}
	if terms == nil {
		terms = []Terminal{}
	}
	out := terms[:0]
	for _, t := range terms {
		if t.ID != "" && t.ChatUsable() {
			out = append(out, t)
		}
	}
	return out, nil
}

// ListChats returns the signed-in user's chats, including pinned ones and
// chats that live in folders. Archived chats stay out, same as the web sidebar.
func (c *Client) ListChats(ctx context.Context) ([]ChatSummary, error) {
	var list []ChatSummary
	err := c.do(ctx, http.MethodGet, "/api/v1/chats/?include_pinned=true&include_folders=true", nil, &list)
	if list == nil {
		list = []ChatSummary{}
	}
	return list, err
}

func (c *Client) CreateChat(ctx context.Context, chat any) (ChatRecord, error) {
	var rec ChatRecord
	err := c.do(ctx, http.MethodPost, "/api/v1/chats/new", map[string]any{"chat": chat}, &rec)
	return rec, err
}

func chatPath(id string) string {
	return "/api/v1/chats/" + url.PathEscape(id)
}

func (c *Client) GetChat(ctx context.Context, id string) (ChatRecord, error) {
	var rec ChatRecord
	err := c.do(ctx, http.MethodGet, chatPath(id), nil, &rec)
	return rec, err
}

func (c *Client) UpdateChat(ctx context.Context, id string, chat any) (ChatRecord, error) {
	var rec ChatRecord
	err := c.do(ctx, http.MethodPost, chatPath(id), map[string]any{"chat": chat}, &rec)
	return rec, err
}

func (c *Client) DeleteChat(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, chatPath(id), nil, nil)
}

func (c *Client) Complete(ctx context.Context, req CompletionRequest) (CompletionAck, error) {
	var ack CompletionAck
	err := c.do(ctx, http.MethodPost, "/api/chat/completions", req, &ack)
	return ack, err
}

func (c *Client) ChatTasks(ctx context.Context, chatID string) ([]string, error) {
	var wrap struct {
		TaskIDs []string `json:"task_ids"`
	}
	err := c.do(ctx, http.MethodGet, "/api/tasks/chat/"+url.PathEscape(chatID), nil, &wrap)
	if wrap.TaskIDs == nil {
		wrap.TaskIDs = []string{}
	}
	return wrap.TaskIDs, err
}

func (c *Client) StopTask(ctx context.Context, taskID string) error {
	return c.do(ctx, http.MethodPost, "/api/tasks/stop/"+url.PathEscape(taskID), map[string]any{}, nil)
}
