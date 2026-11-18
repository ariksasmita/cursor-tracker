# cursor-tracker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go TUI dashboard showing Cursor plan spend vs limit, per-model token/$ breakdown, and OpenBot agent session activity, accumulating history into its own SQLite DB.

**Architecture:** Sources sit behind two interfaces (`UsageSource`, `SessionSource`): a live ConnectRPC client for `api2.cursor.sh` DashboardService (primary), a read-only OpenBot SQLite reader (sessions), and a stale state.vscdb snapshot reader (fallback). An aggregator computes burn rate/projections; a Bubble Tea TUI renders four tabs; every poll upserts into `~/.cursor-tracker/history.db`.

**Tech Stack:** Go 1.22+, github.com/charmbracelet/bubbletea+bubbles+lipgloss, modernc.org/sqlite (no CGO).

**Spec:** `docs/superpowers/specs/2026-11-18-cursor-tracker.md`

## Global Constraints

- Read-only access to all foreign DBs: open with `file:<path>?mode=ro` URI; never write Cursor/OpenBot files.
- No CGO; SQLite via `modernc.org/sqlite`.
- Binary name `cursor-tracker`; module `github.com/sasmitai/cursor-tracker`.
- API base URL `https://api2.cursor.sh` overridable via env `CURSOR_API_ENDPOINT`.
- Token resolution order: env `CURSOR_TOKEN` → keychain (`security find-generic-password -a cursor-user -s cursor-access-token -w`) → `state.vscdb` `cursorAuth/accessToken`.
- History DB at `$CURSOR_TRACKER_HOME/history.db`, default `~/.cursor-tracker/history.db` (create dir on first run).
- All JSON decoding tolerant of unknown fields (no DisallowUnknownFields).
- Every visible number derived from cents; money rendered as `$X.XX`.

## Review Focus

1. **Token unavailable** (no env, keychain denies, state.vscdb missing) → TUI shows LocalSnapshot/fallback with warning line, never a blank crash. → Task 9 step test.
2. **API unreachable/500** → warning + fallback sources; refresh key retries. → Task 9 step test.
3. **First run, empty history; period elapsed ≤ 1 day** → burn rate 0 (no divide-by-zero), projections render "—". → Task 5 tests.
4. **`limit == 0`** (no spend limit set) → gauge shows spend without percent, projection rows hidden. → Task 5 tests.
5. **OpenBot DB locked (WAL, app running)** → open `mode=ro` + `busy_timeout=2000`; Sessions tab shows error row, app keeps running. → Task 6 tests.

---

### Task 1: Scaffold and domain types

**Files:**
- Create: `go.mod`, `main.go` (stub), `internal/source/source.go`

**Interfaces:**
- Produces (used by Tasks 3,4,5,6,7,8):

```go
package source

type PeriodUsage struct {
    PeriodStartMs, PeriodEndMs                       int64
    TotalSpendCents, LimitCents                      int64
    IncludedSpendCents, ApiSpendCents, AutoSpendCents int64
    FetchedAt                                        time.Time
}

type DailySpend struct {
    Day         time.Time // parsed from epoch-ms "day" field, local tz
    Category    string
    SpendCents  int64
    TotalTokens int64
}

type SessionEvent struct {
    AgentID, Provider, Model, Kind string
    OccurredAt                     time.Time
}

type UsageSource interface {
    Name() string
    FetchPeriodUsage(ctx context.Context) (*PeriodUsage, error)
    FetchDailySpend(ctx context.Context, periodStartMs, periodEndMs int64) ([]DailySpend, error)
}

type SessionSource interface {
    FetchSessions(ctx context.Context, limit int) ([]SessionEvent, error)
}
```

- [ ] **Step 1: Init module** — `go mod init github.com/sasmitai/cursor-tracker`; `main.go` prints "cursor-tracker" and exits 0.
- [ ] **Step 2: Verify** — `go build ./...` passes.
- [ ] **Step 3: Commit** — `git init` if needed; `git add -A; git commit -m "chore: scaffold module and domain types"`.

### Task 2: Token resolution

**Files:**
- Create: `internal/auth/token.go`
- Test: `internal/auth/token_test.go`

**Interfaces:**
- Consumes: none.
- Produces: `func ResolveToken(ctx context.Context) (string, string, error)` — returns (token, origin) where origin ∈ {"env","keychain","state.vscdb"}; empty token → error naming every source tried. Var `KeychainCmd func(ctx context.Context) ([]byte, error)` (defaults to running `security find-generic-password -a cursor-user -s cursor-access-token -w`) and `StateDbPath string` (defaults to `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`) are package vars so tests can stub them.

- [ ] **Step 1: Write failing tests** — table test: env set → returns ("tok","env"); keychain stub returns token → ("tok","keychain"); both fail but state DB fixture (temp SQLite with ItemTable row `cursorAuth/accessToken`) → ("tok","state.vscdb"); all fail → error mentions env, keychain, state.vscdb.
- [ ] **Step 2: Run** — `go test ./internal/auth/` → FAIL (undefined: ResolveToken).
- [ ] **Step 3: Implement** — order: env `CURSOR_TOKEN` → `KeychainCmd` (trim whitespace) → open `file:<StateDbPath>?mode=ro` with modernc sqlite, `SELECT value FROM ItemTable WHERE key='cursorAuth/accessToken'`.
- [ ] **Step 4: Run** — `go test ./internal/auth/` → PASS.
- [ ] **Step 5: Commit** — `git add -A; git commit -m "feat: cursor token resolution (env/keychain/state.vscdb)"`.

### Task 3: LiveAPI source

**Files:**
- Create: `internal/source/liveapi.go`
- Test: `internal/source/liveapi_test.go`, testdata `internal/source/testdata/period.json`, `daily.json`

**Interfaces:**
- Consumes: `source.UsageSource` (Task 1), nothing from Task 2 (token injected).
- Produces: `func NewLiveAPI(endpoint, token string) *LiveAPI` implementing `UsageSource`. HTTP via a package-level `func post(url string, body []byte) ([]byte, int, error)` variable stubbable in tests. Requests: POST `<endpoint>/aiserver.v1.DashboardService/GetCurrentPeriodUsage` and `.../GetDailySpendByCategory`, headers `Content-Type: application/json`, `Authorization: Bearer <token>`; daily request body `{"periodStartMs":<ms>,"periodEndMs":<ms>}`; 10s timeout via ctx.

- [ ] **Step 1: Live spike — verify endpoints and record fixtures (manual, guard with token presence)**

```bash
TOKEN=$(security find-generic-password -a cursor-user -s cursor-access-token -w 2>/dev/null || sqlite3 "file:$HOME/Library/Application Support/Cursor/User/globalStorage/state.vscdb?mode=ro" "SELECT value FROM ItemTable WHERE key='cursorAuth/accessToken';")
curl -s -X POST https://api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{}' > internal/source/testdata/period.json
curl -s -X POST https://api2.cursor.sh/aiserver.v1.DashboardService/GetDailySpendByCategory \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"periodStartMs\":<periodStartMs from period.json>,\"periodEndMs\":<periodEndMs>}" > internal/source/testdata/daily.json
```

Expected: JSON containing `planUsage`/`dailySpend` (per spec). If field casing differs, fix the struct tags below to the observed casing and update fixtures; if a different auth header is required (e.g. `Cookie`), record it in the testdata README and adjust `post`.
- [ ] **Step 2: Write failing tests** — stub `post` to serve testdata: `TestFetchPeriodUsage` asserts TotalSpendCents/LimitCents/period bounds parsed; `TestFetchDailySpend` asserts ≥1 row with Category/SpendCents/TotalTokens and Day parsed from epoch-ms string; `TestAPIError` — stub returns 401 body → error contains "401".
- [ ] **Step 3: Run** — `go test ./internal/source/` → FAIL.
- [ ] **Step 4: Implement** — structs mirror spec casing (`planUsage.totalSpend` etc.; `totalTokens` arrives as string per cached snapshot — decode as `json.Number`-safe: use `string` then `strconv.ParseInt`). Map to `PeriodUsage`/`[]DailySpend`.
- [ ] **Step 5: Run** — `go test ./internal/source/` → PASS.
- [ ] **Step 6: Commit** — `git add -A; git commit -m "feat: live DashboardService source"`.

### Task 4: History store

**Files:**
- Create: `internal/history/store.go`
- Test: `internal/history/store_test.go`

**Interfaces:**
- Consumes: `source.PeriodUsage`, `source.DailySpend` (Task 1).
- Produces:

```go
func Open(path string) (*Store, error) // creates dir + schema
func (s *Store) Close() error
func (s *Store) UpsertPeriod(p source.PeriodUsage) error
func (s *Store) UpsertDaily(periodStartMs int64, rows []source.DailySpend) error
func (s *Store) Periods() ([]source.PeriodUsage, error)         // ordered by fetchedAt
func (s *Store) DailyForPeriod(periodStartMs int64) ([]source.DailySpend, error)
```

Schema: `period_snapshots(fetched_at INTEGER PRIMARY KEY, period_start_ms, period_end_ms, total_spend_cents, limit_cents, included_spend_cents, api_spend_cents, auto_spend_cents)`; `daily_spend(period_start_ms TEXT, day TEXT, category TEXT, spend_cents INTEGER, total_tokens INTEGER, PRIMARY KEY(period_start_ms, day, category))` with `day` stored as `YYYY-MM-DD` local.

- [ ] **Step 1: Write failing tests** — temp dir store: UpsertPeriod twice (same fetchedAt) → Periods() length 1 with latest values; UpsertDaily twice → idempotent; empty store → Periods() empty, no error.
- [ ] **Step 2: Run** — `go test ./internal/history/` → FAIL.
- [ ] **Step 3: Implement** — modernc sqlite, `INSERT OR REPLACE`.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `git add -A; git commit -m "feat: history store"`.

### Task 5: Aggregate math

**Files:**
- Create: `internal/agg/agg.go`
- Test: `internal/agg/agg_test.go`

**Interfaces:**
- Consumes: `source.PeriodUsage` (Task 1).
- Produces:

```go
type Aggregate struct {
    BurnRateCentsPerDay, ProjectedEomSpendCents float64
    DaysElapsed, DaysRemaining                 float64
    DaysToLimit                                float64 // +Inf sentinel 0 with HasLimit=false when LimitCents<=0 or BurnRate<=0
    HasLimit, HasProjection                    bool
}
func ComputeAggregate(now time.Time, p *source.PeriodUsage) Aggregate
```

- [ ] **Step 1: Write failing tests** — 30-day period, 10 days elapsed, $30 spent → burn 300c/day, projection 900c, daysToLimit = (limit−spend)/burn; period start == now → BurnRate 0, HasProjection false; LimitCents 0 → HasLimit false, DaysToLimit 0; burn 0 with limit → HasLimit true, HasProjection false, DaysToLimit 0.
- [ ] **Step 2: Run** — `go test ./internal/agg/` → FAIL.
- [ ] **Step 3: Implement** — pure function, no IO.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `git add -A; git commit -m "feat: burn rate and projection math"`.

### Task 6: OpenBot session source

**Files:**
- Create: `internal/source/openbot.go`
- Test: `internal/source/openbot_test.go` (builds temp SQLite with the two tables)

**Interfaces:**
- Consumes: `SessionSource` (Task 1).
- Produces: `func NewOpenBot(path string) (*OpenBot, error)` — opens `file:<path>?mode=ro`; `FetchSessions` runs `SELECT a.agent_id, a.provider, a.model, a.kind, a.occurred_at FROM agent_usage_activity a ORDER BY a.occurred_at DESC LIMIT ?`. DSN includes `&_busy_timeout=2000`.

- [ ] **Step 1: Write failing tests** — fixture DB with 3 rows across providers → returns newest-first, fields mapped (ISO-8601 `occurred_at` parsed to time.Time); missing file → error contains path; table absent → error mentions "agent_usage_activity".
- [ ] **Step 2: Run** — `go test ./internal/source/` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `git add -A; git commit -m "feat: openbot session source"`.

### Task 7: LocalSnapshot fallback source

**Files:**
- Create: `internal/source/snapshot.go`
- Test: `internal/source/snapshot_test.go` (temp SQLite ItemTable with a real-shaped snapshot JSON)

**Interfaces:**
- Consumes: `UsageSource` (Task 1).
- Produces: `func NewLocalSnapshot(path string) (*LocalSnapshot, error)` — reads ItemTable `aiSettings.enterprisePlanUsageSnapshot`; `FetchPeriodUsage` sums the newest chart period's `dailySpend.spendCents`, takes `effectiveLimitCents` and period bounds, `FetchedAt` from top-level `fetchedAt`; `FetchDailySpend(periodStartMs, periodEndMs)` returns the matching chart's rows. No `SessionSource`.

- [ ] **Step 1: Write failing tests** — fixture with 2 chart periods → period usage from the newest; daily filtered by period bounds; key absent → error "snapshot not found".
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: PASS.**
- [ ] **Step 5: Commit** — `git add -A; git commit -m "feat: local snapshot fallback source"`.

### Task 8: TUI

**Files:**
- Create: `internal/tui/app.go`, `internal/tui/tabs.go`, `internal/tui/render.go`
- Test: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `UsageSource`, `SessionSource` (Tasks 1,3,6,7), `history.Store` (Task 4), `agg.ComputeAggregate` (Task 5), `auth.ResolveToken` (Task 2).
- Produces: `func New(deps Deps) *App` where `Deps{Primary UsageSource; Fallback UsageSource; Sessions SessionSource; Store *history.Store; Now func() time.Time}`; `App` implements tea.Model. Data-fetch funcs are fields (`Fetch func(ctx) (FetchResult, error)`) so tests inject results without network.

Behavior: initial fetch on start; `r` re-fetch; `1`–`4` switch tabs; `q`/ctrl-c quit; auto-refresh tick every 60s; on successful fetch, upsert Period+Daily into history. Each successful/fetch result carries `SourceName` + `FetchedAt`; footer always renders `data: <age> via <source>` and warning lines for failed sources (never exit on fetch error).

- [ ] **Step 1: Write failing tests** — inject FetchResult: `TestOverviewRender` — View() contains `$` formatted spend/limit and `via live`; `TestWarnings` — primary error → View() contains fallback warning + still renders; `TestEmptyHistory` — Store empty → View() renders without panic, projections show `—`; `TestTabSwitch` — press `3` → View() contains "Sessions" header.
- [ ] **Step 2: Run** — `go test ./internal/tui/` → FAIL.
- [ ] **Step 3: Implement** — four render funcs: overview (gauge bar of `#`/`·` scaled to width, burn/projection lines via Aggregate), models (group DailySpend by category: period total $, tokens, 14-day sparkline of `▁▂▃▄▅▆▇`), sessions (table of latest 50 SessionEvents), history (one line per stored period snapshot). Lipgloss styles; no external gauge lib.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** — `git add -A; git commit -m "feat: bubbletea TUI with four tabs"`.

### Task 9: Wiring, flags, README

**Files:**
- Create: `internal/app/run.go`; Modify: `main.go`; Create: `README.md`
- Test: `internal/app/run_test.go`

**Interfaces:**
- Consumes: all prior tasks.
- Produces: `func Run(ctx context.Context, out io.Writer) error` — resolves token; builds LiveAPI (endpoint from env `CURSOR_API_ENDPOINT` default `https://api2.cursor.sh`); token failure or LiveAPI construction failure → degrade: primary = LocalSnapshot, warning recorded. OpenBot path default `~/Library/Application Support/OpenBot/openbot.db`, missing → Sessions source nil, tab shows "openbot.db not found". History at `$CURSOR_TRACKER_HOME/history.db` (default `~/.cursor-tracker`). tea.Program runs the App.

- [ ] **Step 1: Write failing tests** — with env pointing at a temp state.vscdb, no token sources → Run builds Deps with LocalSnapshot primary and a warning (assert via returned deps inspector `BuildDeps(ctx) (Deps, []string)` — split BuildDeps out of Run for testability; Run = BuildDeps + tea run). `TestBuildDeps_HappyPath` stubs env token → primary is LiveAPI.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement** BuildDeps + Run + README (keys table, env vars, install `go install`). **Step 4: Run** — PASS; `go build ./...` clean.
- [ ] **Step 5: Manual smoke** — `go run . ` renders Overview with live data (or fallback warning); `q` exits 0.
- [ ] **Step 6: Commit** — `git add -A; git commit -m "feat: wire sources into TUI, README"`.
