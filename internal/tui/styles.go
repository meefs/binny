package tui

import "github.com/charmbracelet/lipgloss"

// BINNY's palette — a small, terminal-safe set. Kept in one place so the
// header, transcript roles, and errors read as one system.
var (
	colAccent = lipgloss.Color("220") // BINNY yellow (brand: black + yellow)
	colDim    = lipgloss.Color("240")
	colUser   = lipgloss.Color("111")
	colBinny  = lipgloss.Color("220")
	colTool   = lipgloss.Color("143")
	colErr    = lipgloss.Color("203")
	colOK     = lipgloss.Color("78")
	colWarn   = lipgloss.Color("214")

	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleChip   = lipgloss.NewStyle().Foreground(colDim)
	styleChipHi = lipgloss.NewStyle().Foreground(colAccent).Bold(true)

	styleUser  = lipgloss.NewStyle().Foreground(colUser).Bold(true)
	styleBinny = lipgloss.NewStyle().Foreground(colBinny).Bold(true)
	styleTool  = lipgloss.NewStyle().Foreground(colTool)
	styleErr   = lipgloss.NewStyle().Foreground(colErr)
	styleOK    = lipgloss.NewStyle().Foreground(colOK)
	styleWarn  = lipgloss.NewStyle().Foreground(colWarn)
	styleDim   = lipgloss.NewStyle().Foreground(colDim)
)
