package socket

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseOpen(t *testing.T) {
	p, ok := ParseOpen(`0{"sid":"abc","upgrades":[],"pingInterval":25000,"pingTimeout":20000}`)
	if !ok || p.SID != "abc" {
		t.Fatalf("%+v %v", p, ok)
	}
	if _, ok := ParseOpen("2"); ok {
		t.Fatal("ping is not an open packet")
	}
}

func TestParseSocketEvent(t *testing.T) {
	pkt := `42["events",{"chat_id":"c1","message_id":"m1","data":{"type":"chat:completion","data":{"content":"Hi","done":false}}}]`
	ev, ok := ParseSocketEvent(pkt)
	if !ok {
		t.Fatal("not ok")
	}
	if ev.Name != "events" || ev.ChatID != "c1" || ev.MessageID != "m1" || ev.Type != "chat:completion" {
		t.Fatalf("%+v", ev)
	}
	if string(ev.Data) == "" || ev.Data[0] != '{' {
		t.Fatalf("data = %s", ev.Data)
	}
}

func TestMarkReadPacket(t *testing.T) {
	pkt := string(MarkReadPacket("c1"))
	if !strings.HasPrefix(pkt, "42") {
		t.Fatal(pkt)
	}
	var frame []json.RawMessage
	if err := json.Unmarshal([]byte(pkt[2:]), &frame); err != nil || len(frame) != 2 {
		t.Fatal(pkt)
	}
	var name string
	if json.Unmarshal(frame[0], &name) != nil || name != "events:chat" {
		t.Fatal(name)
	}
	var body struct {
		ChatID string `json:"chat_id"`
		Data   struct {
			Type string `json:"type"`
		} `json:"data"`
	}
	if json.Unmarshal(frame[1], &body) != nil || body.ChatID != "c1" || body.Data.Type != "last_read_at" {
		t.Fatalf("%+v", body)
	}
}

func TestParseNamespacedEvent(t *testing.T) {
	pkt := `42/admin,["events",{"chat_id":"c","message_id":"m","data":{"type":"status","data":{"description":"searching"}}}]`
	ev, ok := ParseSocketEvent(pkt)
	if !ok || ev.Type != "status" {
		t.Fatalf("%+v %v", ev, ok)
	}
}
