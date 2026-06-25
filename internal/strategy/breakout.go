package strategy

import (
	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/leykun/bybit-backtester/internal/indicators"
)

type Breakout struct {
	lookback   int
	donchian   *indicators.Donchian
	highSeries []IndicatorPoint
	lowSeries  []IndicatorPoint
}

func NewBreakout(lookback int) *Breakout {
	if lookback <= 0 {
		lookback = 20
	}
	return &Breakout{
		lookback: lookback,
		donchian: indicators.NewDonchian(lookback),
	}
}

func (s *Breakout) Name() string { return "breakout" }

func (s *Breakout) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	channelHigh, channelLow, ok := s.donchian.Update(bar.High, bar.Low)
	if !ok {
		return nil
	}

	s.highSeries = append(s.highSeries, IndicatorPoint{Time: bar.Time, Value: channelHigh})
	s.lowSeries = append(s.lowSeries, IndicatorPoint{Time: bar.Time, Value: channelLow})

	var orders []engine.Order
	if bar.Close > channelHigh && position == 0 && cash > 0 {
		orders = append(orders, engine.Order{
			Time: bar.Time, Side: engine.Buy, Qty: cash / bar.Close,
		})
	} else if bar.Close < channelLow && position > 0 {
		orders = append(orders, engine.Order{
			Time: bar.Time, Side: engine.Sell, Qty: position,
		})
	}
	return orders
}

func (s *Breakout) Indicators() map[string][]IndicatorPoint {
	return map[string][]IndicatorPoint{
		"Donchian High (" + itoa(s.lookback) + ")": s.highSeries,
		"Donchian Low (" + itoa(s.lookback) + ")":  s.lowSeries,
	}
}
