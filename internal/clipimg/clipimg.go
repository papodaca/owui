// Package clipimg reads an image from the system clipboard.
// Terminal paste only delivers text. A pasted picture stays on the clipboard,
// and the program has to ask for it, which is what Ctrl+V does here.
package clipimg

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxBytes = 8 << 20

// ErrNone means the clipboard has no image. The caller should stay quiet,
// because Ctrl+V is also pressed when the user meant to paste text.
var ErrNone = errors.New("clipboard has no image")

// ErrTool means neither wl-paste nor xclip is installed.
var ErrTool = errors.New("paste needs wl-paste or xclip")

// Read returns the clipboard image bytes. The MIME type is sniffed from the
// bytes, not from the clipboard label.
func Read() (mime string, data []byte, err error) {
	data, err = readClipboard()
	if err != nil {
		return "", nil, err
	}
	mime = sniff(data)
	if mime == "" {
		return "", nil, errors.New("clipboard image is not a png, jpeg, gif, or webp")
	}
	return mime, data, nil
}

// DataURL is the URL Open WebUI stores on a message file.
func DataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func readClipboard() ([]byte, error) {
	wayland := os.Getenv("WAYLAND_DISPLAY") != ""
	haveWL := look("wl-paste")
	haveX := look("xclip")
	switch {
	case wayland && haveWL:
		return readWL()
	case haveX:
		return readX()
	case haveWL:
		return readWL()
	default:
		return nil, ErrTool
	}
}

func look(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func readWL() ([]byte, error) {
	types, err := output("wl-paste", "--list-types")
	if err != nil {
		return nil, quietClipboard(err)
	}
	mime := pickMIME(string(types))
	if mime == "" {
		return nil, ErrNone
	}
	data, err := output("wl-paste", "-t", mime)
	if err != nil {
		return nil, quietClipboard(err)
	}
	return data, nil
}

func readX() ([]byte, error) {
	types, err := output("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o")
	if err != nil {
		return nil, quietClipboard(err)
	}
	mime := pickMIME(string(types))
	if mime == "" {
		return nil, ErrNone
	}
	data, err := output("xclip", "-selection", "clipboard", "-t", mime, "-o")
	if err != nil {
		return nil, quietClipboard(err)
	}
	return data, nil
}

func quietClipboard(err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return ErrNone
	}
	return err
}

func output(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("clipboard timed out")
	}
	if err != nil {
		return nil, err
	}
	if len(out) > maxBytes {
		return nil, fmt.Errorf("clipboard image is larger than %dMB", maxBytes>>20)
	}
	return out, nil
}

func pickMIME(list string) string {
	have := map[string]bool{}
	for _, line := range strings.Split(list, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		have[strings.ToLower(strings.TrimSpace(line))] = true
	}
	for _, mime := range []string{"image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp"} {
		if have[mime] {
			return mime
		}
	}
	return ""
}

func sniff(b []byte) string {
	if len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png"
	}
	if len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff {
		return "image/jpeg"
	}
	if len(b) >= 6 && (bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))) {
		return "image/gif"
	}
	if len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	return ""
}
