package clipimg

import "testing"

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
