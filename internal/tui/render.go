package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sasmitai/cursor-tracker/internal/agg"
	"github.com/sasmitai/cursor-tracker/internal/source"
)

const (
	defaultWidth = 88
	minWidth     = 60
)

var tabNames = []string{"Overview", "Models", "Sessions", "History"}

var dash = "—"

// paintCanvas pads each line to the terminal width and fills it with the
// design's dark canvas colour.
func paintCanvas(view string, w int) string {
	bg := lipgloss.NewStyle().Background(colBg)
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		lines[i] = bg.Render(padStyled(l, w))
	}
	return strings.Join(lines, "\n")
}

// lipglossWidth reports the printable width of a styled string.
func lipglossWidth(s string) int { return lipgloss.Width(s) }

// render composes the whole screen: chrome, hero, budget bar, metrics, tabs,
// the active tab's body, and the key hints.
func render(a *App) string {
	w := a.width
	if w < minWidth {
		w = defaultWidth
	}
	inner := w - 2 // body padding

	var b strings.Builder

	if a.warning != "" {
		b.WriteString(" " + stWarn.Render("! "+a.warning) + "\n\n")
	}

	b.WriteString(headerRow(a, inner) + "\n\n")

	p := a.result.Period
	if p == nil {
		if a.warning == "" {
			b.WriteString(centerIn(stMuted.Render("loading usage data…"), inner) + "\n")
		} else {
			b.WriteString(centerIn(stMuted.Render("no usage data available"), inner) + "\n")
		}
	} else {
		ag := agg.ComputeAggregate(a.deps.Now(), p)
		b.WriteString(heroRow(a, ag, inner) + "\n\n")
		b.WriteString(budgetBar(a, ag, inner) + "\n\n")
		b.WriteString(legendRow(a, ag, inner) + "\n\n")
		if !a.compact {
			b.WriteString(rule(inner) + "\n\n")
			b.WriteString(metricsRow(a, ag, inner) + "\n\n")
		}
	}

	b.WriteString(rule(inner) + "\n\n")
	if a.compact {
		// Compact mode is a spend glance: the overview chart and model
		// leaderboard are the first things to go.
		if a.tab != 1 {
			b.WriteString(a.body(inner))
		}
	} else {
		b.WriteString(tabRow(a, inner) + "\n\n")
		b.WriteString(a.body(inner))
	}
	b.WriteString("\n")
	b.WriteString(footer(a, inner))
	return paintCanvas(b.String(), w)
}

func headerRow(a *App, inner int) string {
	word := stWordmark.Render("CURSOR SPEND") + stCaret.Render("▌")
	right := ""
	if p := a.result.Period; p != nil {
		right = stPeriod.Render(monthRangeLabel(p))
	}
	gap := inner - lipglossWidth(word) - lipglossWidth(right)
	if gap < 1 {
		gap = 1
	}
	return word + strings.Repeat(" ", gap) + right
}

// monthRangeLabel renders "NOV 01 — DEC 01 2026" from period bounds.
func monthRangeLabel(p *source.PeriodUsage) string {
	start := time.UnixMilli(p.PeriodStartMs).In(time.Local)
	end := time.UnixMilli(p.PeriodEndMs).In(time.Local)
	return strings.ToUpper(fmt.Sprintf("%s %02d — %s %02d %d",
		start.Format("Jan"), start.Day(), end.Format("Jan"), end.Day(), end.Year()))
}

func heroRow(a *App, ag agg.Aggregate, inner int) string {
	p := a.result.Period

	// Amount block: tall numerals with a trailing "$" like the percentage.
	amountRows := stampColour(heroBlock(fmt.Sprintf("%.2f", float64(p.TotalSpendCents)/100), "$"), stAmount)
	left := strings.Join(append(amountRows,
		stMuted.Render("of "+fmtMoney(float64(p.LimitCents)))), "\n")

	if !ag.HasLimit {
		return left
	}

	pctStyle := stAccentBold
	if ag.SpendFraction >= 1 {
		pctStyle = stBad
	}
	pctRows := stampColour(heroBlock(fmt.Sprintf("%.1f", ag.SpendFraction*100), "%"), pctStyle)

	leftLabel := stLabel.Render(fmtMoney(float64(ag.RemainingCents)) + " LEFT")
	if ag.RemainingCents < 0 {
		leftLabel = stWarn.Render("OVER BUDGET " + fmtMoney(float64(-ag.RemainingCents)))
	}
	right := strings.Join(append(pctRows, leftLabel), "\n")

	gap := inner - maxLineWidth(left) - maxLineWidth(right)
	if gap < 1 {
		gap = 1
	}
	return joinColumns(left, right, gap)
}

// joinColumns lays two multi-line blocks side by side.
func joinColumns(left, right string, gap int) string {
	ls := strings.Split(left, "\n")
	rs := strings.Split(right, "\n")
	n := len(ls)
	if len(rs) > n {
		n = len(rs)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		var l, r string
		if i < len(ls) {
			l = ls[i]
		}
		if i < len(rs) {
			r = rs[i]
		}
		pad := gap - lipglossWidth(l)
		if pad < 0 {
			pad = 0
		}
		out[i] = l + strings.Repeat(" ", pad) + r
	}
	return strings.Join(out, "\n")
}

func maxLineWidth(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		if lw := lipglossWidth(line); lw > w {
			w = lw
		}
	}
	return w
}

func budgetBar(a *App, ag agg.Aggregate, inner int) string {
	cells := inner
	if cells > 60 {
		cells = 60
	}
	spentFrac := ag.SpendFraction
	projFrac := ag.ProjectedFraction
	if !ag.HasLimit {
		spentFrac, projFrac = 0, 0
	}
	if projFrac < spentFrac {
		projFrac = spentFrac
	}
	if ag.HasLimit && spentFrac >= 1 {
		return strings.Repeat(stBad.Render("█"), cells)
	}
	return segmentedBar(spentFrac, projFrac, cells)
}

func legendRow(a *App, ag agg.Aggregate, inner int) string {
	p := a.result.Period
	spent := stLabel.Render("spent ") + stSoft.Render(fmtMoney(float64(p.TotalSpendCents)))
	projected := stLabel.Render("projected ") + stSoft.Render(fmtMoneyOrDash(ag.HasProjection, ag.ProjectedEomSpendCents))
	left := stLabel.Render("left ") + stSoft.Render(fmtMoney(float64(ag.RemainingCents)))

	pace := ""
	if ag.HasLimit && ag.ElapsedFraction > 0 {
		raw := fmt.Sprintf("day %d of %d · %.2f× pace", ag.DayOfPeriod, ag.DaysInPeriod, ag.PaceRatio)
		pace = stMuted.Render(raw)
		if ag.PaceRatio > 1 {
			pace = stBad.Render(raw)
		}
	}
	body := spent + "   " + projected + "   " + left
	gap := inner - lipglossWidth(body) - lipglossWidth(pace)
	if gap < 1 {
		gap = 1
	}
	return body + strings.Repeat(" ", gap) + pace
}

func fmtMoneyOrDash(ok bool, cents float64) string {
	if !ok {
		return dash
	}
	return fmtMoney(cents)
}

func metricsRow(a *App, ag agg.Aggregate, inner int) string {
	burn := metricBlock("BURN RATE",
		fmtMoneyOrDash(ag.BurnRateCentsPerDay > 0, ag.BurnRateCentsPerDay), "/day", stValueBold)

	projSuffix := dash
	if ag.HasLimit && ag.HasProjection {
		projSuffix = fmtPct(ag.ProjectedFraction)
	}
	proj := metricBlock("PROJECTED EOM",
		fmtMoneyOrDash(ag.HasProjection, ag.ProjectedEomSpendCents), projSuffix, stValueBold)

	daysVal := dash
	daysSuffix := ""
	daysStyle := stValueBold
	if ag.HasLimit && ag.HasProjection {
		daysVal = fmt.Sprintf("%.0f", ag.DaysToLimit)
		daysSuffix = "at pace"
		if ag.DaysToLimit < ag.DaysRemaining {
			daysStyle = stBad
		} else {
			daysStyle = stGood
		}
	}
	days := metricBlock("DAYS TO LIMIT", daysVal, daysSuffix, daysStyle)

	elapsed := metricBlock("ELAPSED", fmt.Sprintf("%.1f", ag.DaysElapsed),
		fmt.Sprintf("of %d d", ag.DaysInPeriod), stValueBold)

	return columns(inner, 4, burn, proj, days, elapsed)
}

// metricBlock renders a label above a value/suffix pair.
func metricBlock(label, value, suffix string, vs lipgloss.Style) string {
	top := stLabel.Render(label)
	line := vs.Render(value)
	if suffix != "" {
		line += " " + stMuted.Render(suffix)
	}
	return top + "\n" + line
}

// columns lays n blocks out side by side across the given width, separated
// by a vertical rule.
func columns(inner, n int, blocks ...string) string {
	if len(blocks) == 0 {
		return ""
	}
	sep := " " + stRule.Render("│") + " "
	sepW := lipglossWidth(sep)
	colW := (inner - sepW*(len(blocks)-1)) / len(blocks)
	if colW < 8 {
		colW = 8
	}

	split := make([][]string, len(blocks))
	height := 0
	for i, blk := range blocks {
		split[i] = strings.Split(blk, "\n")
		if len(split[i]) > height {
			height = len(split[i])
		}
	}

	out := make([]string, 0, height)
	for row := 0; row < height; row++ {
		var line strings.Builder
		for i := range blocks {
			cell := ""
			if row < len(split[i]) {
				cell = split[i][row]
			}
			line.WriteString(padStyled(cell, colW))
			if i < len(blocks)-1 {
				line.WriteString(sep)
			}
		}
		out = append(out, line.String())
	}
	return strings.Join(out, "\n")
}

// padLeftStyled left-pads a styled string to n visible columns.
func padLeftStyled(s string, n int) string {
	pad := n - lipglossWidth(s)
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad) + s
}

// padStyled right-pads a styled string to n visible columns.
func padStyled(s string, n int) string {
	pad := n - lipglossWidth(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

// padBlock right-pads every line of a block to width.
func tabRow(a *App, inner int) string {
	var tabs []string
	for i, name := range tabNames {
		key := fmt.Sprintf("%d", i+1)
		if i+1 == a.tab {
			tabs = append(tabs, stTabKeyOn.Render(" "+key+" ")+stTabLabelOn.Render(" "+name+" "))
		} else {
			tabs = append(tabs, stTabKey.Render(" "+key+" ")+stTabLabel.Render(" "+name+" "))
		}
	}
	body := strings.Join(tabs, " ")
	right := ""
	if len(a.result.Daily) > 0 {
		right = stMuted.Render(fmt.Sprintf("%d models · %d rows",
			countCategories(a.result.Daily), len(a.result.Daily)))
	}
	gap := inner - lipglossWidth(body) - lipglossWidth(right)
	if gap < 1 {
		gap = 1
	}
	return body + strings.Repeat(" ", gap) + right
}

func footer(a *App, inner int) string {
	src := a.result.SourceName
	if src == "" {
		src = "none"
	}
	age := dash
	if !a.result.FetchedAt.IsZero() {
		age = humanAge(a.deps.Now().Sub(a.result.FetchedAt).Seconds())
	}
	left := stMuted.Render(fmt.Sprintf("data: %s old via %s", age, src))
	keys := stTabKey.Render(" r ") + stMuted.Render("refresh") + "   " +
		stTabKey.Render(" 1-4 ") + stMuted.Render("tabs") + "   " +
		stTabKey.Render(" c ") + stMuted.Render("compact toggle") + "   " +
		stTabKey.Render(" q ") + stMuted.Render("quit")
	gap := inner - lipglossWidth(left) - lipglossWidth(keys)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + keys
}

func rule(inner int) string {
	return stRule.Render(strings.Repeat("─", inner))
}

func centerIn(s string, inner int) string {
	w := lipglossWidth(s)
	if w >= inner {
		return s
	}
	left := (inner - w) / 2
	return strings.Repeat(" ", left) + s
}

// body renders the active tab.
func (a *App) body(inner int) string {
	switch a.tab {
	case 2:
		return a.modelsBody(inner)
	case 3:
		return a.sessionsBody(inner)
	case 4:
		return a.historyBody(inner)
	default:
		return a.overviewBody(inner)
	}
}

// overviewBody is the chart + top models split from the design.
func (a *App) overviewBody(inner int) string {
	days, labels := a.dailyTotals()
	chartW := inner - 32
	if chartW < 24 {
		chartW = 24
	}
	modelsW := 30

	chart := []string{stSection.Render("DAILY SPEND")}
	note := ""
	if len(days) > 0 {
		var sum float64
		var peak float64
		for _, d := range days {
			sum += d
			if d > peak {
				peak = d
			}
		}
		note = fmt.Sprintf("%d days · avg %s · peak %s",
			len(days), fmtMoney(sum/float64(len(days))), fmtMoney(peak))
	}
	chart[0] = chart[0] + " " + stMuted.Render(note)

	const plotH = 8
	for _, r := range dailyBars(days, plotH, chartW) {
		chart = append(chart, r)
	}
	chart = append(chart, stBaselineRule(chartW))
	if len(labels) > 0 {
		axis := stMuted.Render(labels[0])
		mid := stMuted.Render(labels[len(labels)/2])
		axisW := lipglossWidth(stMuted.Render(labels[0])) + lipglossWidth(stMuted.Render(labels[len(labels)/2])) + lipglossWidth(stMuted.Render(labels[len(labels)-1]))
		g1 := (chartW - axisW) / 2
		if g1 < 1 {
			g1 = 1
		}
		chart = append(chart, axis+strings.Repeat(" ", g1)+mid+
			strings.Repeat(" ", maxInt(1, chartW-axisW-g1))+stMuted.Render(labels[len(labels)-1]))
	}

	top := a.topModels(modelsW)
	leftBlock := strings.Join(chart, "\n")
	return joinColumns(leftBlock, top, chartW+2)
}

func stBaselineRule(width int) string {
	return stRule.Render(strings.Repeat("─", width))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// dailyTotals returns per-day spend across all models, day-ordered, plus
// axis labels for the first, middle, and last day.
func (a *App) dailyTotals() ([]float64, []string) {
	if len(a.result.Daily) == 0 {
		return nil, nil
	}
	byDay := map[string]float64{}
	var order []string
	for _, r := range a.result.Daily {
		key := r.Day.Format("2006-01-02")
		if _, ok := byDay[key]; !ok {
			order = append(order, key)
		}
		byDay[key] += float64(r.SpendCents)
	}
	sort.Strings(order)

	// Span every day of the period up to now, so quiet days render as empty
	// columns instead of collapsing the timeline.
	now := a.deps.Now()
	first := now
	if p := a.result.Period; p != nil {
		first = time.UnixMilli(p.PeriodStartMs).In(time.Local)
	}
	cur := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.Local)
	last := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if tail, err := time.ParseInLocation("2006-01-02", order[len(order)-1], time.Local); err == nil && tail.After(last) {
		last = tail
	}

	var vals []float64
	var labels []string
	for !cur.After(last) {
		vals = append(vals, byDay[cur.Format("2006-01-02")])
		labels = append(labels, cur.Format("Jan 02"))
		cur = cur.AddDate(0, 0, 1)
	}
	return vals, labels
}

// topModels renders the model leaderboard with share bars.
func (a *App) topModels(width int) string {
	totals := map[string]int64{}
	var order []string
	for _, r := range a.result.Daily {
		if _, ok := totals[r.Category]; !ok {
			order = append(order, r.Category)
		}
		totals[r.Category] += r.SpendCents
	}
	sort.Slice(order, func(i, j int) bool { return totals[order[i]] > totals[order[j]] })

	lines := []string{stSection.Render("TOP MODELS")}
	if len(order) == 0 {
		lines = append(lines, stMuted.Render("no model data"))
		return strings.Join(lines, "\n")
	}
	var maxCents int64
	for _, c := range totals {
		if c > maxCents {
			maxCents = c
		}
	}
	const shown = 5
	for i, name := range order {
		if i >= shown {
			break
		}
		cents := totals[name]
		nameStyle := stValue
		if i > 0 {
			nameStyle = stSoft
		}
		label := nameStyle.Render(truncate(name, width-11))
		cost := stValueBold.Render(fmtMoneyCompact(float64(cents)))
		gap := width - lipglossWidth(label) - lipglossWidth(cost)
		if gap < 1 {
			gap = 1
		}
		frac := 0.0
		if maxCents > 0 {
			frac = float64(cents) / float64(maxCents)
		}
		lines = append(lines, label+strings.Repeat(" ", gap)+cost, miniBar(frac, float64(width)))
	}
	return strings.Join(lines, "\n")
}

// modelsBody lists every model with tokens, spend, and a recent-days sparkline.
func (a *App) modelsBody(inner int) string {
	if len(a.result.Daily) == 0 {
		return stMuted.Render("no per-model data") + "\n"
	}
	type total struct {
		cents  int64
		tokens int64
		byDay  map[string]float64
	}
	totals := map[string]*total{}
	var grandCents, grandTokens int64
	for _, r := range a.result.Daily {
		t, ok := totals[r.Category]
		if !ok {
			t = &total{byDay: map[string]float64{}}
			totals[r.Category] = t
		}
		t.cents += r.SpendCents
		t.tokens += r.TotalTokens
		t.byDay[r.Day.Format("2006-01-02")] += float64(r.SpendCents)
		grandCents += r.SpendCents
		grandTokens += r.TotalTokens
	}
	names := make([]string, 0, len(totals))
	for n := range totals {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return totals[names[i]].cents > totals[names[j]].cents })

	var b strings.Builder
	b.WriteString(stSection.Render("BY MODEL") + "  " +
		stMuted.Render(fmt.Sprintf("period %s · %s tokens · %d models",
			fmtMoney(float64(grandCents)), humanTokens(grandTokens), len(names))) + "\n\n")

	nameW := inner - 30
	if nameW < 16 {
		nameW = 16
	}
	header := padStyled(stLabel.Render("MODEL"), nameW) + " " +
		padLeftStyled(stLabel.Render("TOKENS"), 9) + " " +
		padLeftStyled(stLabel.Render("SPEND"), 9) + "  " + stLabel.Render("14-DAY")
	b.WriteString(header + "\n")

	rows := make([]string, 0, len(names))
	for _, n := range names {
		t := totals[n]
		spark := sparkline(a.recentDays(t.byDay, 14))
		rows = append(rows, padStyled(stValue.Render(truncate(n, nameW-1)), nameW)+" "+
			padLeftStyled(stSoft.Render(humanTokens(t.tokens)), 9)+" "+
			padLeftStyled(stValueBold.Render(fmtMoney(float64(t.cents))), 9)+"  "+
			stAccent.Render(spark))
	}
	visible, more := sliceRows(rows, a.offset, a.listHeight())
	for _, row := range visible {
		b.WriteString(row + "\n")
	}
	if more > 0 {
		b.WriteString(stMuted.Render(fmt.Sprintf("  … %d more · j/k to scroll", more)) + "\n")
	}
	return b.String()
}

// recentDays returns the last n days of spend for one model.
func (a *App) recentDays(byDay map[string]float64, n int) []float64 {
	now := a.deps.Now()
	out := make([]float64, 0, n)
	for i := n - 1; i >= 0; i-- {
		out = append(out, byDay[now.AddDate(0, 0, -i).Format("2006-01-02")])
	}
	return out
}

func (a *App) sessionsBody(inner int) string {
	var b strings.Builder
	b.WriteString(stSection.Render("AGENT ACTIVITY") + "\n\n")
	if a.result.SessionsErr != nil {
		b.WriteString(stWarn.Render("unavailable: "+a.result.SessionsErr.Error()) + "\n")
		return b.String()
	}
	if len(a.result.Sessions) == 0 {
		b.WriteString(stMuted.Render("no activity recorded") + "\n")
		return b.String()
	}
	whenW, provW, agentW := 17, 9, 12
	modelW := inner - whenW - provW - agentW - 10
	if modelW < 12 {
		modelW = 12
	}
	b.WriteString(padStyled(stLabel.Render("WHEN"), whenW) + " " +
		padStyled(stLabel.Render("PROVIDER"), provW) + " " +
		padStyled(stLabel.Render("AGENT"), agentW) + " " +
		padStyled(stLabel.Render("MODEL"), modelW) + " " +
		stLabel.Render("EVENT") + "\n")
	rows := make([]string, 0, len(a.result.Sessions))
	for _, s := range a.result.Sessions {
		rows = append(rows, padStyled(stSoft.Render(s.OccurredAt.Local().Format("Jan 02 15:04")), whenW)+" "+
			padStyled(stValue.Render(truncate(s.Provider, provW-1)), provW)+" "+
			padStyled(stMuted.Render(shortAgent(s.AgentID)), agentW)+" "+
			padStyled(stSoft.Render(truncate(modelLabel(s.Model), modelW-1)), modelW)+" "+
			stMuted.Render(s.Kind))
	}
	visible, more := sliceRows(rows, a.offset, a.listHeight())
	for _, row := range visible {
		b.WriteString(row + "\n")
	}
	if more > 0 {
		b.WriteString(stMuted.Render(fmt.Sprintf("  … %d more · j/k to scroll", more)) + "\n")
	}
	return b.String()
}

// modelLabel drops provider bracket parameters so long ids stay readable.
func modelLabel(model string) string {
	if i := strings.IndexByte(model, '['); i > 0 {
		return model[:i]
	}
	return model
}

// shortAgent trims an OpenBot agent id for the activity table.
func shortAgent(id string) string { return truncate(id, 11) }

func (a *App) historyBody(inner int) string {
	var b strings.Builder
	b.WriteString(stSection.Render("SNAPSHOTS") + "\n\n")
	if a.deps.Store == nil {
		b.WriteString(stMuted.Render("history store unavailable") + "\n")
		return b.String()
	}
	periods, err := a.deps.Store.Periods()
	if err != nil {
		b.WriteString(stWarn.Render("error: "+err.Error()) + "\n")
		return b.String()
	}
	if len(periods) == 0 {
		b.WriteString(stMuted.Render("no snapshots stored yet") + "\n")
		return b.String()
	}
	cteW := 10
	b.WriteString(padStyled(stLabel.Render("PERIOD"), cteW) + " " +
		padStyled(stLabel.Render("FETCHED"), 15) + " " +
		padLeftStyled(stLabel.Render("SPEND"), 10) + " " +
		padLeftStyled(stLabel.Render("LIMIT"), 9) + " " + stLabel.Render("SHARE") + "\n")
	rows := make([]string, 0, len(periods))
	for _, p := range periods {
		frac := 0.0
		if p.LimitCents > 0 {
			frac = float64(p.TotalSpendCents) / float64(p.LimitCents)
		}
		barW := float64(inner - cteW - 15 - 10 - 9 - 4)
		if barW < 4 {
			barW = 4
		}
		rows = append(rows, padStyled(stMuted.Render(time.UnixMilli(p.PeriodStartMs).In(time.Local).Format("Jan 02")), cteW)+" "+
			padStyled(stSoft.Render(p.FetchedAt.Local().Format("Jan 02 15:04")), 15)+" "+
			padLeftStyled(stValueBold.Render(fmtMoney(float64(p.TotalSpendCents))), 10)+" "+
			padLeftStyled(stMuted.Render(fmtMoney(float64(p.LimitCents))), 9)+" "+
			miniBar(frac, barW))
	}
	visible, more := sliceRows(rows, a.offset, a.listHeight())
	for _, row := range visible {
		b.WriteString(row + "\n")
	}
	if more > 0 {
		b.WriteString(stMuted.Render(fmt.Sprintf("  … %d more · j/k to scroll", more)) + "\n")
	}
	return b.String()
}

// countCategories counts distinct model categories.
func countCategories(rows []source.DailySpend) int {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[r.Category] = struct{}{}
	}
	return len(seen)
}
