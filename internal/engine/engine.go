package engine

type DataHandler interface {
	Next() (Bar, bool)
}

type Strategy interface {
	Name() string
	OnBar(bar Bar, position, cash float64) []Order
}

type Broker interface {
	Execute(order Order, bar Bar) Fill
}

type Portfolio interface {
	OnFill(fill Fill)
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
			return
		}

		q := NewEventQueue()
		q.Push(Event{Type: MarketEvent, Time: bar.Time, Bar: bar})

		for _, o := range pending {
			fillOrder := o
			fillOrder.Time = bar.Time
			q.Push(Event{Type: OrderEvent, Time: bar.Time, Order: fillOrder, Bar: bar})
		}
		pending = nil

		for !q.IsEmpty() {
			ev := q.Pop()
			switch ev.Type {
			case MarketEvent:
				pending = append(pending, e.strategy.OnBar(ev.Bar, e.pf.Position(), e.pf.Cash())...)
			case OrderEvent:
				fill := e.broker.Execute(ev.Order, ev.Bar)
				q.Push(Event{Type: FillEvent, Time: fill.Time, Fill: fill, Bar: ev.Bar})
			case FillEvent:
				e.pf.OnFill(ev.Fill)
			}
		}

		e.pf.MarkToMarket(bar)
	}
}
