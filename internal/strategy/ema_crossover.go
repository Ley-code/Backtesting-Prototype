package strategy

import (
	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/leykun/bybit-backtester/internal/indicators"
)

type EMACrossover struct {
	fast, slow int
	fastEMA    *indicators.EMA
	slowEMA    *indicators.EMA
	prevFast   float64
	prevSlow   float64
	prevReady  bool

	fastSeries []IndicatorPoint
	slowSeries []IndicatorPoint
}

func NewEMACrossover(fast, slow int) *EMACrossover {
	if fast <= 0 {
		fast = 10
	}
	if slow <= fast {
		slow = fast * 3
	}
	return &EMACrossover{
		fast: fast, slow: slow,
		fastEMA: indicators.NewEMA(fast),
		slowEMA: indicators.NewEMA(slow),
	}
}

func (s *EMACrossover) Name() string { return "ema_crossover" }

func (s *EMACrossover) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	fast, fastOK := s.fastEMA.Update(bar.Close)
	slow, slowOK := s.slowEMA.Update(bar.Close)
	if !fastOK || !slowOK {
		return nil
	}

	s.fastSeries = append(s.fastSeries, IndicatorPoint{Time: bar.Time, Value: fast})
	s.slowSeries = append(s.slowSeries, IndicatorPoint{Time: bar.Time, Value: slow})

	var orders []engine.Order
	if s.prevReady {
		crossedUp := s.prevFast <= s.prevSlow && fast > slow
		crossedDown := s.prevFast >= s.prevSlow && fast < slow

		if crossedUp && position == 0 && cash > 0 {
			orders = append(orders, engine.Order{
				Time: bar.Time, Side: engine.Buy, Qty: cash / bar.Close,
			})
		} else if crossedDown && position > 0 {
			orders = append(orders, engine.Order{
				Time: bar.Time, Side: engine.Sell, Qty: position,
			})
		}
	}

	s.prevFast, s.prevSlow, s.prevReady = fast, slow, true
	return orders
}

func (s *EMACrossover) Indicators() map[string][]IndicatorPoint {
	return map[string][]IndicatorPoint{
		"Fast EMA (" + itoa(s.fast) + ")": s.fastSeries,
		"Slow EMA (" + itoa(s.slow) + ")": s.slowSeries,
	}
}
