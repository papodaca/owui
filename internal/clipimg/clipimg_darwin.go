//go:build darwin

package clipimg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func init() {
	darwinClipboard = true
	runPasteboard = execOsascript
}

func execOsascript(script, path string) (string, error) {
	const bin = "/usr/bin/osascript"
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("paste needs osascript")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-e", script, path)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("clipboard timed out")
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}
