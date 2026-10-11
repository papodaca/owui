package clipimg

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestPickMIME(t *testing.T) {
	list := "text/plain\ntext/plain;charset=utf-8\nimage/jpeg\nimage/png\n"
	if got := pickMIME(list); got != "image/png" {
		t.Fatalf("mime = %q", got)
	}
	if got := pickMIME("text/plain\n"); got != "" {
		t.Fatalf("mime = %q", got)
	}
}

func TestSniffAndDataURL(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00}
	if sniff(png) != "image/png" {
		t.Fatal(sniff(png))
	}
	url := DataURL("image/png", png)
	if url != "data:image/png;base64,iVBORw0KGgoA" {
		t.Fatalf("url = %s", url)
	}
	if sniff([]byte("hello")) != "" {
		t.Fatal("text sniffed as an image")
	}
}

func TestPasteboardMIME(t *testing.T) {
	tests := []struct {
		token string
		want  string
	}{
		{"public.png", "image/png"},
		{"public.jpeg", "image/jpeg"},
		{"com.compuserve.gif", "image/gif"},
		{"org.webmproject.webp", "image/webp"},
		{"image/png", "image/png"},
		{"image/jpeg", "image/jpeg"},
		{"image/gif", "image/gif"},
		{"image/webp", "image/webp"},
		{"none", ""},
		{"", ""},
		{"public.tiff", ""},
		{"public.heic", ""},
	}
	for _, tt := range tests {
		if got := pasteboardMIME(tt.token); got != tt.want {
			t.Fatalf("pasteboardMIME(%q) = %q, want %q", tt.token, got, tt.want)
		}
	}
}

func TestPasteboardScript(t *testing.T) {
	want := []string{
		"public.png",
		"public.jpeg",
		"com.compuserve.gif",
		"org.webmproject.webp",
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/webp",
	}
	at := 0
	for _, name := range want {
		i := strings.Index(pasteboardScript[at:], name)
		if i < 0 {
			t.Fatalf("script missing %q", name)
		}
		at += i + len(name)
	}
	if !strings.Contains(pasteboardScript, "argv") {
		t.Fatal("script does not take the temp path as an argument")
	}
	if strings.Contains(pasteboardScript, "«class PNGf»") {
		t.Fatal("script reads the legacy PNGf class")
	}
}

func requireTempGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file remains: %v", err)
	}
}

func stubPasteboard(t *testing.T, run func(script, path string) (string, error)) {
	t.Helper()
	prevDarwin := darwinClipboard
	prevRun := runPasteboard
	darwinClipboard = true
	runPasteboard = run
	t.Cleanup(func() {
		darwinClipboard = prevDarwin
		runPasteboard = prevRun
	})
}

func TestDarwinNone(t *testing.T) {
	var path string
	stubPasteboard(t, func(script, dest string) (string, error) {
		if script != pasteboardScript {
			t.Fatal("runner was not given the pasteboard script")
		}
		path = dest
		return "none\n", nil
	})
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	_, _, err := Read()
	if !errors.Is(err, ErrNone) {
		t.Fatalf("err = %v", err)
	}
	if path == "" {
		t.Fatal("runner was not called")
	}
	requireTempGone(t, path)
}

func TestDarwinPNG(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00}
	var path string
	stubPasteboard(t, func(script, dest string) (string, error) {
		path = dest
		info, err := os.Stat(dest)
		if err != nil {
			return "", err
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o", info.Mode().Perm())
		}
		if err := os.WriteFile(dest, png, 0o600); err != nil {
			return "", err
		}
		return "public.png", nil
	})
	mime, data, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" {
		t.Fatalf("mime = %q", mime)
	}
	if string(data) != string(png) {
		t.Fatalf("data = %v", data)
	}
	requireTempGone(t, path)
}

func TestDarwinListedTypeNoBytes(t *testing.T) {
	stubPasteboard(t, func(script, dest string) (string, error) {
		return "public.png", nil
	})
	_, _, err := Read()
	if err == nil || errors.Is(err, ErrNone) || err.Error() == "clipboard image is not a png, jpeg, gif, or webp" {
		t.Fatalf("err = %v", err)
	}
}

func TestDarwinTooLarge(t *testing.T) {
	stubPasteboard(t, func(script, dest string) (string, error) {
		if err := os.WriteFile(dest, make([]byte, maxBytes+1), 0o600); err != nil {
			return "", err
		}
		return "public.png", nil
	})
	_, _, err := Read()
	if err == nil || err.Error() != "clipboard image is larger than 8MB" {
		t.Fatalf("err = %v", err)
	}
}

func TestDarwinMissingOsascript(t *testing.T) {
	stubPasteboard(t, func(script, dest string) (string, error) {
		return "", errors.New("paste needs osascript")
	})
	_, _, err := Read()
	if err == nil || errors.Is(err, ErrTool) || err.Error() == ErrTool.Error() {
		t.Fatalf("err = %v", err)
	}
}

func TestDarwinUnsetRunner(t *testing.T) {
	stubPasteboard(t, nil)
	_, _, err := Read()
	if err == nil || errors.Is(err, ErrTool) || err.Error() != "paste needs osascript" {
		t.Fatalf("err = %v", err)
	}
}

func TestDarwinTempRemovedOnFailure(t *testing.T) {
	var path string
	stubPasteboard(t, func(script, dest string) (string, error) {
		path = dest
		return "", errors.New("osascript failed")
	})
	_, _, err := Read()
	if err == nil || errors.Is(err, ErrNone) {
		t.Fatalf("err = %v", err)
	}
	requireTempGone(t, path)
}
