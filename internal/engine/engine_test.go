package engine

import (
	"testing"
	"time"
)

// --- test doubles ---

// sliceData is a DataHandler over a fixed slice of bars.
type sliceData struct {
	bars []Bar
	i    int
}

func (d *sliceData) Next() (Bar, bool) {
	if d.i >= len(d.bars) {
		return Bar{}, false
	}
	b := d.bars[d.i]
	d.i++
	return b, true
}

// buyOnceStrategy emits a single buy when it first sees the bar at buyIndex.
type buyOnceStrategy struct {
	seen     int
	buyIndex int
	bought   bool
}

func (s *buyOnceStrategy) Name() string { return "buy_once" }
func (s *buyOnceStrategy) OnBar(bar Bar, position, cash float64) []Order {
	idx := s.seen
	s.seen++
	if idx == s.buyIndex && !s.bought {
		s.bought = true
		return []Order{{Time: bar.Time, Side: Buy, Qty: 1}}
	}
	return nil
}

// openFillBroker fills at the bar's open with no fees/slippage, so we can assert
// the exact fill price.
type openFillBroker struct{}

func (openFillBroker) Execute(o Order, bar Bar) Fill {
	return Fill{Time: o.Time, Side: o.Side, Qty: o.Qty, Price: bar.Open}
}

// recordingPortfolio captures the fills it receives.
type recordingPortfolio struct {
	fills []Fill
	pos   float64
}

func (p *recordingPortfolio) OnFill(f Fill) {
	p.fills = append(p.fills, f)
	if f.Side == Buy {
		p.pos += f.Qty
	} else {
		p.pos -= f.Qty
	}
}
func (p *recordingPortfolio) MarkToMarket(Bar)  {}
func (p *recordingPortfolio) Position() float64 { return p.pos }
func (p *recordingPortfolio) Cash() float64     { return 1e9 }

// TestNoLookAhead proves the core guarantee: a strategy that decides to buy on
// bar T is filled at bar T+1's OPEN — never at bar T's close (which it used to
// decide) and never at a price from the same bar.
func TestNoLookAhead(t *testing.T) {
	t0 := time.Unix(0, 0).UTC()
	min := time.Minute
	bars := []Bar{
		{Time: t0, Open: 100, High: 105, Low: 99, Close: 104},        // bar 0 — decide here
		{Time: t0.Add(min), Open: 110, High: 112, Low: 108, Close: 111}, // bar 1 — fill at THIS open (110)
		{Time: t0.Add(2 * min), Open: 120, High: 121, Low: 118, Close: 119},
	}

	pf := &recordingPortfolio{}
	eng := New(&sliceData{bars: bars}, &buyOnceStrategy{buyIndex: 0}, openFillBroker{}, pf)
	eng.Run()

	if len(pf.fills) != 1 {
		t.Fatalf("expected exactly 1 fill, got %d", len(pf.fills))
	}
	got := pf.fills[0]

	// The decisive assertion: filled at bar 1's open (110), NOT bar 0's close (104).
	if got.Price != 110 {
		t.Fatalf("fill price = %v; want 110 (next bar's open). Got 104 would mean look-ahead at the decision bar's close.", got.Price)
	}
	// And the fill is stamped at bar 1's time, not bar 0's.
	if !got.Time.Equal(t0.Add(min)) {
		t.Fatalf("fill time = %v; want bar 1's time %v", got.Time, t0.Add(min))
	}
}

// TestOrderOnLastBarIsDropped: an order decided on the final bar has no next bar
// to fill against, so it is never executed (we can't invent a future price).
func TestOrderOnLastBarIsDropped(t *testing.T) {
	t0 := time.Unix(0, 0).UTC()
	bars := []Bar{
		{Time: t0, Open: 100, Close: 104},
		{Time: t0.Add(time.Minute), Open: 110, Close: 111},
	}
	pf := &recordingPortfolio{}
	// Buy on the LAST bar (index 1) — there is no bar 2 to fill at.
	eng := New(&sliceData{bars: bars}, &buyOnceStrategy{buyIndex: 1}, openFillBroker{}, pf)
	eng.Run()

	if len(pf.fills) != 0 {
		t.Fatalf("expected 0 fills (order on last bar cannot fill), got %d", len(pf.fills))
	}
}
