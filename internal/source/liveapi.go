package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// post is the HTTP transport used by LiveAPI; a package var so tests stub it.
// It sends the Authorization header required by DashboardService.
var post = func(url string, body []byte, token string) ([]byte, int, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

// nowFn is the clock; a package var so tests pin the calendar month.
var nowFn = time.Now

// monthBounds returns [first of this month, first of next month) in local
// time, in epoch milliseconds.
func monthBounds(now time.Time) (int64, int64) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return start.UnixMilli(), start.AddDate(0, 1, 0).UnixMilli()
}

// LiveAPI reads spend data from Cursor's DashboardService ConnectRPC API.
type LiveAPI struct {
	endpoint string
	token    string
}

// NewLiveAPI builds a live source. endpoint is the API base, e.g.
// https://api2.cursor.sh.
func NewLiveAPI(endpoint, token string) *LiveAPI {
	return &LiveAPI{endpoint: endpoint, token: token}
}

// Name implements UsageSource.
func (a *LiveAPI) Name() string { return "live" }

const methodDailySpend = "/aiserver.v1.DashboardService/GetDailySpendByCategory"

type dailySpendResponse struct {
	DailySpend []struct {
		Day         string `json:"day"`
		Category    string `json:"category"`
		SpendCents  int64  `json:"spendCents"`
		TotalTokens string `json:"totalTokens"`
	} `json:"dailySpend"`
	EffectiveLimitCents int64 `json:"effectiveLimitCents"`
}

func (a *LiveAPI) call(method string, req any, out any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	respBody, code, err := post(a.endpoint+method, body, a.token)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d: %s", method, code, truncate(respBody, 200))
	}
	return json.Unmarshal(respBody, out)
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}

// FetchPeriodUsage implements UsageSource for the current calendar month,
// from the 1st through the end of the month.
func (a *LiveAPI) FetchPeriodUsage(ctx context.Context) (*PeriodUsage, error) {
	now := nowFn()
	startMs, endMs := monthBounds(now)

	daily, err := a.fetchDaily(startMs, endMs)
	if err != nil {
		return nil, err
	}

	var total int64
	for _, r := range daily.DailySpend {
		total += r.SpendCents
	}
	return &PeriodUsage{
		PeriodStartMs:   startMs,
		PeriodEndMs:     endMs,
		TotalSpendCents: total,
		LimitCents:      daily.EffectiveLimitCents,
		FetchedAt:       now,
	}, nil
}

// FetchDailySpend implements UsageSource.
func (a *LiveAPI) FetchDailySpend(ctx context.Context, periodStartMs, periodEndMs int64) ([]DailySpend, error) {
	daily, err := a.fetchDaily(periodStartMs, periodEndMs)
	if err != nil {
		return nil, err
	}
	rows := make([]DailySpend, 0, len(daily.DailySpend))
	for _, r := range daily.DailySpend {
		tokens, _ := strconv.ParseInt(r.TotalTokens, 10, 64)
		ms, err := strconv.ParseInt(r.Day, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad day %q: %w", r.Day, err)
		}
		utc := time.UnixMilli(ms).UTC()
		rows = append(rows, DailySpend{
			Day:         time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.Local),
			Category:    r.Category,
			SpendCents:  r.SpendCents,
			TotalTokens: tokens,
		})
	}
	return rows, nil
}

func (a *LiveAPI) fetchDaily(startMs, endMs int64) (*dailySpendResponse, error) {
	var out dailySpendResponse
	err := a.call(methodDailySpend, map[string]any{
		"periodStartMs": startMs,
		"periodEndMs":   endMs,
	}, &out)
	return &out, err
}
