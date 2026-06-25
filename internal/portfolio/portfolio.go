// Package portfolio tracks cash, the open position, and the equity curve — the
// account value at every bar. The equity curve is the single source of truth
// from which all performance metrics are computed.
package portfolio

import (
	"time"

	"github.com/leykun/bybit-backtester/internal/engine"
)

// EquityPoint is one timestamped account value on the equity curve.
type EquityPoint struct {
	Time   time.Time `json:"time"`
	Equity float64   `json:"equity"`
}

// TradeLog records an executed fill for display and analysis.
type TradeLog struct {
	Time  time.Time `json:"time"`
	Side  string    `json:"side"`
	Qty   float64   `json:"qty"`
	Price float64   `json:"price"`
	Fee   float64   `json:"fee"`
}

// Portfolio holds quote cash and a base position (long-only in this prototype).
type Portfolio struct {
	cash     float64
	position float64 // base units held (e.g. BTC)

	equity []EquityPoint
	trades []TradeLog
}

func New(initialCash float64) *Portfolio {
	return &Portfolio{cash: initialCash}
}

// OnFill applies an executed trade to cash and position.
func (p *Portfolio) OnFill(f engine.Fill) {
	notional := f.Price * f.Qty
	switch f.Side {
	case engine.Buy:
		p.cash -= notional + f.Fee
		p.position += f.Qty
	case engine.Sell:
		p.cash += notional - f.Fee
		p.position -= f.Qty
	}
	p.trades = append(p.trades, TradeLog{
		Time: f.Time, Side: f.Side.String(), Qty: f.Qty, Price: f.Price, Fee: f.Fee,
	})
}

// MarkToMarket records total account value (cash + position valued at the bar's
// close) onto the equity curve.
func (p *Portfolio) MarkToMarket(bar engine.Bar) {
	equity := p.cash + p.position*bar.Close
	p.equity = append(p.equity, EquityPoint{Time: bar.Time, Equity: equity})
}

func (p *Portfolio) Position() float64        { return p.position }
func (p *Portfolio) Cash() float64            { return p.cash }
func (p *Portfolio) Equity() []EquityPoint    { return p.equity }
func (p *Portfolio) Trades() []TradeLog       { return p.trades }
