package data

import (
	"context"
	"fmt"
	"time"

	"github.com/leykun/bybit-backtester/internal/cache"
	"github.com/leykun/bybit-backtester/internal/engine"
)

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

func (f *BarFeed) Len() int { return len(f.bars) }

func (f *BarFeed) Bars() []engine.Bar { return f.bars }

func Load(ctx context.Context, rdb *cache.Client, symbol, interval string, start, end time.Time) (*BarFeed, error) {
	key := cache.BarsKey(symbol, interval, start, end)

	if bars, ok, err := rdb.GetBars(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return &BarFeed{bars: bars}, nil
	}

	bars, err := FetchKlines(symbol, interval, start, end)
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no bars returned for %s %sm in the requested range", symbol, interval)
	}
	_ = rdb.SetBars(ctx, key, bars)
	return &BarFeed{bars: bars}, nil
}
