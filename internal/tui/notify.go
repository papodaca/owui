package tui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/papodaca/owui/internal/api"
	"github.com/papodaca/owui/internal/chattree"
)

// doneChime marks a finished reply. Inside Herdr the toast title is the chat
// name and the body is the start of the reply, using the done sound from the
// user's sound pack. Anywhere else it is the terminal bell.
func doneChime(title, reply string, termFocused bool) tea.Cmd {
	if !shouldChime(termFocused) {
		return nil
	}
	if !inHerdr() {
		return tea.Printf("\a")
	}
	args := herdrNotifyArgs(title, reply)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "herdr", args...)
		if err := cmd.Run(); err != nil {
			return bellMsg{}
		}
		return nil
	}
}

// The title has to come before the flags. This Herdr build treats a trailing
// title as an unknown option, so --sound never runs and no toast is shown.
func herdrNotifyArgs(title, reply string) []string {
	args := []string{"notification", "show", chimeTitle(title), "--sound", "done"}
	if body := oneLine(reply, 140); body != "" {
		args = append(args, "--body", body)
	}
	return args
}

type bellMsg struct{}

// shouldChime is false while this terminal window is focused. A blur from
// the terminal (DEC 1004, which Herdr forwards) turns it back on. Inside
// Herdr, another focused pane also counts as away, even if the Herdr window
// itself still has focus. If the pane query fails, the window focus stands.
func shouldChime(termFocused bool) bool {
	if !termFocused {
		return true
	}
	if inHerdr() && !herdrPaneFocused() {
		return true
	}
	return false
}

func herdrPaneFocused() bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "herdr", "pane", "current").Output()
	if err != nil {
		return true
	}
	var wrap struct {
		Result struct {
			Pane struct {
				Focused bool `json:"focused"`
			} `json:"pane"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &wrap) != nil {
		return true
	}
	return wrap.Result.Pane.Focused
}

func inHerdr() bool {
	return os.Getenv("HERDR_ENV") != "" || os.Getenv("HERDR_PANE_ID") != ""
}

func chimeTitle(title string) string {
	title = oneLine(title, 80)
	if title == "" || title == "New Chat" || title == "untitled" {
		return "Reply ready"
	}
	return title
}

func replySnippet(h api.History) string {
	chain := chattree.Chain(h)
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].Role != "assistant" {
			continue
		}
		if s := oneLine(chain[i].Content.String(), 140); s != "" && s != "…" {
			return s
		}
	}
	return ""
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 2 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
