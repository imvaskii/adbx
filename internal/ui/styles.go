package ui

import "github.com/charmbracelet/lipgloss"

// styles.go — all Lipgloss style declarations for the ui package.
// Centralised here so visual changes require editing exactly one file.

var (
	// General / shared
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

	// Device list
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	pairingBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	connectBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("78")).Bold(true)
	emptyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)

	// Awaiting pairing screen
	stepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	stepNumStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	watchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	boldStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))

	// Pairing input screen
	inputLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	inputBoxStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("69")).
			Padding(0, 1)
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

	// Result screen
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("78")).Bold(true)
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	msgStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)
