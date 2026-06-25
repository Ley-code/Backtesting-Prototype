// Package broker simulates order execution in a backtest. There is no real
// exchange, so the broker MODELS reality: it applies a slippage adjustment to
// the fill price and charges a percentage fee. A backtest with zero fees and
// perfect fills is fantasy — modelling both keeps results honest.
package broker

import "github.com/leykun/bybit-backtester/internal/engine"

// Simulated fills market orders at the bar's close, adjusted for slippage,
// and charges a fee proportional to notional.
type Simulated struct {
	feeRate      float64 // e.g. 0.0006 = 6 bps (Bybit-ish taker fee)
	slippageRate float64 // e.g. 0.0002 = 2 bps adverse price move
}

// NewSimulated builds a broker. feeRate and slippageRate are fractions
// (0.0006 == 0.06%). Sensible defaults are applied for non-positive inputs.
func NewSimulated(feeRate, slippageRate float64) *Simulated {
	if feeRate <= 0 {
		feeRate = 0.0006
	}
	if slippageRate < 0 {
		slippageRate = 0.0002
	}
	return &Simulated{feeRate: feeRate, slippageRate: slippageRate}
}

// Execute fills the order against the given bar's OPEN. The engine hands an
// order to the broker on the bar *after* the strategy decided, so filling at
// the open is the earliest realistic execution and avoids look-ahead. Buys fill
// slightly above the open, sells slightly below — slippage always works against
// the trader, which is the conservative (honest) assumption.
func (b *Simulated) Execute(o engine.Order, bar engine.Bar) engine.Fill {
	price := bar.Open
	if o.Side == engine.Buy {
		price *= (1 + b.slippageRate)
	} else {
		price *= (1 - b.slippageRate)
	}

	fee := price * o.Qty * b.feeRate
	return engine.Fill{
		Time:  o.Time,
		Side:  o.Side,
		Qty:   o.Qty,
		Price: price,
		Fee:   fee,
	}
}
