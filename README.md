# cursor-tracker

Terminal dashboard for Cursor AI spend: how much of your plan limit is used,
burn rate and projection, per-model token/cost breakdown, and OpenBot agent
session activity. Built for the **OpenBot → `cursor-agent` (API key)** workflow,
so it works without the Cursor desktop app running.

```
 Cursor Spend ── Oct 20–Nov 19 2026

 $12.90 / $100.00 ████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  12.9%
 Burn rate: $0.72/day · Projected EOM: $21.6 · Days to limit: 121

 [1] Overview  [2] Models  [3] Sessions  [4] History
```

## Install

```bash
go install github.com/sasmitai/cursor-tracker@latest
```

Or from a checkout: `go build -o cursor-tracker .`

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
| `~/.cursor-tracker/history.db` | accumulated snapshots | the tracker's own DB, grows forever |

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
- The period window is the API's billing cycle when it reports sane bounds;
  otherwise it falls back to a trailing 30-day window.
- `aiSettings.enterprisePlanUsageSnapshot`-backed fallback data can be stale; the
  footer always shows the age of the data and which source produced it.
