package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leykun/bybit-backtester/internal/engine"
)

// BarFeed is a simple in-memory DataHandler: it streams a pre-loaded, ordered
// slice of bars one at a time. Loading from Bybit/cache happens up front in
// Load; the engine only ever sees Next(). This keeps the look-ahead guarantee
// purely a function of iteration order.
type BarFeed struct {
	bars []engine.Bar
	i    int
}

func (f *BarFeed) Next() (engine.Bar, bool) {
	if f.i >= len(f.bars) {
		return engine.Bar{}, false
	}
	b := f.bars[f.i]
	f.i++
	return b, true
}

// Len reports how many bars were loaded (handy for the API response).
func (f *BarFeed) Len() int { return len(f.bars) }

// Bars exposes the loaded bars (read-only use) for charting the price series.
func (f *BarFeed) Bars() []engine.Bar { return f.bars }

// Load returns a BarFeed for symbol/interval over [start, end), reading from the
// on-disk cache if present and otherwise fetching from Bybit and caching the
// result. This is the DataHandler abstraction the engine depends on: today it's
// Bybit + a JSON cache; swapping in a live websocket feed later changes nothing
// upstream.
func Load(cacheDir, symbol, interval string, start, end time.Time) (*BarFeed, error) {
	path := cachePath(cacheDir, symbol, interval, start, end)

	if bars, ok := readCache(path); ok {
		return &BarFeed{bars: bars}, nil
	}

	bars, err := FetchKlines(symbol, interval, start, end)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no bars returned for %s %sm in the requested range", symbol, interval)
	}
	writeCache(path, bars) // best-effort; a cache write failure shouldn't fail the run
	return &BarFeed{bars: bars}, nil
}

func cachePath(dir, symbol, interval string, start, end time.Time) string {
	name := fmt.Sprintf("%s_%sm_%d_%d.json", symbol, interval, start.Unix(), end.Unix())
	return filepath.Join(dir, name)
}

func readCache(path string) ([]engine.Bar, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var bars []engine.Bar
	if err := json.Unmarshal(b, &bars); err != nil {
		return nil, false
	}
	return bars, len(bars) > 0
}

func writeCache(path string, bars []engine.Bar) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(bars)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}
