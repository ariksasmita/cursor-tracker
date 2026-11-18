package tui

import "github.com/charmbracelet/lipgloss"

// Palette from the exported UI design: dark canvas, amber accent, two state colours.
var (
	colBg        = lipgloss.Color("#0C1017")
	colBarBg     = lipgloss.Color("#141A23")
	colOutline   = lipgloss.Color("#232C39")
	colRule      = lipgloss.Color("#1A222E")
	colBaseline  = lipgloss.Color("#26303E")
	colFg        = lipgloss.Color("#E8EDF4")
	colFgSoft    = lipgloss.Color("#96A2B4")
	colFgMuted   = lipgloss.Color("#7E8B9E")
	colAccent    = lipgloss.Color("#FFB454")
	colAccentDim = lipgloss.Color("#8A5F22")
	colTrack     = lipgloss.Color("#1B2331")
	colGood      = lipgloss.Color("#5FD08A")
	colBad       = lipgloss.Color("#EE6A5F")
	colOnAccent  = lipgloss.Color("#1A1206")
)

var (
	// Text roles.
	stWordmark   = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	stCaret      = lipgloss.NewStyle().Foreground(colAccent)
	stPeriod     = lipgloss.NewStyle().Foreground(colFgMuted)
	stLabel      = lipgloss.NewStyle().Foreground(colFgMuted)
	stSection    = lipgloss.NewStyle().Foreground(colFgSoft).Bold(true)
	stValue      = lipgloss.NewStyle().Foreground(colFg)
	stValueBold  = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	stAmount     = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	stMuted      = lipgloss.NewStyle().Foreground(colFgMuted)
	stSoft       = lipgloss.NewStyle().Foreground(colFgSoft)
	stAccent     = lipgloss.NewStyle().Foreground(colAccent)
	stAccentBold = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stGood       = lipgloss.NewStyle().Foreground(colGood)
	stBad        = lipgloss.NewStyle().Foreground(colBad)

	// Filled bars and cells.
	stSpentCell     = lipgloss.NewStyle().Foreground(colAccent)
	stProjectedCell = lipgloss.NewStyle().Foreground(colAccentDim)
	stEmptyCell     = lipgloss.NewStyle().Foreground(colTrack)
	stShareFill     = lipgloss.NewStyle().Foreground(colAccent)
	stShareTrack    = lipgloss.NewStyle().Foreground(colTrack)

	// Tabs.
	stTabActive  = lipgloss.NewStyle().Foreground(colOnAccent).Background(colAccent).Bold(true)
	stTabKey     = lipgloss.NewStyle().Foreground(colFgMuted).Bold(true)
	stTabLabel   = lipgloss.NewStyle().Foreground(colFgSoft)
	stTabKeyOn   = lipgloss.NewStyle().Foreground(colOnAccent).Background(colAccent).Bold(true)
	stTabLabelOn = lipgloss.NewStyle().Foreground(colOnAccent).Background(colAccent).Bold(true)
	stRule       = lipgloss.NewStyle().Foreground(colRule)

	// Warning banner.
	stWarn = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
)

// stateColor picks a colour for a value that can be healthy or over budget.
func stateStyle(onPace bool) lipgloss.Style {
	if onPace {
		return stGood
	}
	return stBad
}
