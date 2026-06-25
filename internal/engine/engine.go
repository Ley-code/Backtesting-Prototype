package engine

// The component interfaces. The engine depends only on these — it does not know
// or care which concrete strategy, broker, or data source it is running. Adding
// a new trading system is implementing Strategy in one file; nothing in the
// engine changes.

// DataHandler abstracts WHERE bars come from (Bybit API, cache, CSV, ...).
// Next streams one bar at a time, in chronological order.
type DataHandler interface {
	Next() (Bar, bool)
}

// Strategy receives each new bar and may emit orders. It holds whatever
// indicator state it needs (moving averages, RSI, ...).
type Strategy interface {
	Name() string
	// OnBar sees a price bar and returns zero or more orders. The current
	// position (base units held) and cash (available quote) are passed in so the
	// strategy can size an entry and know whether it is entering, exiting, or
	// flat. Strategies in this prototype go all-in / all-out (one position).
	OnBar(bar Bar, position, cash float64) []Order
}

// Broker simulates execution: applies fees and slippage, decides fill price.
type Broker interface {
	Execute(order Order, bar Bar) Fill
}

// Portfolio tracks cash, position, and the equity curve.
type Portfolio interface {
	OnFill(fill Fill)
	// MarkToMarket records account value at the given bar's close. This is what
	// every metric is computed from.
	MarkToMarket(bar Bar)
	Position() float64
	Cash() float64
}

// Engine wires the components together and runs the event loop.
type Engine struct {
	data     DataHandler
	strategy Strategy
	broker   Broker
	pf       Portfolio
}

func New(data DataHandler, s Strategy, b Broker, pf Portfolio) *Engine {
	return &Engine{data: data, strategy: s, broker: b, pf: pf}
}

// Run replays the historical data through the components one event at a time.
//
// This is the heart of the system. The ordering guarantee: the strategy only
// ever sees a bar when that bar's MarketEvent is popped, and any order it emits
// is filled against the NEXT bar's open — never the bar it was decided on.
//
// This is the crucial anti-look-ahead rule. A strategy decides on bar T using
// data up to and including T's close (the close is only knowable once T is
// over). It therefore cannot trade at T's close — the earliest realistic
// execution is the OPEN of bar T+1. We enforce this by holding orders emitted
// on bar T as "pending" and filling them when bar T+1 arrives. The result: the
// strategy can never act on information from the same instant it trades.
func (e *Engine) Run() {
	var pending []Order // orders decided on the previous bar, awaiting fill

	for {
		bar, ok := e.data.Next()
		if !ok {
			// No more bars: any pending orders cannot be filled (there is no
			// next open to fill them at), so they are dropped. In a long-only,
			// fully-invested run this just means the final equity is marked at
			// the last close, which is correct.
			return
		}

		q := NewEventQueue()
		q.Push(Event{Type: MarketEvent, Time: bar.Time, Bar: bar})

		// Fill orders that were decided on the PREVIOUS bar, against THIS bar's
		// open. They are queued as OrderEvents stamped at this bar's time.
		for _, o := range pending {
			fillOrder := o
			fillOrder.Time = bar.Time
			q.Push(Event{Type: OrderEvent, Time: bar.Time, Order: fillOrder, Bar: bar})
		}
		pending = nil

		// Drain this bar's event chain. Type ordering in the heap ensures
		// Market < Order < Fill at the same timestamp.
		for !q.IsEmpty() {
			ev := q.Pop()
			switch ev.Type {
			case MarketEvent:
				// The strategy decides on this bar's close; its orders are held
				// as pending and will fill on the NEXT bar's open.
				pending = append(pending, e.strategy.OnBar(ev.Bar, e.pf.Position(), e.pf.Cash())...)
			case OrderEvent:
				fill := e.broker.Execute(ev.Order, ev.Bar)
				q.Push(Event{Type: FillEvent, Time: fill.Time, Fill: fill, Bar: ev.Bar})
			case FillEvent:
				e.pf.OnFill(ev.Fill)
			}
		}

		// Record account value at this bar's close — the equity curve.
		e.pf.MarkToMarket(bar)
	}
}
