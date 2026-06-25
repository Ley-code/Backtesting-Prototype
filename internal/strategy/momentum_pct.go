package strategy

import (
	"github.com/leykun/bybit-backtester/internal/engine"
	"github.com/leykun/bybit-backtester/internal/indicators"
)

type MomentumPct struct {
	lookback int
	buyPct   float64
	sellPct  float64
	roller   *indicators.RollingMinMax

	peakSinceEntry float64
}

func NewMomentumPct(lookback, buyPct, sellPct int) *MomentumPct {
	if lookback <= 0 {
		lookback = 20
	}
	if buyPct <= 0 {
		buyPct = 10
	}
	if sellPct <= 0 {
		sellPct = 5
	}
	return &MomentumPct{
		lookback: lookback,
		buyPct:   float64(buyPct),
		sellPct:  float64(sellPct),
		roller:   indicators.NewRollingMinMax(lookback),
	}
}

func (s *MomentumPct) Name() string { return "momentum_pct" }

func (s *MomentumPct) OnBar(bar engine.Bar, position, cash float64) []engine.Order {
	recentLow, _, ok := s.roller.Update(bar.Close)
	if !ok {
		return nil
	}

	var orders []engine.Order
	if position == 0 {
		s.peakSinceEntry = 0
		threshold := recentLow * (1 + s.buyPct/100)
		if bar.Close >= threshold && cash > 0 {
			orders = append(orders, engine.Order{
				Time: bar.Time, Side: engine.Buy, Qty: cash / bar.Close,
			})
		}
	} else {
		if s.peakSinceEntry == 0 || bar.Close > s.peakSinceEntry {
			s.peakSinceEntry = bar.Close
		}
		exitLevel := s.peakSinceEntry * (1 - s.sellPct/100)
		if bar.Close <= exitLevel {
			orders = append(orders, engine.Order{
				Time: bar.Time, Side: engine.Sell, Qty: position,
			})
			s.peakSinceEntry = 0
		}
	}
	return orders
}
