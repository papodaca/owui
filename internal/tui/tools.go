package tui

import (
	"encoding/json"
	"html"
	"strings"
)

// contentPart is either markdown prose or one tool call. Newer chats store the
// calls on message.output as function_call items. Older chats embed them in the
// text as <details type="tool_calls">. The transcript lists each name and
// leaves the result out.
type contentPart struct {
	text    string
	tool    string
	running bool
}

func messageParts(content string, output json.RawMessage) []contentPart {
	if parts, ok := partsFromOutput(output); ok {
		if partsHaveText(parts) {
			return parts
		}
		return append(parts, splitToolCalls(content)...)
	}
	return splitToolCalls(content)
}

func partsHaveText(parts []contentPart) bool {
	for _, p := range parts {
		if p.tool == "" && strings.TrimSpace(p.text) != "" {
			return true
		}
	}
	return false
}

func partsFromOutput(raw json.RawMessage) ([]contentPart, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return nil, false
	}
	var parts []contentPart
	var tools []contentPart
	flush := func() {
		parts = append(parts, tools...)
		tools = nil
	}
	saw := false
	for _, item := range items {
		typ, _ := item["type"].(string)
		switch typ {
		case "function_call_output":
			continue
		case "function_call", "web_search_call", "file_search_call":
			saw = true
			name, _ := item["name"].(string)
			if name == "" {
				name = typ
			}
			status, _ := item["status"].(string)
			running := status == "in_progress" || status == "incomplete"
			tools = append(tools, contentPart{tool: name, running: running})
		case "message":
			text := outputMessageText(item)
			if strings.TrimSpace(text) == "" {
				continue
			}
			flush()
			saw = true
			parts = append(parts, splitToolCalls(text)...)
		}
	}
	flush()
	if !saw {
		return nil, false
	}
	return parts, true
}

func outputMessageText(item map[string]any) string {
	switch c := item["content"].(type) {
	case string:
		return c
	case []any:
		var b strings.Builder
		for _, part := range c {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t != "" && t != "output_text" && t != "text" {
				continue
			}
			if s, ok := m["text"].(string); ok {
				b.WriteString(s)
			}
		}
		return b.String()
	default:
		return ""
	}
}

func splitToolCalls(s string) []contentPart {
	if !strings.Contains(strings.ToLower(s), "<details") {
		return []contentPart{{text: s}}
	}
	var parts []contentPart
	rest := s
	for {
		i := indexFold(rest, "<details")
		if i < 0 {
			parts = appendText(parts, rest)
			break
		}
		rel := strings.IndexByte(rest[i:], '>')
		if rel < 0 {
			parts = appendText(parts, rest[:i])
			break
		}
		attrs := rest[i+len("<details") : i+rel]
		if htmlAttr(attrs, "type") != "tool_calls" {
			parts = appendText(parts, rest[:i+rel+1])
			rest = rest[i+rel+1:]
			continue
		}
		parts = appendText(parts, rest[:i])
		name := htmlAttr(attrs, "name")
		if name == "" {
			name = "tool"
		}
		done := htmlAttr(attrs, "done")
		part := contentPart{tool: name, running: done != "" && done != "true"}
		after := i + rel + 1
		closeRel := indexFold(rest[after:], "</details>")
		if closeRel < 0 {
			parts = append(parts, part)
			break
		}
		parts = append(parts, part)
		rest = rest[after+closeRel+len("</details>"):]
	}
	if len(parts) == 0 {
		return []contentPart{{text: s}}
	}
	return parts
}

func appendText(parts []contentPart, s string) []contentPart {
	if strings.TrimSpace(s) == "" {
		return parts
	}
	return append(parts, contentPart{text: strings.Trim(s, "\n")})
}

func indexFold(s, sub string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(sub))
}

func htmlAttr(attrs, key string) string {
	lower := strings.ToLower(attrs)
	key = strings.ToLower(key)
	from := 0
	for {
		i := strings.Index(lower[from:], key)
		if i < 0 {
			return ""
		}
		i += from
		if i > 0 && isAttrName(lower[i-1]) {
			from = i + len(key)
			continue
		}
		rest := strings.TrimLeft(attrs[i+len(key):], " \t\r\n")
		if !strings.HasPrefix(rest, "=") {
			from = i + len(key)
			continue
		}
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		if rest == "" {
			return ""
		}
		switch rest[0] {
		case '"', '\'':
			q := rest[0]
			end := strings.IndexByte(rest[1:], q)
			if end < 0 {
				return html.UnescapeString(rest[1:])
			}
			return html.UnescapeString(rest[1 : 1+end])
		default:
			end := strings.IndexAny(rest, " \t\r\n")
			if end < 0 {
				return html.UnescapeString(rest)
			}
			return html.UnescapeString(rest[:end])
		}
	}
}

func isAttrName(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == ':'
}
