// Package socket is a small Engine.IO v4 / Socket.IO client.
// Open WebUI pushes chat tokens on the "events" event after the socket joins
// the user room during connect. The connect packet carries auth.token.
package socket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Event is one Socket.IO event, or a local connection notice.
type Event struct {
	Name      string
	ChatID    string
	MessageID string
	Type      string
	Data      json.RawMessage
	// Local is true for notices this client invented (connected, disconnected).
	Local bool
	Err   string
}

// Client keeps one websocket open and reconnects when it drops.
type Client struct {
	BaseURL string
	Token   string

	Events chan Event

	mu      sync.RWMutex
	sid     string
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		Events:  make(chan Event, 256),
	}
}

func (c *Client) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sid
}

func (c *Client) setSID(sid string) {
	c.mu.Lock()
	c.sid = sid
	c.mu.Unlock()
}

func (c *Client) setConn(conn *websocket.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

func (c *Client) clearConn(conn *websocket.Conn) {
	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
	}
	c.mu.Unlock()
}

func (c *Client) writePacket(ctx context.Context, pkt []byte) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return errNotConnected
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.Write(ctx, websocket.MessageText, pkt)
}

// MarkRead tells the server this chat was opened, the same signal the web UI
// sends so the sidebar unread light goes out.
func (c *Client) MarkRead(ctx context.Context, chatID string) error {
	if chatID == "" {
		return nil
	}
	return c.writePacket(ctx, MarkReadPacket(chatID))
}

// MarkReadPacket is the Socket.IO event the server handles as last_read_at.
func MarkReadPacket(chatID string) []byte {
	payload, _ := json.Marshal([]any{
		"events:chat",
		map[string]any{
			"chat_id": chatID,
			"data":    map[string]string{"type": "last_read_at"},
		},
	})
	return append([]byte("42"), payload...)
}

var errNotConnected = errStringType("socket is not connected")

// Run dials until ctx is cancelled. Call it in a goroutine.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.session(ctx)
		c.setSID("")
		if ctx.Err() != nil {
			return
		}
		c.emit(Event{Name: "local", Type: "socket:disconnected", Local: true, Err: errString(err)})
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 15*time.Second {
			backoff *= 2
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (c *Client) emit(ev Event) {
	select {
	case c.Events <- ev:
	default:
	}
}

func (c *Client) wsURL() (string, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	base := strings.TrimRight(u.Path, "/")
	u.Path = base + "/ws/socket.io/"
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) session(ctx context.Context) error {
	wsURL, err := c.wsURL()
	if err != nil {
		return err
	}
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + c.Token}},
	})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(8 << 20)

	_, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	open, ok := ParseOpen(string(data))
	if !ok {
		return errBadOpen
	}
	c.setSID(open.SID)
	c.setConn(conn)
	defer c.clearConn(conn)
	auth, _ := json.Marshal(map[string]string{"token": c.Token})
	if err := c.writePacket(ctx, []byte("40"+string(auth))); err != nil {
		return err
	}
	c.emit(Event{Name: "local", Type: "socket:connected", Local: true})

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		pkt := string(data)
		if pkt == "" {
			continue
		}
		switch pkt[0] {
		case '2':
			if err := c.writePacket(ctx, []byte("3")); err != nil {
				return err
			}
		case '1':
			return nil
		case '4':
			if ev, ok := ParseSocketEvent(pkt); ok {
				c.emit(ev)
			}
		}
	}
}

type openPacket struct {
	SID string `json:"sid"`
}

var errBadOpen = errStringType("engine.io open packet missing sid")

type errStringType string

func (e errStringType) Error() string { return string(e) }

// ParseOpen reads the Engine.IO open packet (0{...}).
func ParseOpen(pkt string) (openPacket, bool) {
	if !strings.HasPrefix(pkt, "0") {
		return openPacket{}, false
	}
	var p openPacket
	if err := json.Unmarshal([]byte(pkt[1:]), &p); err != nil || p.SID == "" {
		return openPacket{}, false
	}
	return p, true
}

// ParseSocketEvent reads an Engine.IO message that wraps a Socket.IO event.
// The shape is 42["events", {...}] on the default namespace.
func ParseSocketEvent(pkt string) (Event, bool) {
	if !strings.HasPrefix(pkt, "4") {
		return Event{}, false
	}
	rest := pkt[1:]
	if rest == "" || rest[0] != '2' {
		return Event{}, false
	}
	rest = rest[1:]
	// Optional namespace: 42/admin,["event", data]
	if strings.HasPrefix(rest, "/") {
		comma := strings.IndexByte(rest, ',')
		if comma < 0 {
			return Event{}, false
		}
		rest = rest[comma+1:]
	}
	// Optional ack id.
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	rest = rest[i:]
	if !strings.HasPrefix(rest, "[") {
		return Event{}, false
	}
	var frame []json.RawMessage
	if err := json.Unmarshal([]byte(rest), &frame); err != nil || len(frame) < 1 {
		return Event{}, false
	}
	var name string
	if err := json.Unmarshal(frame[0], &name); err != nil {
		return Event{}, false
	}
	ev := Event{Name: name}
	if len(frame) < 2 {
		return ev, true
	}
	if name != "events" {
		ev.Data = frame[1]
		return ev, true
	}
	var payload struct {
		ChatID    string          `json:"chat_id"`
		MessageID string          `json:"message_id"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(frame[1], &payload); err != nil {
		ev.Data = frame[1]
		return ev, true
	}
	ev.ChatID = payload.ChatID
	ev.MessageID = payload.MessageID
	ev.Data = payload.Data
	var inner struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(payload.Data, &inner) == nil {
		ev.Type = inner.Type
		if len(inner.Data) > 0 {
			ev.Data = inner.Data
		}
	}
	return ev, true
}
