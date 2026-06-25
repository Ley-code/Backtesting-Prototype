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

var (
	allowedSymbols = map[string]bool{"BTCUSDT": true, "ETHUSDT": true}
	allowedIntervals = map[string]bool{"1": true, "5": true, "15": true}
	allowedStrategies = map[string]bool{
		"ma_crossover":      true,
		"rsi_reversion":     true,
		"ema_crossover":     true,
		"bollinger_bounce":  true,
		"breakout":          true,
		"momentum_pct":      true,
		"tp_sl":             true,
	}
)

type BacktestRequest struct {
	Symbol   string         `json:"symbol"`
	Interval string         `json:"interval"`
	Strategy string         `json:"strategy"`
	Days     int            `json:"days"`
	Params   map[string]int `json:"params"`
}

type BacktestResult struct {
	Request    BacktestRequest                      `json:"request"`
	Metrics    metrics.Result                       `json:"metrics"`
	Equity     []portfolio.EquityPoint              `json:"equity_curve"`
	Price      []pricePoint                         `json:"price"`
	Indicators map[string][]strategy.IndicatorPoint `json:"indicators"`
	RSIBands   *rsiBands                            `json:"rsi_bands,omitempty"`
	Trades     []portfolio.TradeLog                 `json:"trades"`
	Bars       int                                  `json:"bars"`
	From       time.Time                            `json:"from"`
	To         time.Time                            `json:"to"`
	BuildMS    int64                                `json:"build_ms"`
}

type pricePoint struct {
	Time  time.Time `json:"time"`
	Close float64   `json:"close"`
}

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
		req.Days = 60
	}
	return nil
}

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

func maParams(p map[string]int) (fast, slow int) {
	fast = param(p, "fast", 10, 2, 200)
	slow = param(p, "slow", 30, 3, 400)
	if slow <= fast {
		slow = fast + 1
	}
	return fast, slow
}

func buildStrategy(name string, p map[string]int) engine.Strategy {
	switch name {
	case "rsi_reversion":
		period := param(p, "period", 14, 2, 100)
		oversold := param(p, "oversold", 30, 1, 49)
		overbought := param(p, "overbought", 70, 51, 99)
		return strategy.NewRSIReversion(period, float64(oversold), float64(overbought))
	case "ema_crossover":
		fast, slow := maParams(p)
		return strategy.NewEMACrossover(fast, slow)
	case "bollinger_bounce":
		period := param(p, "period", 20, 5, 100)
		stdTenths := param(p, "std_dev", 20, 10, 40)
		return strategy.NewBollingerBounce(period, float64(stdTenths)/10.0)
	case "breakout":
		lookback := param(p, "lookback", 20, 5, 200)
		return strategy.NewBreakout(lookback)
	case "momentum_pct":
		lookback := param(p, "lookback", 20, 5, 200)
		buyPct := param(p, "buy_pct", 10, 1, 50)
		sellPct := param(p, "sell_pct", 5, 1, 50)
		return strategy.NewMomentumPct(lookback, buyPct, sellPct)
	case "tp_sl":
		fast, slow := maParams(p)
		tpPct := param(p, "tp_pct", 10, 1, 100)
		slPct := param(p, "sl_pct", 5, 1, 50)
		return strategy.NewTakeProfitStopLoss(fast, slow, tpPct, slPct)
	default:
		fast, slow := maParams(p)
		return strategy.NewMACrossover(fast, slow)
	}
}

const initialCash = 10_000.0

func runBacktest(cacheDir string, req BacktestRequest) (*BacktestResult, error) {
	if err := validate(&req); err != nil {
		return nil, err
	}

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

	price := make([]pricePoint, 0, feed.Len())
	for _, b := range feed.Bars() {
		price = append(price, pricePoint{Time: b.Time, Close: b.Close})
	}

	var indicators map[string][]strategy.IndicatorPoint
	if ind, ok := strat.(strategy.Indicating); ok {
		indicators = ind.Indicators()
	}

	var bands *rsiBands
	if rsi, ok := strat.(*strategy.RSIReversion); ok {
		bands = &rsiBands{Oversold: rsi.Oversold(), Overbought: rsi.Overbought()}
	}

	trades := pf.Trades()
	if trades == nil {
		trades = []portfolio.TradeLog{}
	}

	return &BacktestResult{
		Request:    req,
		Metrics:    res,
		Equity:     pf.Equity(),
		Price:      price,
		Indicators: indicators,
		RSIBands:   bands,
		Trades:     trades,
		Bars:       feed.Len(),
		From:       from,
		To:         to,
		BuildMS:    time.Since(started).Milliseconds(),
	}, nil
}
