package strategy

import (
	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/leykun/bybit-backtester/internal/indicators"
)

type BollingerBounce struct {
	period  int
	stdMult float64
	bb      *indicators.Bollinger

	midSeries   []IndicatorPoint
	upperSeries []IndicatorPoint
	lowerSeries []IndicatorPoint
}

func NewBollingerBounce(period int, stdMult float64) *BollingerBounce {
	if period <= 0 {
		period = 20
	}
	if stdMult <= 0 {
		stdMult = 2.0
	}
	return &BollingerBounce{
		period:  period,
		stdMult: stdMult,
		bb:      indicators.NewBollinger(period, stdMult),
	}
}

func (s *BollingerBounce) Name() string { return "bollinger_bounce" }

func (s *BollingerBounce) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	mid, upper, lower, ok := s.bb.Update(bar.Close)
	if !ok {
		return nil
	}

	s.midSeries = append(s.midSeries, IndicatorPoint{Time: bar.Time, Value: mid})
	s.upperSeries = append(s.upperSeries, IndicatorPoint{Time: bar.Time, Value: upper})
	s.lowerSeries = append(s.lowerSeries, IndicatorPoint{Time: bar.Time, Value: lower})

	var orders []engine.Order
	if bar.Close <= lower && position == 0 && cash > 0 {
		orders = append(orders, engine.Order{
			Time: bar.Time, Side: engine.Buy, Qty: cash / bar.Close,
		})
	} else if bar.Close >= upper && position > 0 {
		orders = append(orders, engine.Order{
			Time: bar.Time, Side: engine.Sell, Qty: position,
		})
	}
	return orders
}

func (s *BollingerBounce) Indicators() map[string][]IndicatorPoint {
	return map[string][]IndicatorPoint{
		"BB Middle (" + itoa(s.period) + ")": s.midSeries,
		"BB Upper":                           s.upperSeries,
		"BB Lower":                           s.lowerSeries,
	}
}
