package portfolio

import (
	"time"

	"github.com/leykun/bybit-backtester/internal/engine"
)

type EquityPoint struct {
	Time   time.Time `json:"time"`
	Equity float64   `json:"equity"`
}

type TradeLog struct {
	Time  time.Time `json:"time"`
	Side  string    `json:"side"`
	Qty   float64   `json:"qty"`
	Price float64   `json:"price"`
	Fee   float64   `json:"fee"`
}

type Portfolio struct {
	cash     float64
	position float64

	equity []EquityPoint
	trades []TradeLog
}

func New(initialCash float64) *Portfolio {
	return &Portfolio{cash: initialCash, trades: []TradeLog{}}
}

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

func (p *Portfolio) MarkToMarket(bar engine.Bar) {
	equity := p.cash + p.position*bar.Close
	p.equity = append(p.equity, EquityPoint{Time: bar.Time, Equity: equity})
}

func (p *Portfolio) Position() float64     { return p.position }
func (p *Portfolio) Cash() float64         { return p.cash }
func (p *Portfolio) Equity() []EquityPoint { return p.equity }
func (p *Portfolio) Trades() []TradeLog    { return p.trades }
