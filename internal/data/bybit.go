// Package data implements the DataHandler: it sources OHLCV bars from Bybit's
// public REST API and caches them on disk so reruns are instant and offline-safe.
package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/leykun/bybit-backtester/internal/engine"
)

const bybitBase = "https://api.bybit.com"

// bybitLimit is the max bars Bybit returns per kline request.
const bybitLimit = 1000

// intervalMinutes maps a Bybit interval string to its duration in minutes.
var intervalMinutes = map[string]int{"1": 1, "5": 5, "15": 15}

// IntervalMinutes returns the duration of a Bybit interval in minutes (0 if
// unknown). Exported so callers can align time windows to bar boundaries.
func IntervalMinutes(interval string) int { return intervalMinutes[interval] }

// FetchKlines pulls [start, end) bars for symbol/interval from Bybit, paginating
// as needed (Bybit caps each response at 1000 bars and returns them newest-first).
// The returned slice is sorted oldest-first, ready to stream through the engine.
func FetchKlines(symbol, interval string, start, end time.Time) ([]engine.Bar, error) {
	mins, ok := intervalMinutes[interval]
	if !ok {
		return nil, fmt.Errorf("unsupported interval %q (use 1, 5, or 15)", interval)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// We know the time range and bars-per-page up front, so instead of walking
	// the pages one sequential round-trip at a time, we compute every page
	// window ahead of time and fetch them concurrently with a bounded pool.
	// This turns N sequential HTTP calls into a few parallel batches — the
	// single biggest speedup for the fetch.
	page := time.Duration(mins) * time.Minute * bybitLimit
	type window struct{ start, end time.Time }
	var windows []window
	for s := start; s.Before(end); s = s.Add(page) {
		e := s.Add(page)
		if e.After(end) {
			e = end
		}
		windows = append(windows, window{s, e})
	}

	results := make([][]engine.Bar, len(windows))
	errs := make([]error, len(windows))

	const maxConcurrency = 8 // be a good citizen to Bybit's rate limits
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for i, w := range windows {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, w window) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i], errs[i] = fetchPageWithRetry(client, symbol, interval, w.start, w.end)
		}(i, w)
	}
	wg.Wait()

	var all []engine.Bar
	for i := range windows {
		if errs[i] != nil {
			return nil, errs[i]
		}
		all = append(all, results[i]...)
	}

	// Dedup + sort oldest-first.
	seen := make(map[int64]struct{}, len(all))
	out := all[:0]
	for _, b := range all {
		k := b.Time.UnixMilli()
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		if !b.Time.Before(start) && b.Time.Before(end) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out, nil
}

// klineResponse models the subset of Bybit's /v5/market/kline response we use.
type klineResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List [][]string `json:"list"` // [startMs, open, high, low, close, volume, turnover]
	} `json:"result"`
}

// fetchPageWithRetry wraps fetchPage with a few retries and exponential backoff.
// Bybit (and container networks) can hiccup; a transient timeout shouldn't fail
// a whole backtest during a live demo.
func fetchPageWithRetry(client *http.Client, symbol, interval string, start, end time.Time) ([]engine.Bar, error) {
	const attempts = 4
	var lastErr error
	for i := 0; i < attempts; i++ {
		bars, err := fetchPage(client, symbol, interval, start, end)
		if err == nil {
			return bars, nil
		}
		lastErr = err
		if i < attempts-1 {
			// 1s, 2s, 4s backoff.
			time.Sleep(time.Duration(1<<i) * time.Second)
		}
	}
	return nil, fmt.Errorf("bybit fetch failed after %d attempts: %w", attempts, lastErr)
}

func fetchPage(client *http.Client, symbol, interval string, start, end time.Time) ([]engine.Bar, error) {
	url := fmt.Sprintf(
		"%s/v5/market/kline?category=spot&symbol=%s&interval=%s&start=%d&end=%d&limit=%d",
		bybitBase, symbol, interval, start.UnixMilli(), end.UnixMilli(), bybitLimit,
	)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("bybit request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bybit body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bybit HTTP %d: %s", resp.StatusCode, string(body))
	}

	var kr klineResponse
	if err := json.Unmarshal(body, &kr); err != nil {
		return nil, fmt.Errorf("decode bybit json: %w", err)
	}
	if kr.RetCode != 0 {
		return nil, fmt.Errorf("bybit error %d: %s", kr.RetCode, kr.RetMsg)
	}

	bars := make([]engine.Bar, 0, len(kr.Result.List))
	for _, row := range kr.Result.List {
		if len(row) < 6 {
			continue
		}
		b, err := parseRow(row)
		if err != nil {
			return nil, err
		}
		bars = append(bars, b)
	}
	return bars, nil
}

func parseRow(row []string) (engine.Bar, error) {
	ms, err := strconv.ParseInt(row[0], 10, 64)
	if err != nil {
		return engine.Bar{}, fmt.Errorf("parse time: %w", err)
	}
	f := func(s string) (float64, error) { return strconv.ParseFloat(s, 64) }
	open, err := f(row[1])
	if err != nil {
		return engine.Bar{}, err
	}
	high, err := f(row[2])
	if err != nil {
		return engine.Bar{}, err
	}
	low, err := f(row[3])
	if err != nil {
		return engine.Bar{}, err
	}
	cl, err := f(row[4])
	if err != nil {
		return engine.Bar{}, err
	}
	vol, err := f(row[5])
	if err != nil {
		return engine.Bar{}, err
	}
	return engine.Bar{
		Time:   time.UnixMilli(ms).UTC(),
		Open:   open,
		High:   high,
		Low:    low,
		Close:  cl,
		Volume: vol,
	}, nil
}
