package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/usr1/owui/internal/api"
	"github.com/usr1/owui/internal/chattree"
)

// doneChime marks a finished reply. Inside Herdr the toast title is the chat
// name and the body is the start of the reply, using the done sound from the
// user's sound pack. Anywhere else it is the terminal bell.
func doneChime(title, reply string) tea.Cmd {
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
