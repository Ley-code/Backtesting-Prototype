package api

import (
	"fmt"
	"time"

	"github.com/leykun/bybit-backtester/internal/broker"
	"github.com/leykun/bybit-backtester/internal/data"
	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/leykun/bybit-backtester/internal/metrics"
	"github.com/leykun/bybit-backtester/internal/portfolio"
	"github.com/leykun/bybit-backtester/internal/strategy"
)

// allowed inputs — the prototype's fixed surface.
var (
	allowedSymbols    = map[string]bool{"BTCUSDT": true, "ETHUSDT": true}
	allowedIntervals  = map[string]bool{"1": true, "5": true, "15": true}
	allowedStrategies = map[string]bool{"ma_crossover": true, "rsi_reversion": true}
)

// BacktestRequest is the validated input for a run. Params carries the
// strategy-specific knobs (e.g. fast/slow for MA, period/oversold/overbought for
// RSI); unknown or missing keys fall back to sensible defaults.
type BacktestRequest struct {
	Symbol   string         `json:"symbol"`
	Interval string         `json:"interval"`
	Strategy string         `json:"strategy"`
	Days     int            `json:"days"` // lookback window; defaults to 30
	Params   map[string]int `json:"params"`
}

// BacktestResult is the full payload returned to the frontend.
type BacktestResult struct {
	Request    BacktestRequest                     `json:"request"`
	Metrics    metrics.Result                      `json:"metrics"`
	Equity     []portfolio.EquityPoint             `json:"equity_curve"`
	Price      []pricePoint                        `json:"price"`      // close price per bar
	Indicators map[string][]strategy.IndicatorPoint `json:"indicators"` // overlay series
	RSIBands   *rsiBands                           `json:"rsi_bands,omitempty"`
	Trades     []portfolio.TradeLog                `json:"trades"`
	Bars       int                                 `json:"bars"`
	From       time.Time                           `json:"from"`
	To         time.Time                           `json:"to"`
	BuildMS    int64                               `json:"build_ms"`
}

// pricePoint is one close price for the price-chart view.
type pricePoint struct {
	Time  time.Time `json:"time"`
	Close float64   `json:"close"`
}

// rsiBands tells the UI where to draw the oversold/overbought reference lines.
type rsiBands struct {
	Oversold   float64 `json:"oversold"`
	Overbought float64 `json:"overbought"`
}

func validate(req *BacktestRequest) error {
	if !allowedSymbols[req.Symbol] {
		return fmt.Errorf("unsupported symbol %q (BTCUSDT or ETHUSDT)", req.Symbol)
	}
	if !allowedIntervals[req.Interval] {
		return fmt.Errorf("unsupported interval %q (1, 5, or 15)", req.Interval)
	}
	if !allowedStrategies[req.Strategy] {
		return fmt.Errorf("unsupported strategy %q", req.Strategy)
	}
	if req.Days <= 0 {
		req.Days = 30
	}
	if req.Days > 60 {
		req.Days = 60 // keep pagination bounded for the prototype
	}
	return nil
}

// param reads an int param with a default and clamps it to [min,max].
func param(p map[string]int, key string, def, min, max int) int {
	v, ok := p[key]
	if !ok || v == 0 {
		v = def
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return v
}

func buildStrategy(name string, p map[string]int) engine.Strategy {
	switch name {
	case "rsi_reversion":
		period := param(p, "period", 14, 2, 100)
		oversold := param(p, "oversold", 30, 1, 49)
		overbought := param(p, "overbought", 70, 51, 99)
		return strategy.NewRSIReversion(period, float64(oversold), float64(overbought))
	default:
		fast := param(p, "fast", 10, 2, 200)
		slow := param(p, "slow", 30, 3, 400)
		if slow <= fast {
			slow = fast + 1
		}
		return strategy.NewMACrossover(fast, slow)
	}
}

const initialCash = 10_000.0

// runBacktest executes one fully-isolated backtest: it loads data (Bybit +
// cache), wires fresh components, runs the engine, and computes metrics. No
// state is shared between runs.
func runBacktest(cacheDir string, req BacktestRequest) (*BacktestResult, error) {
	if err := validate(&req); err != nil {
		return nil, err
	}

	// Snap `to` down to the current interval boundary so repeated runs within
	// the same bar produce an identical [from,to] window — and therefore an
	// identical cache key, making reruns instant instead of re-fetching.
	mins := data.IntervalMinutes(req.Interval)
	bucket := time.Duration(mins) * time.Minute
	to := time.Now().UTC().Truncate(bucket)
	from := to.Add(-time.Duration(req.Days) * 24 * time.Hour)

	started := time.Now()
	feed, err := data.Load(cacheDir, req.Symbol, req.Interval, from, to)
	if err != nil {
		return nil, err
	}

	strat := buildStrategy(req.Strategy, req.Params)
	brk := broker.NewSimulated(0.0006, 0.0002)
	pf := portfolio.New(initialCash)

	eng := engine.New(feed, strat, brk, pf)
	eng.Run()

	res := metrics.Compute(pf.Equity(), len(pf.Trades()), req.Interval)

	// Price series (close per bar) for the price-chart view.
	price := make([]pricePoint, 0, feed.Len())
	for _, b := range feed.Bars() {
		price = append(price, pricePoint{Time: b.Time, Close: b.Close})
	}

	// Indicator overlays, if the strategy exposes them.
	var indicators map[string][]strategy.IndicatorPoint
	if ind, ok := strat.(strategy.Indicating); ok {
		indicators = ind.Indicators()
	}

	// RSI reference bands, if applicable.
	var bands *rsiBands
	if rsi, ok := strat.(*strategy.RSIReversion); ok {
		bands = &rsiBands{Oversold: rsi.Oversold(), Overbought: rsi.Overbought()}
	}

	return &BacktestResult{
		Request:    req,
		Metrics:    res,
		Equity:     pf.Equity(),
		Price:      price,
		Indicators: indicators,
		RSIBands:   bands,
		Trades:     pf.Trades(),
		Bars:       feed.Len(),
		From:       from,
		To:         to,
		BuildMS:    time.Since(started).Milliseconds(),
	}, nil
}
