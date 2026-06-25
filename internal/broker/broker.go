package broker

import "github.com/leykun/bybit-backtester/internal/engine"

type Simulated struct {
	feeRate      float64
	slippageRate float64
}

func NewSimulated(feeRate, slippageRate float64) *Simulated {
	if feeRate <= 0 {
		feeRate = 0.0006
	}
	if slippageRate < 0 {
		slippageRate = 0.0002
	}
	return &Simulated{feeRate: feeRate, slippageRate: slippageRate}
}

func (b *Simulated) Execute(o engine.Order, bar engine.Bar) engine.Fill {
	price := bar.Open
	if o.Side == engine.Buy {
		price *= (1 + b.slippageRate)
	} else {
		price *= (1 - b.slippageRate)
	}

	fee := price * o.Qty * b.feeRate
	return engine.Fill{
		Time:  o.Time,
		Side:  o.Side,
		Qty:   o.Qty,
		Price: price,
		Fee:   fee,
	}
}
