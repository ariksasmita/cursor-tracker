package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// fmtMoney renders US cents as $X.XX.
func fmtMoney(cents float64) string {
	return fmt.Sprintf("$%.2f", cents/100)
}

// fmtMoneyCompact is fmtMoney for large values: $1.2K, $1.4M.
func fmtMoneyCompact(cents float64) string {
	d := cents / 100
	switch {
	case math.Abs(d) >= 1_000_000:
		return fmt.Sprintf("$%.1fM", d/1e6)
	case math.Abs(d) >= 10_000:
		return fmt.Sprintf("$%.1fK", d/1e3)
	default:
		return fmt.Sprintf("$%.2f", d)
	}
}

// fmtPct renders a fraction as a percentage with one decimal.
func fmtPct(fraction float64) string {
	return fmt.Sprintf("%.1f%%", fraction*100)
}

// humanTokens renders a token count compactly.
func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// truncate shortens s to n runes with an ellipsis.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// padVisible right-pads s with spaces to n visible runes.
func padVisible(s string, n int) string {
	l := len([]rune(s))
	if l >= n {
		return s
	}
	return s + strings.Repeat(" ", n-l)
}

// padLeft left-pads s with spaces to n visible runes.
func padLeft(s string, n int) string {
	l := len([]rune(s))
	if l >= n {
		return s
	}
	return strings.Repeat(" ", n-l) + s
}

// miniBar renders a proportional fill of the given width using block glyphs.
// Always fills exactly width cells so columns stay aligned.
func miniBar(fraction, width float64) string {
	w := int(math.Round(width))
	if w < 1 {
		w = 1
	}
	f := fraction
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	filled := int(math.Round(f * float64(w)))
	if filled > w {
		filled = w
	}
	return stShareFill.Render(strings.Repeat("█", filled)) +
		stShareTrack.Render(strings.Repeat("░", w-filled))
}

// segmentedBar renders the budget track as discrete cells: spent (accent),
// projected headroom (dim accent), untouched (track). Cell count adapts to
// the available width.
func segmentedBar(spentFrac, projectedFrac float64, cells int) string {
	if cells < 4 {
		cells = 4
	}
	spentCells := int(math.Round(clamp01(spentFrac) * float64(cells)))
	projCells := int(math.Round(clamp01(projectedFrac)*float64(cells))) - spentCells

	var b strings.Builder
	for i := 0; i < cells; i++ {
		switch {
		case i < spentCells:
			b.WriteString(stSpentCell.Render("█"))
		case i < spentCells+projCells:
			b.WriteString(stProjectedCell.Render("▓"))
		default:
			b.WriteString(stEmptyCell.Render("░"))
		}
	}
	return b.String()
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// dailyBars renders a vertical bar chart of daily spend scaled to the
// tallest day. Bars widen to two cells with a one-cell gap when the plot is
// wide enough, which reads closer to the design's block columns.
func dailyBars(days []float64, height, width int) []string {
	if height < 1 {
		height = 1
	}
	if len(days) == 0 {
		return nil
	}
	var max float64
	for _, d := range days {
		if d > max {
			max = d
		}
	}

	barW, gap := 1, 0
	if width >= len(days)*3 {
		barW, gap = 2, 1
	}
	rows := make([]string, height)
	for r := 0; r < height; r++ {
		var b strings.Builder
		for i, d := range days {
			h := 0
			if max > 0 {
				h = int(math.Ceil(d / max * float64(height)))
			}
			filled := height-r <= h
			// Zero-spend days keep a faint floor so the timeline stays readable.
			glyph := strings.Repeat(" ", barW)
			switch {
			case filled:
				glyph = stAccent.Render(strings.Repeat("█", barW))
			case r == height-1 && d == 0:
				glyph = stEmptyCell.Render(strings.Repeat("▁", barW))
			}
			b.WriteString(glyph)
			if gap > 0 && i < len(days)-1 {
				b.WriteString(strings.Repeat(" ", gap))
			}
		}
		rows[r] = b.String()
	}
	return rows
}

// sparkline renders recent values as block glyphs, scaled to their own max.
func sparkline(vals []float64) string {
	blocks := []rune("▁▂▃▄▅▆▇█")
	var max float64
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range vals {
		if max <= 0 {
			b.WriteString(" ")
			continue
		}
		idx := int(math.Round(float64(len(blocks)-1) * clamp01(v/max)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteString(string(blocks[idx]))
	}
	return b.String()
}

// sliceRows returns the visible window of rows for a scroll offset and the
// number of rows still below it.
func sliceRows(rows []string, offset, limit int) ([]string, int) {
	if limit < 1 {
		limit = 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(rows) {
		return nil, 0
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], len(rows) - end
}

// humanAge renders an elapsed duration compactly.
func humanAge(d float64) string { // d in seconds
	if d < 0 {
		d = 0
	}
	switch {
	case d < 60:
		return fmt.Sprintf("%.0fs", d)
	case d < 3600:
		return fmt.Sprintf("%.0fm", d/60)
	case d < 86400:
		return fmt.Sprintf("%.1fh", d/3600)
	default:
		return fmt.Sprintf("%.1fd", d/86400)
	}
}

// bigFont is a 3-wide x 6-tall pixel font drawn as three text rows using
// half blocks. Swapping fonts later only touches this table.
var bigFont = map[rune][6][3]int{
	'0': {{1, 1, 1}, {1, 0, 1}, {1, 0, 1}, {1, 0, 1}, {1, 0, 1}, {1, 1, 1}},
	'1': {{0, 1, 0}, {0, 1, 0}, {0, 1, 0}, {0, 1, 0}, {0, 1, 0}, {1, 1, 1}},
	'2': {{1, 1, 1}, {0, 0, 1}, {0, 0, 1}, {1, 1, 1}, {1, 0, 0}, {1, 1, 1}},
	'3': {{1, 1, 1}, {0, 0, 1}, {0, 1, 1}, {0, 0, 1}, {0, 0, 1}, {1, 1, 1}},
	'4': {{1, 0, 1}, {1, 0, 1}, {1, 1, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
	'5': {{1, 1, 1}, {1, 0, 0}, {1, 1, 1}, {0, 0, 1}, {0, 0, 1}, {1, 1, 1}},
	'6': {{1, 1, 1}, {1, 0, 0}, {1, 1, 1}, {1, 0, 1}, {1, 0, 1}, {1, 1, 1}},
	'7': {{1, 1, 1}, {0, 0, 1}, {0, 0, 1}, {0, 1, 0}, {0, 1, 0}, {0, 1, 0}},
	'8': {{1, 1, 1}, {1, 0, 1}, {1, 1, 1}, {1, 0, 1}, {1, 0, 1}, {1, 1, 1}},
	'9': {{1, 1, 1}, {1, 0, 1}, {1, 1, 1}, {0, 0, 1}, {0, 0, 1}, {1, 1, 1}},
	'.': {{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 1, 0}},
}

// bigDigits renders s as three text rows of tall block numerals. Characters
// with no glyph (currency signs, percent) are skipped — callers place them
// at normal scale so baselines stay clean.
func bigDigits(s string) []string {
	rows := make([]strings.Builder, 3)
	for _, ch := range s {
		// A period is one narrow column at the baseline; a full glyph cell
		// would leave it floating with gaps either side.
		if ch == '.' {
			rows[0].WriteString("  ")
			rows[1].WriteString("  ")
			rows[2].WriteString("▄ ")
			continue
		}
		g, ok := bigFont[ch]
		if !ok {
			continue
		}
		for pair := 0; pair < 3; pair++ {
			for c := 0; c < 3; c++ {
				top, bot := g[pair*2][c], g[pair*2+1][c]
				switch {
				case top == 1 && bot == 1:
					rows[pair].WriteRune('█')
				case top == 1:
					rows[pair].WriteRune('▀')
				case bot == 1:
					rows[pair].WriteRune('▄')
				default:
					rows[pair].WriteRune(' ')
				}
			}
			rows[pair].WriteString(" ")
		}
	}
	out := make([]string, 3)
	for i := range rows {
		out[i] = strings.TrimRight(rows[i].String(), " ")
	}
	return out
}

// heroBlock renders digits as three tall rows with symbol appended to the
// end of the middle row, so currency signs and percent marks both trail the
// number. The returned block is plain text with equal-width rows.
func heroBlock(digits, symbol string) []string {
	rows := bigDigits(digits)
	if len(rows) == 0 {
		return rows
	}
	mid := len(rows) / 2
	rows[mid] = strings.TrimRight(rows[mid], " ") + symbol
	return trimCommonIndent(padRows(rows))
}

// trimCommonIndent removes the blank left columns a glyph set carries as side
// bearing, so hero numerals line up with the text above and below them.
func trimCommonIndent(rows []string) []string {
	if len(rows) == 0 {
		return rows
	}
	min := -1
	for _, r := range rows {
		n := len([]rune(r)) - len([]rune(strings.TrimLeft(r, " ")))
		if min < 0 || n < min {
			min = n
		}
	}
	if min <= 0 {
		return rows
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = string([]rune(r)[min:])
	}
	return out
}

// rightAlignBlock pads every line on the left so the block shares one right
// edge, which is what keeps a percentage and its caption aligned under it.
func rightAlignBlock(block string) string {
	lines := strings.Split(block, "\n")
	w := 0
	for _, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			w = lw
		}
	}
	for i, l := range lines {
		if pad := w - lipgloss.Width(l); pad > 0 {
			lines[i] = strings.Repeat(" ", pad) + l
		}
	}
	return strings.Join(lines, "\n")
}

// padRows makes every row of a glyph block the same width so blocks stack
// into a clean rectangle.
func padRows(rows []string) []string {
	w := 0
	for _, r := range rows {
		if l := len([]rune(r)); l > w {
			w = l
		}
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r + strings.Repeat(" ", w-len([]rune(r)))
	}
	return out
}

// stampColour paints every row of a glyph block with one colour.
func stampColour(rows []string, style lipgloss.Style) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = style.Render(r)
	}
	return out
}
