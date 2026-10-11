//go:build !darwin

package clipimg

// The osascript runner is compiled only in clipimg_darwin.go.
// On this OS, readClipboard keeps the wl-paste / xclip switch, and tests
// that cover the pasteboard path assign runPasteboard themselves.
