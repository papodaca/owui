package tui

import (
	"runtime/debug"
	"strings"

	"charm.land/lipgloss/v2"
)

// release is copied from main.version. Link the tag with:
//
//	go build -ldflags "-X main.version=v1.0.0"
//
// An empty value falls through to the module version recorded by go install.
var release string

// SetRelease stores the tag linked into main.version.
func SetRelease(tag string) {
	release = tag
}

// Block letters for OWUI. Kept narrow so it still fits in an 80-column terminal.
const wordmark = `
 ██████╗ ██╗    ██╗██╗   ██╗██╗
██╔═══██╗██║    ██║██║   ██║██║
██║   ██║██║ █╗ ██║██║   ██║██║
██║   ██║██║███╗██║██║   ██║██║
╚██████╔╝╚███╔███╔╝╚██████╔╝██║
 ╚═════╝  ╚══╝╚══╝  ╚═════╝ ╚═╝`

var splashFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m App) splashView() string {
	w, h := m.width, m.height
	if w < 1 {
		w = 80
	}
	if h < 1 {
		h = 24
	}
	mark := m.styles.accent.Render(strings.Trim(wordmark, "\n"))
	name := m.styles.title.Render("open webui")
	tag := m.styles.dim.Render(releaseTag())
	host := m.styles.dim.Render(m.cfg.URL)
	spin := splashFrames[m.frame%len(splashFrames)]
	status := m.styles.accent.Render(spin + "  connecting")
	if m.quitArmed {
		status = m.styles.err.Render(quitHint)
	}
	inner := lipgloss.JoinVertical(lipgloss.Center, mark, "", name, tag, host, "", status)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(1, 4).
		Render(inner)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func releaseTag() string {
	if release != "" {
		return release
	}
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
