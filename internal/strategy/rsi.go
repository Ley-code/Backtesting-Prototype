package strategy

import (
	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/leykun/bybit-backtester/internal/indicators"
)

type RSIReversion struct {
	period     int
	oversold   float64
	overbought float64
	rsi        *indicators.RSI
	rsiSeries  []IndicatorPoint
}

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
	return &RSIReversion{
		period: period, oversold: oversold, overbought: overbought,
		rsi: indicators.NewRSI(period),
	}
}

func (s *RSIReversion) Name() string { return "rsi_reversion" }

func (s *RSIReversion) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	rsi, ok := s.rsi.Update(bar.Close)
	if !ok {
		return nil
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

func (s *RSIReversion) Indicators() map[string][]IndicatorPoint {
	return map[string][]IndicatorPoint{
		"RSI (" + itoa(s.period) + ")": s.rsiSeries,
	}
}
