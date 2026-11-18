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

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	tabActive    = lipgloss.NewStyle().Bold(true).Reverse(true)
	tabIdle      = lipgloss.NewStyle().Faint(true)
	warningStyle = lipgloss.NewStyle().Bold(true)
	dash         = "—"
)

var tabNames = []string{"Overview", "Models", "Sessions", "History"}

func money(cents float64) string {
	return fmt.Sprintf("$%.2f", cents/100)
}

func render(a *App) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" Cursor Spend "))
	b.WriteString(periodLabel(a))
	b.WriteString("\n\n")

	if a.warning != "" {
		b.WriteString(warningStyle.Render(a.warning))
		b.WriteString("\n\n")
	}

	if a.result.Period != nil {
		b.WriteString(gaugeLine(a))
		b.WriteString("\n")
		b.WriteString(metricsLine(a))
		b.WriteString("\n")
	} else if a.warning == "" {
		b.WriteString("loading usage data…\n")
	} else {
		b.WriteString("no usage data available\n")
	}

	b.WriteString("\n")
	b.WriteString(tabBar(a.tab))
	b.WriteString("\n\n")

	switch a.tab {
	case 2:
		b.WriteString(modelsTab(a))
	case 3:
		b.WriteString(sessionsTab(a))
	case 4:
		b.WriteString(historyTab(a))
	default:
		b.WriteString(overviewTab(a))
	}

	b.WriteString("\n")
	b.WriteString(footer(a))
	return b.String()
}

func periodLabel(a *App) string {
	p := a.result.Period
	if p == nil {
		return ""
	}
	start := time.UnixMilli(p.PeriodStartMs).Format("Jan 2")
	end := time.UnixMilli(p.PeriodEndMs).Format("Jan 2")
	return fmt.Sprintf("── %s–%s %s", start, end, time.UnixMilli(p.PeriodEndMs).Format("2006"))
}

func gaugeLine(a *App) string {
	p := a.result.Period
	if p.LimitCents <= 0 {
		return fmt.Sprintf("%s spent (no limit set)", money(float64(p.TotalSpendCents)))
	}
	pct := float64(p.TotalSpendCents) / float64(p.LimitCents)
	if pct < 0 {
		pct = 0
	}
	const width = 32
	filled := int(pct * width)
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("%s / %s %s %5.1f%%",
		money(float64(p.TotalSpendCents)), money(float64(p.LimitCents)), bar, pct*100)
}

func metricsLine(a *App) string {
	ag := agg.ComputeAggregate(a.deps.Now(), a.result.Period)
	if !ag.HasProjection {
		return fmt.Sprintf("Burn rate: %s · Projected EOM: %s · Days to limit: %s",
			dash, dash, dash)
	}
	daysToLimit := dash
	if ag.HasLimit {
		daysToLimit = fmt.Sprintf("%.0f", ag.DaysToLimit)
	}
	return fmt.Sprintf("Burn rate: %s/day · Projected EOM: %s · Days to limit: %s",
		money(ag.BurnRateCentsPerDay), money(ag.ProjectedEomSpendCents), daysToLimit)
}

func tabBar(active int) string {
	parts := make([]string, 0, len(tabNames))
	for i, name := range tabNames {
		label := fmt.Sprintf("[%d] %s", i+1, name)
		if i+1 == active {
			parts = append(parts, tabActive.Render(label))
		} else {
			parts = append(parts, tabIdle.Render(label))
		}
	}
	return strings.Join(parts, "  ")
}

func overviewTab(a *App) string {
	if a.result.Period == nil {
		return ""
	}
	ag := agg.ComputeAggregate(a.deps.Now(), a.result.Period)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Elapsed: %.1f days · Remaining: %.1f days\n", ag.DaysElapsed, ag.DaysRemaining))
	if len(a.result.Daily) > 0 {
		b.WriteString(fmt.Sprintf("Tracked models today: %d · rows: %d\n",
			countCategories(a.result.Daily), len(a.result.Daily)))
	}
	return b.String()
}

func countCategories(rows []source.DailySpend) int {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[r.Category] = struct{}{}
	}
	return len(seen)
}

func modelsTab(a *App) string {
	if len(a.result.Daily) == 0 {
		return "no per-model data\n"
	}
	type total struct {
		cents  int64
		tokens int64
		byDay  map[string]int64
	}
	totals := map[string]*total{}
	var grandCents, grandTokens int64
	for _, r := range a.result.Daily {
		t, ok := totals[r.Category]
		if !ok {
			t = &total{byDay: map[string]int64{}}
			totals[r.Category] = t
		}
		t.cents += r.SpendCents
		t.tokens += r.TotalTokens
		t.byDay[r.Day.Format("2006-01-02")] += r.SpendCents
		grandCents += r.SpendCents
		grandTokens += r.TotalTokens
	}
	names := make([]string, 0, len(totals))
	for n := range totals {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return totals[names[i]].cents > totals[names[j]].cents })

	var b strings.Builder
	b.WriteString(fmt.Sprintf(" Models — period %s · %s tokens\n\n", money(float64(grandCents)), humanTokens(grandTokens)))
	b.WriteString(fmt.Sprintf(" %-34s %10s %10s  %s\n", "CATEGORY", "TOKENS", "SPEND", "14-DAY"))
	for _, n := range names {
		t := totals[n]
		b.WriteString(fmt.Sprintf(" %-34s %10s %10s  %s\n",
			truncate(n, 34), humanTokens(t.tokens), money(float64(t.cents)), sparkline(t.byDay, a.deps.Now())))
	}
	return b.String()
}

func sparkline(byDay map[string]int64, now time.Time) string {
	const days = 14
	const blocks = "▁▂▃▄▅▆▇█"
	vals := make([]int64, 0, days)
	var max int64
	for i := days - 1; i >= 0; i-- {
		key := now.AddDate(0, 0, -i).Format("2006-01-02")
		v := byDay[key]
		vals = append(vals, v)
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return strings.Repeat(" ", days)
	}
	var sb strings.Builder
	for _, v := range vals {
		idx := int(float64(len(blocks)-1) * float64(v) / float64(max))
		sb.WriteByte(blocks[idx])
	}
	return sb.String()
}

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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func sessionsTab(a *App) string {
	var b strings.Builder
	b.WriteString(" Sessions\n\n")
	if a.result.SessionsErr != nil {
		b.WriteString(fmt.Sprintf(" unavailable: %v\n", a.result.SessionsErr))
		return b.String()
	}
	if len(a.result.Sessions) == 0 {
		b.WriteString(" no activity recorded\n")
		return b.String()
	}
	b.WriteString(fmt.Sprintf(" %-19s %-10s %-12s %-28s %s\n", "WHEN", "PROVIDER", "AGENT", "MODEL", "KIND"))
	for _, s := range a.result.Sessions {
		b.WriteString(fmt.Sprintf(" %-19s %-10s %-12s %-28s %s\n",
			s.OccurredAt.Local().Format("2006-01-02 15:04"),
			truncate(s.Provider, 10), truncate(s.AgentID, 12),
			truncate(s.Model, 28), s.Kind))
	}
	return b.String()
}

func historyTab(a *App) string {
	var b strings.Builder
	b.WriteString(" History\n\n")
	if a.deps.Store == nil {
		b.WriteString(" history store unavailable\n")
		return b.String()
	}
	periods, err := a.deps.Store.Periods()
	if err != nil {
		b.WriteString(fmt.Sprintf(" error: %v\n", err))
		return b.String()
	}
	if len(periods) == 0 {
		b.WriteString(" no snapshots stored yet\n")
		return b.String()
	}
	b.WriteString(fmt.Sprintf(" %-19s %12s %12s\n", "FETCHED", "SPEND", "LIMIT"))
	for _, p := range periods {
		b.WriteString(fmt.Sprintf(" %-19s %12s %12s\n",
			p.FetchedAt.Local().Format("2006-01-02 15:04"),
			money(float64(p.TotalSpendCents)), money(float64(p.LimitCents))))
	}
	return b.String()
}

func footer(a *App) string {
	source := a.result.SourceName
	if source == "" {
		source = "none"
	}
	age := dash
	if !a.result.FetchedAt.IsZero() {
		age = humanAge(a.deps.Now().Sub(a.result.FetchedAt))
	}
	return fmt.Sprintf(" data: %s old via %s · [r]efresh [1-4] tabs [q]uit", age, source)
}

func humanAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
