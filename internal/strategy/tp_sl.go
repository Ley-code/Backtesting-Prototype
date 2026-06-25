package strategy

import (
	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/leykun/bybit-backtester/internal/indicators"
)

type TakeProfitStopLoss struct {
	fast, slow int
	tpPct      float64
	slPct      float64
	closes     []float64
	prevFast   float64
	prevSlow   float64
	prevReady  bool
	entryPrice float64

	fastSeries []IndicatorPoint
	slowSeries []IndicatorPoint
}

func NewTakeProfitStopLoss(fast, slow, tpPct, slPct int) *TakeProfitStopLoss {
	if fast <= 0 {
		fast = 10
	}
	if slow <= fast {
		slow = fast * 3
	}
	if tpPct <= 0 {
		tpPct = 10
	}
	if slPct <= 0 {
		slPct = 5
	}
	return &TakeProfitStopLoss{
		fast: fast, slow: slow,
		tpPct: float64(tpPct), slPct: float64(slPct),
	}
}

func (s *TakeProfitStopLoss) Name() string { return "tp_sl" }

func (s *TakeProfitStopLoss) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	s.closes = append(s.closes, bar.Close)
	if len(s.closes) < s.slow {
		return nil
	}

	fast := indicators.SMA(s.closes, s.fast)
	slow := indicators.SMA(s.closes, s.slow)
	s.fastSeries = append(s.fastSeries, IndicatorPoint{Time: bar.Time, Value: fast})
	s.slowSeries = append(s.slowSeries, IndicatorPoint{Time: bar.Time, Value: slow})

	var orders []engine.Order

	if position > 0 {
		if s.entryPrice == 0 {
			s.entryPrice = bar.Close
		}
		tpLevel := s.entryPrice * (1 + s.tpPct/100)
		slLevel := s.entryPrice * (1 - s.slPct/100)
		if bar.Close >= tpLevel || bar.Close <= slLevel {
			orders = append(orders, engine.Order{
				Time: bar.Time, Side: engine.Sell, Qty: position,
			})
			s.entryPrice = 0
			s.prevFast, s.prevSlow, s.prevReady = fast, slow, true
			return orders
		}
	} else {
		s.entryPrice = 0
		if s.prevReady {
			crossedUp := s.prevFast <= s.prevSlow && fast > slow
			if crossedUp && cash > 0 {
				orders = append(orders, engine.Order{
					Time: bar.Time, Side: engine.Buy, Qty: cash / bar.Close,
				})
			}
		}
	}

	s.prevFast, s.prevSlow, s.prevReady = fast, slow, true
	return orders
}

func (s *TakeProfitStopLoss) Indicators() map[string][]IndicatorPoint {
	return map[string][]IndicatorPoint{
		"Fast MA (" + itoa(s.fast) + ")": s.fastSeries,
		"Slow MA (" + itoa(s.slow) + ")": s.slowSeries,
	}
}
