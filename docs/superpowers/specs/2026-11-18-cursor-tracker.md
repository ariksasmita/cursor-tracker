# cursor-tracker — Spec

Terminal UI dashboard tracking Cursor AI spend against plan limit. Built for a
workflow where Cursor usage flows through **OpenBot → `cursor-agent` CLI (API
key)**, not the Cursor app.

## Goal

Open a terminal, see: how much of the plan's spend limit is used, burn rate,
projection, per-model token/$ breakdown, and agent session activity. Data
accumulates into local history forever.

## Primary views (priority order)

1. **Overview** — spend vs limit gauge, burn rate ($/day), end-of-period
   projection, days-to-limit.
2. **Models** — per-model-per-day tokens + cost breakdown (from
   `GetDailySpendByCategory`), with sparklines.
3. **Sessions** — agent/model/turn activity from OpenBot's DB.
4. **History** — accumulated period snapshots over time.

## Data sources (UsageSource interface)

| Source | Role | Freshness |
|---|---|---|
| `LiveAPI` — ConnectRPC to `https://api2.cursor.sh/aiserver.v1.DashboardService` (`GetCurrentPeriodUsage`, `GetDailySpendByCategory`, optional probe `GetFilteredUsageEvents`) | primary: spend vs limit + per-model/day | live |
| `OpenBot` — `~/Library/Application Support/OpenBot/openbot.db` (read-only): `agent_usage_activity`, `projection_provider_sessions` | sessions/activity tab | live |
| `LocalSnapshot` — `state.vscdb` ItemTable `aiSettings.enterprisePlanUsageSnapshot` | fallback when API unreachable | stale |
| own history DB `~/.cursor-tracker/history.db` | persistent time series; upsert every poll | grows forever |

## Auth

Token resolution order:
1. macOS keychain `security find-generic-password -a cursor-user -s cursor-access-token -w` (kept fresh by the daily CLI runs)
2. `state.vscdb` ItemTable key `cursorAuth/accessToken`
3. `CURSOR_TOKEN` env override

## Verified data shapes (from local recon, 2026-11-18)

`GetDailySpendByCategory` response (as cached in `aiSettings.enterprisePlanUsageSnapshot.charts[].response`):

```json
{"dailySpend":[{"day":"1793664000000","category":"model-alpha","spendCents":330,"totalTokens":"1234567"}],
 "categories":["model-alpha"],
 "effectiveLimitCents":10000}
```

`GetCurrentPeriodUsage` exposes `planUsage.{totalSpend,includedSpend,bonusSpend,limit,autoSpend,apiSpend}` (cents — field names from app bundle; live spike in Task 3 confirms exact JSON casing).

## Non-goals

- Writing to any Cursor/OpenBot database (strictly read-only, `mode=ro`).
- Live streaming of in-flight requests.
- Cost accounting for non-Cursor providers (OpenBot's opencode/pi records out of scope).

## Anti-goals (failure even if it technically works)

- Crashing or hanging when the API is unreachable, token expired, or a DB is locked — must degrade to available sources with a visible warning.
- Requiring the Cursor desktop app to be running.

## Decisions

- Go + Bubble Tea (bubbletea/bubbles/lipgloss), pure-Go SQLite (`modernc.org/sqlite`) — single static binary, no CGO.
- Unofficial API risk accepted; all API calls are read-only unary POSTs; fallback sources cover outage.
- `UsageSource`/`SessionSource` interfaces so sources are swappable.
- Binary name: `cursor-tracker`. History DB path overridable via `CURSOR_TRACKER_HOME`.
