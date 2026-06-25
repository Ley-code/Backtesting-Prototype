package indicators

import "math"

func SMA(xs []float64, n int) float64 {
	if n <= 0 || len(xs) < n {
		return 0
	}
	sum := 0.0
	for _, v := range xs[len(xs)-n:] {
		sum += v
	}
	return sum / float64(n)
}

type EMA struct {
	period int
	value  float64
	ready  bool
	count  int
	sum    float64
}

func NewEMA(period int) *EMA {
	return &EMA{period: period}
}

func (e *EMA) Update(x float64) (float64, bool) {
	e.count++
	if e.count < e.period {
		e.sum += x
		return 0, false
	}
	if e.count == e.period {
		e.value = e.sum / float64(e.period)
		e.ready = true
		return e.value, true
	}
	k := 2.0 / float64(e.period+1)
	e.value = x*k + e.value*(1-k)
	return e.value, true
}

type RSI struct {
	period    int
	prevClose float64
	avgGain   float64
	avgLoss   float64
	count     int
	seeded    bool
}

func NewRSI(period int) *RSI {
	return &RSI{period: period}
}

func (r *RSI) Update(close float64) (float64, bool) {
	if r.prevClose == 0 {
		r.prevClose = close
		return 0, false
	}

	change := close - r.prevClose
	r.prevClose = close

	gain, loss := 0.0, 0.0
	if change > 0 {
		gain = change
	} else {
		loss = -change
	}

	r.count++
	if r.count <= r.period {
		r.avgGain += gain
		r.avgLoss += loss
		if r.count == r.period {
			r.avgGain /= float64(r.period)
			r.avgLoss /= float64(r.period)
			r.seeded = true
		}
		return 0, false
	}

	r.avgGain = (r.avgGain*float64(r.period-1) + gain) / float64(r.period)
	r.avgLoss = (r.avgLoss*float64(r.period-1) + loss) / float64(r.period)
	if !r.seeded {
		return 0, false
	}

	rsi := 100.0
	if r.avgLoss != 0 {
		rs := r.avgGain / r.avgLoss
		rsi = 100 - (100 / (1 + rs))
	}
	return rsi, true
}

type Bollinger struct {
	period  int
	stdMult float64
	closes  []float64
}

func NewBollinger(period int, stdMult float64) *Bollinger {
	return &Bollinger{period: period, stdMult: stdMult}
}

func (b *Bollinger) Update(close float64) (middle, upper, lower float64, ready bool) {
	b.closes = append(b.closes, close)
	if len(b.closes) < b.period {
		return 0, 0, 0, false
	}
	window := b.closes[len(b.closes)-b.period:]
	middle = SMA(window, b.period)
	variance := 0.0
	for _, v := range window {
		d := v - middle
		variance += d * d
	}
	std := math.Sqrt(variance / float64(b.period))
	upper = middle + b.stdMult*std
	lower = middle - b.stdMult*std
	return middle, upper, lower, true
}

type Donchian struct {
	lookback int
	highs    []float64
	lows     []float64
}

func NewDonchian(lookback int) *Donchian {
	return &Donchian{lookback: lookback}
}

func (d *Donchian) Update(high, low float64) (channelHigh, channelLow float64, ready bool) {
	d.highs = append(d.highs, high)
	d.lows = append(d.lows, low)
	if len(d.highs) <= d.lookback {
		return 0, 0, false
	}
	windowH := d.highs[len(d.highs)-d.lookback-1 : len(d.highs)-1]
	windowL := d.lows[len(d.lows)-d.lookback-1 : len(d.lows)-1]
	channelHigh = windowH[0]
	channelLow = windowL[0]
	for _, v := range windowH[1:] {
		if v > channelHigh {
			channelHigh = v
		}
	}
	for _, v := range windowL[1:] {
		if v < channelLow {
			channelLow = v
		}
	}
	return channelHigh, channelLow, true
}

type RollingMinMax struct {
	lookback int
	values   []float64
}

func NewRollingMinMax(lookback int) *RollingMinMax {
	return &RollingMinMax{lookback: lookback}
}

func (r *RollingMinMax) Update(x float64) (min, max float64, ready bool) {
	r.values = append(r.values, x)
	if len(r.values) < r.lookback {
		return 0, 0, false
	}
	window := r.values[len(r.values)-r.lookback:]
	min, max = window[0], window[0]
	for _, v := range window[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max, true
}
