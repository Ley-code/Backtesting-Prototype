// Package engine contains the event-driven backtest core.
//
// Everything flows through a strictly time-ordered event queue. The engine
// loop pops the earliest event, routes it to a component, and pushes any new
// events that component emits. A strategy positioned at timestamp T can never
// be handed data stamped after T — look-ahead bias is prevented mechanically
// by the queue ordering, not by discipline.
package engine

import "time"

// EventType identifies what kind of event is flowing through the queue.
type EventType int

const (
	// MarketEvent: a new price bar arrived. Drives the strategy.
	MarketEvent EventType = iota
	// OrderEvent: a strategy decided to buy or sell. Drives the broker.
	OrderEvent
	// FillEvent: the broker executed an order. Drives the portfolio.
	FillEvent
)

// Side is the direction of an order/fill.
type Side int

const (
	Buy Side = iota
	Sell
)

func (s Side) String() string {
	if s == Buy {
		return "BUY"
	}
	return "SELL"
}

// Bar is a single OHLCV candle — the standard shape of historical price data.
type Bar struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

// Order is an instruction emitted by a strategy. Quantity is in base units
// (e.g. BTC). For this prototype we use market orders only.
type Order struct {
	Time time.Time
	Side Side
	Qty  float64
}

// Fill is a broker-executed order with the realized price (after slippage)
// and the fee charged.
type Fill struct {
	Time  time.Time
	Side  Side
	Qty   float64
	Price float64
	Fee   float64
}

// Event is the unit that flows through the queue. Only the fields relevant to
// the Type are populated. Time is the ordering key for the whole system.
type Event struct {
	Type  EventType
	Time  time.Time
	Bar   Bar
	Order Order
	Fill  Fill
}
