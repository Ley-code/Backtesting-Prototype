// Package metrics computes performance statistics from the equity curve. For
// this prototype the two required values are the Sharpe ratio and maximum
// drawdown.
package metrics

import (
	"math"

	"github.com/leykun/bybit-backtester/internal/portfolio"
)

// Result holds the computed metrics plus a couple of context values that make
// the demo readable.
type Result struct {
	SharpeRatio   float64 `json:"sharpe_ratio"`
	MaxDrawdown   float64 `json:"max_drawdown"`   // as a fraction, e.g. 0.12 = 12%
	TotalReturn   float64 `json:"total_return"`   // fraction over the whole run
	FinalEquity   float64 `json:"final_equity"`
	InitialEquity float64 `json:"initial_equity"`
	NumTrades     int     `json:"num_trades"`
}

// barsPerYear is the number of bars of each timeframe in a 365-day year. It is
// the annualization factor for the Sharpe ratio: 1m/5m/15m have very different
// counts, so each timeframe must be annualized with its own value.
var barsPerYear = map[string]float64{
	"1":  365 * 24 * 60,      // every minute
	"5":  365 * 24 * 60 / 5,  // every 5 minutes
	"15": 365 * 24 * 60 / 15, // every 15 minutes
}

// Compute derives the metrics from the equity curve. interval is the Bybit
// timeframe ("1", "5", "15") used to annualize Sharpe correctly.
func Compute(eq []portfolio.EquityPoint, numTrades int, interval string) Result {
	if len(eq) < 2 {
		var fe float64
		if len(eq) == 1 {
			fe = eq[0].Equity
		}
		return Result{FinalEquity: fe, InitialEquity: fe, NumTrades: numTrades}
	}

	initial := eq[0].Equity
	final := eq[len(eq)-1].Equity

	// Per-bar simple returns.
	returns := make([]float64, 0, len(eq)-1)
	for i := 1; i < len(eq); i++ {
		prev := eq[i-1].Equity
		if prev == 0 {
			returns = append(returns, 0)
			continue
		}
		returns = append(returns, (eq[i].Equity-prev)/prev)
	}

	res := Result{
		SharpeRatio:   sharpe(returns, barsPerYear[interval]),
		MaxDrawdown:   maxDrawdown(eq),
		TotalReturn:   (final - initial) / initial,
		FinalEquity:   final,
		InitialEquity: initial,
		NumTrades:     numTrades,
	}
	return res
}

// sharpe = mean(returns)/std(returns) * sqrt(periodsPerYear), risk-free rate
// assumed 0 for the prototype. Measures return per unit of risk.
func sharpe(returns []float64, periodsPerYear float64) float64 {
	if len(returns) < 2 || periodsPerYear <= 0 {
		return 0
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	variance := 0.0
	for _, r := range returns {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(returns) - 1) // sample std dev
	std := math.Sqrt(variance)
	if std == 0 {
		return 0
	}
	return (mean / std) * math.Sqrt(periodsPerYear)
}

// maxDrawdown is the worst peak-to-trough fractional drop in the equity curve.
// We track the running maximum and the largest percentage fall from it.
func maxDrawdown(eq []portfolio.EquityPoint) float64 {
	peak := eq[0].Equity
	worst := 0.0
	for _, p := range eq {
		if p.Equity > peak {
			peak = p.Equity
		}
		if peak > 0 {
			dd := (peak - p.Equity) / peak
			if dd > worst {
				worst = dd
			}
		}
	}
	return worst
}
