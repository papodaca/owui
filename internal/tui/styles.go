package tui

import "charm.land/lipgloss/v2"

type styles struct {
	title    lipgloss.Style
	dim      lipgloss.Style
	accent   lipgloss.Style
	user     lipgloss.Style
	assist   lipgloss.Style
	err      lipgloss.Style
	on       lipgloss.Style
	off      lipgloss.Style
	disabled lipgloss.Style
	sel      lipgloss.Style
	status   lipgloss.Style
	help     lipgloss.Style
}

func newStyles() styles {
	return styles{
		title:    lipgloss.NewStyle().Bold(true),
		dim:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		accent:   lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		user:     lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
		assist:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
		err:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		on:       lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")),
		off:      lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		disabled: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		sel:      lipgloss.NewStyle().Reverse(true),
		status:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		help:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	}
}
