// Package strategy holds pluggable trading strategies. Each implements
// engine.Strategy — the engine doesn't know which one it's running, so adding a
// new trading system is writing one file here, nothing else.
package strategy

import (
	"time"

	"github.com/leykun/bybit-backtester/internal/engine"
)

// IndicatorPoint is one timestamped indicator value for charting.
type IndicatorPoint struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

// MACrossover goes long when the fast moving average crosses above the slow one
// and flat when it crosses back below. A classic, easy-to-explain trend
// strategy. fast/slow are measured in BARS (not days): on a 5-minute timeframe a
// fast of 10 is a 50-minute average. What the periods mean in wall-clock time
// depends entirely on the chosen timeframe.
type MACrossover struct {
	fast, slow int
	closes     []float64
	prevFast   float64
	prevSlow   float64
	prevReady  bool

	// recorded indicator series for the chart overlay
	fastSeries []IndicatorPoint
	slowSeries []IndicatorPoint
}

func NewMACrossover(fast, slow int) *MACrossover {
	if fast <= 0 {
		fast = 10
	}
	if slow <= fast {
		slow = fast * 3
	}
	return &MACrossover{fast: fast, slow: slow}
}

func (s *MACrossover) Name() string { return "ma_crossover" }

func (s *MACrossover) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	s.closes = append(s.closes, bar.Close)
	if len(s.closes) < s.slow {
		return nil // not enough history yet — stay flat
	}

	fast := sma(s.closes, s.fast)
	slow := sma(s.closes, s.slow)
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

// Indicators exposes the computed series for charting, keyed by a label the UI
// can show in the legend.
func (s *MACrossover) Indicators() map[string][]IndicatorPoint {
	return map[string][]IndicatorPoint{
		fastLabel(s.fast): s.fastSeries,
		slowLabel(s.slow): s.slowSeries,
	}
}

func fastLabel(n int) string { return "Fast MA (" + itoa(n) + ")" }
func slowLabel(n int) string { return "Slow MA (" + itoa(n) + ")" }

// sma returns the simple moving average of the last n values of xs.
func sma(xs []float64, n int) float64 {
	if n <= 0 || len(xs) < n {
		return 0
	}
	sum := 0.0
	for _, v := range xs[len(xs)-n:] {
		sum += v
	}
	return sum / float64(n)
}
