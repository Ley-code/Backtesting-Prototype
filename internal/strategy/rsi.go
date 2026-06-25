package strategy

import "github.com/leykun/bybit-backtester/internal/engine"

// RSIReversion is a mean-reversion strategy: buy when RSI falls below an
// oversold threshold, sell when it rises above an overbought threshold. RSI is
// computed with Wilder's smoothing over `period` bars.
type RSIReversion struct {
	period     int
	oversold   float64
	overbought float64

	prevClose float64
	avgGain   float64
	avgLoss   float64
	count     int // number of price changes seen
	seeded    bool

	rsiSeries []IndicatorPoint // recorded for the chart sub-panel
}

// Oversold and Overbought expose the configured thresholds so the UI can draw
// the reference lines on the RSI panel.
func (s *RSIReversion) Oversold() float64   { return s.oversold }
func (s *RSIReversion) Overbought() float64 { return s.overbought }

func NewRSIReversion(period int, oversold, overbought float64) *RSIReversion {
	if period <= 0 {
		period = 14
	}
	if oversold <= 0 {
		oversold = 30
	}
	if overbought <= 0 {
		overbought = 70
	}
	return &RSIReversion{period: period, oversold: oversold, overbought: overbought}
}

func (s *RSIReversion) Name() string { return "rsi_reversion" }

func (s *RSIReversion) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	defer func() { s.prevClose = bar.Close }()

	if s.prevClose == 0 { // first bar, no change to measure yet
		return nil
	}

	change := bar.Close - s.prevClose
	gain, loss := 0.0, 0.0
	if change > 0 {
		gain = change
	} else {
		loss = -change
	}

	s.count++
	if s.count <= s.period {
		// Accumulate the initial average over the first `period` changes.
		s.avgGain += gain
		s.avgLoss += loss
		if s.count == s.period {
			s.avgGain /= float64(s.period)
			s.avgLoss /= float64(s.period)
			s.seeded = true
		}
		return nil
	}

	// Wilder's smoothing.
	s.avgGain = (s.avgGain*float64(s.period-1) + gain) / float64(s.period)
	s.avgLoss = (s.avgLoss*float64(s.period-1) + loss) / float64(s.period)

	if !s.seeded {
		return nil
	}

	rsi := 100.0
	if s.avgLoss != 0 {
		rs := s.avgGain / s.avgLoss
		rsi = 100 - (100 / (1 + rs))
	}
	s.rsiSeries = append(s.rsiSeries, IndicatorPoint{Time: bar.Time, Value: rsi})

	var orders []engine.Order
	if rsi < s.oversold && position == 0 && cash > 0 {
		orders = append(orders, engine.Order{
			Time: bar.Time, Side: engine.Buy, Qty: cash / bar.Close,
		})
	} else if rsi > s.overbought && position > 0 {
		orders = append(orders, engine.Order{
			Time: bar.Time, Side: engine.Sell, Qty: position,
		})
	}
	return orders
}

// Indicators exposes the RSI series for the chart sub-panel.
func (s *RSIReversion) Indicators() map[string][]IndicatorPoint {
	return map[string][]IndicatorPoint{
		"RSI (" + itoa(s.period) + ")": s.rsiSeries,
	}
}
