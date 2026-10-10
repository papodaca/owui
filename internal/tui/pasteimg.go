package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/papodaca/owui/internal/api"
	"github.com/papodaca/owui/internal/clipimg"
)

// imageToken is the marker inserted into the composer for one pasted image.
// Backspace and delete remove the whole marker when the cursor touches it.
var imageToken = regexp.MustCompile(`\[ image (\d+) \]`)

// readClipboard is replaced in tests.
var readClipboard = clipimg.Read

type composerImage struct {
	N    int
	File api.Attachment
}

type pasteImageMsg struct {
	mime string
	data []byte
	err  error
}

type imageSpan struct {
	n     int
	start int
	end   int
}

func (m *App) pasteClipboardImage() tea.Cmd {
	return func() tea.Msg {
		mime, data, err := readClipboard()
		return pasteImageMsg{mime: mime, data: data, err: err}
	}
}

func (m *App) applyPastedImage(msg pasteImageMsg) {
	if errors.Is(msg.err, clipimg.ErrNone) {
		return
	}
	if msg.err != nil {
		m.errText = msg.err.Error()
		return
	}
	m.errText = ""
	m.insertImageToken(api.Attachment{
		Type: "image",
		URL:  clipimg.DataURL(msg.mime, msg.data),
		Name: imageFileName(msg.mime),
	})
}

func (m *App) insertImageToken(file api.Attachment) {
	n := 1
	for _, img := range m.images {
		if img.N >= n {
			n = img.N + 1
		}
	}
	tok := fmt.Sprintf("[ image %d ]", n)
	idx := cursorIndex(m.input)
	runes := []rune(m.input.Value())
	before := string(runes[:idx])
	after := string(runes[idx:])
	insert := tok
	if before != "" && !strings.HasSuffix(before, " ") && !strings.HasSuffix(before, "\n") {
		insert = " " + insert
	}
	if after != "" && !strings.HasPrefix(after, " ") && !strings.HasPrefix(after, "\n") {
		insert += " "
	}
	m.input.InsertString(insert)
	m.images = append(m.images, composerImage{N: n, File: file})
}

// removeImageToken deletes the marker under the cursor. backspace is true when
// the key removes the character before the cursor.
func (m *App) removeImageToken(backspace bool) bool {
	idx := cursorIndex(m.input)
	for _, sp := range imageSpans(m.input.Value()) {
		hit := sp.start < idx && idx <= sp.end
		if !backspace {
			hit = sp.start <= idx && idx < sp.end
		}
		if !hit {
			continue
		}
		runes := []rune(m.input.Value())
		before := string(runes[:sp.start])
		after := string(runes[sp.end:])
		switch {
		case strings.HasSuffix(before, " ") && strings.HasPrefix(after, " "):
			after = strings.TrimPrefix(after, " ")
		case after == "":
			before = strings.TrimSuffix(before, " ")
		case before == "":
			after = strings.TrimPrefix(after, " ")
		}
		m.input.SetValue(before + after)
		placeCursor(&m.input, len([]rune(before)))
		m.dropImage(sp.n)
		return true
	}
	return false
}

func (m *App) dropImage(n int) {
	kept := make([]composerImage, 0, len(m.images))
	for _, img := range m.images {
		if img.N != n {
			kept = append(kept, img)
		}
	}
	m.images = kept
}

// dropMissingImages forgets an image once its marker is no longer in the composer.
func (m *App) dropMissingImages() {
	if len(m.images) == 0 {
		return
	}
	present := map[int]bool{}
	for _, sp := range imageSpans(m.input.Value()) {
		present[sp.n] = true
	}
	kept := make([]composerImage, 0, len(m.images))
	for _, img := range m.images {
		if present[img.N] {
			kept = append(kept, img)
		}
	}
	m.images = kept
}

func (m *App) filesInPrompt(text string) []api.Attachment {
	byN := map[int]api.Attachment{}
	for _, img := range m.images {
		byN[img.N] = img.File
	}
	var out []api.Attachment
	seen := map[int]bool{}
	for _, sp := range imageSpans(text) {
		f, ok := byN[sp.n]
		if !ok || seen[sp.n] {
			continue
		}
		seen[sp.n] = true
		out = append(out, f)
	}
	return out
}

// promptWithoutImages is the text actually sent. The [ image N ] marker stays
// in the composer so the cursor can delete one image, and is removed here.
func promptWithoutImages(text string) string {
	text = imageToken.ReplaceAllString(text, "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func imageSpans(s string) []imageSpan {
	var out []imageSpan
	for _, loc := range imageToken.FindAllStringSubmatchIndex(s, -1) {
		n, err := strconv.Atoi(s[loc[2]:loc[3]])
		if err != nil {
			continue
		}
		out = append(out, imageSpan{
			n:     n,
			start: utf8.RuneCountInString(s[:loc[0]]),
			end:   utf8.RuneCountInString(s[:loc[1]]),
		})
	}
	return out
}

func cursorIndex(ta textarea.Model) int {
	lines := strings.Split(ta.Value(), "\n")
	row := ta.Line()
	col := ta.Column()
	n := 0
	for i := 0; i < row && i < len(lines); i++ {
		n += utf8.RuneCountInString(lines[i]) + 1
	}
	if row >= 0 && row < len(lines) {
		line := []rune(lines[row])
		if col > len(line) {
			col = len(line)
		}
		if col < 0 {
			col = 0
		}
		n += col
	}
	return n
}

func placeCursor(ta *textarea.Model, runeIndex int) {
	focused := ta.Focused()
	ta.Focus()
	ta.MoveToBegin()
	right := tea.KeyPressMsg{Code: tea.KeyRight}
	for i := 0; i < runeIndex; i++ {
		next, _ := ta.Update(right)
		*ta = next
	}
	if !focused {
		ta.Blur()
	}
}

func imageFileName(mime string) string {
	switch mime {
	case "image/jpeg":
		return "image.jpg"
	case "image/gif":
		return "image.gif"
	case "image/webp":
		return "image.webp"
	default:
		return "image.png"
	}
}

func imageLabel(n int) string {
	if n == 1 {
		return "1 image"
	}
	return fmt.Sprintf("%d images", n)
}

func countImages(files []api.Attachment) int {
	n := 0
	for _, f := range files {
		if f.Type == "image" {
			n++
		}
	}
	return n
}
