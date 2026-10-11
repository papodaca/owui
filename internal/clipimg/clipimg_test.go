package clipimg

import (
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
