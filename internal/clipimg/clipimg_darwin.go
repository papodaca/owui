//go:build darwin

package clipimg

import (
	"context"
	"errors"
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-e", script, path)
	out, err := cmd.Output()
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("paste needs osascript")
	}
	if ctx.Err() != nil {
		return "", fmt.Errorf("clipboard timed out")
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}
