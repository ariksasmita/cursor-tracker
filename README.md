# cursor-tracker

Terminal dashboard for Cursor AI spend: month-to-date spend against your plan
limit, burn rate and end-of-month projection, per-model token/cost breakdown, and
OpenBot agent session activity. Built for the **OpenBot → `cursor-agent` (API key)**
workflow, so it works without the Cursor desktop app running.

```
 Cursor Spend ── Nov 1–Dec 1 2026

 $21.30 / $100.00 ██████░░░░░░░░░░░░░░░░░░░░░░░░░░  21.3%
 Burn rate: $1.20/day · Projected EOM: $36.05 · Days to limit: 65

 [1] Overview  [2] Models  [3] Sessions  [4] History
```

 Models — period $21.30 · 281.22M tokens

 CATEGORY                               TOKENS      SPEND  14-DAY
 model-alpha                      6.98M      $4.75  ▁▃▁█▂▁▁▁▁▁▁▁▁▁
 model-epsilon                        6.13M      $4.57  ▁▁▁▁▁█▁▁▁▁▁▁▁▁
 model-delta          225.73M      $4.46  ▂▁█▃▂▁▁▁▁▁▁▁▁▁
 model-beta                       4.78M      $3.88  ▁▁▁▁▁▁▁▁▁▁▁▁▁▁
```

## Install

```bash
go install github.com/sasmitai/cursor-tracker@latest
```

Or from a checkout: `go build -o cursor-tracker .`

Requires Go 1.22+. No CGO: SQLite comes from `modernc.org/sqlite`, so the binary
is static and self-contained.

Token discovery uses the macOS keychain, so on macOS it works with no setup at
all. On Linux/Windows, set `CURSOR_TOKEN` (or point `CURSOR_STATE_DB` at a Cursor
`state.vscdb`).

## Keys

| Key | Action |
|---|---|
| `1`–`4` | Overview / Models / Sessions / History |
| `r` | Refresh now (auto-refreshes every 60s) |
| `q`, `ctrl+c` | Quit |

## Data sources

| Source | Provides | Notes |
|---|---|---|
| Cursor DashboardService API (`api2.cursor.sh`) | spend vs limit, per-model/day spend + tokens | primary; unofficial read-only API |
| OpenBot `openbot.db` | agent session activity (provider/model/kind per event) | read-only, `busy_timeout` |
| Cursor `state.vscdb` snapshot | cached dashboard data | fallback, only as fresh as the app's last run |
| `~/.cursor-tracker/history.db` | accumulated snapshots | the tracker's own DB; one row per spend change, not per refresh |

If a source is unavailable the dashboard keeps working and shows a warning line.

## Token

Resolved in this order, first hit wins:

1. `CURSOR_TOKEN` environment variable
2. macOS keychain entry `cursor-user` / `cursor-access-token` (kept fresh by `cursor-agent` runs)
3. `cursorAuth/accessToken` inside Cursor's `state.vscdb`

## Environment

| Variable | Default |
|---|---|
| `CURSOR_TOKEN` | — (token override) |
| `CURSOR_API_ENDPOINT` | `https://api2.cursor.sh` |
| `CURSOR_TRACKER_HOME` | `~/.cursor-tracker` |
| `CURSOR_STATE_DB` | `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb` |
| `OPENBOT_DB` | `~/Library/Application Support/OpenBot/openbot.db` |

## Notes

- Strictly read-only against Cursor and OpenBot files.
- The period window is the **current calendar month**: from the 1st through the
  end of the month, in local time. Both the live API and the cached-snapshot
  fallback report that same window, so month-to-date spend is comparable
  whichever source answered.
- `aiSettings.enterprisePlanUsageSnapshot`-backed fallback data can be stale; the
  footer always shows the age of the data and which source produced it.
