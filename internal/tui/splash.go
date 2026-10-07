package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

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
	host := m.styles.dim.Render(m.cfg.URL)
	spin := splashFrames[m.frame%len(splashFrames)]
	status := m.styles.accent.Render(spin + "  connecting")
	if m.quitArmed {
		status = m.styles.err.Render(quitHint)
	}
	inner := lipgloss.JoinVertical(lipgloss.Center, mark, "", name, host, "", status)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(1, 4).
		Render(inner)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
