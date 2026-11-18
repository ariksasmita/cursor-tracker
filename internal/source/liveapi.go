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

const (
	methodPeriodUsage = "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	methodDailySpend  = "/aiserver.v1.DashboardService/GetDailySpendByCategory"
)

type currentPeriodResponse struct {
	BillingCycleStart string `json:"billingCycleStart"`
	BillingCycleEnd   string `json:"billingCycleEnd"`
}

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

// FetchPeriodUsage implements UsageSource. The API's cycle fields are
// timestamps of the fetch moment rather than usable bounds, so the spend
// window defaults to the trailing 30 days when they are not sane.
func (a *LiveAPI) FetchPeriodUsage(ctx context.Context) (*PeriodUsage, error) {
	var period currentPeriodResponse
	if err := a.call(methodPeriodUsage, map[string]any{}, &period); err != nil {
		return nil, err
	}

	now := time.Now()
	startMs, endMs := parseBounds(period, now)

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

func parseBounds(period currentPeriodResponse, now time.Time) (int64, int64) {
	start, err1 := strconv.ParseInt(period.BillingCycleStart, 10, 64)
	end, err2 := strconv.ParseInt(period.BillingCycleEnd, 10, 64)
	sane := err1 == nil && err2 == nil && start > 0 && end > start && start > now.Add(-45*24*time.Hour).UnixMilli()
	if sane {
		return start, end
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return dayStart.AddDate(0, 0, -29).UnixMilli(), dayStart.AddDate(0, 0, 1).UnixMilli()
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
