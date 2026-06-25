package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/leykun/bybit-backtester/internal/portfolio"
)

func eqCurve(vals ...float64) []portfolio.EquityPoint {
	pts := make([]portfolio.EquityPoint, len(vals))
	t := time.Unix(0, 0)
	for i, v := range vals {
		pts[i] = portfolio.EquityPoint{Time: t.Add(time.Duration(i) * time.Minute), Equity: v}
	}
	return pts
}

func TestMaxDrawdown(t *testing.T) {
	// Peak 120, trough 90 → drawdown = 30/120 = 0.25.
	eq := eqCurve(100, 120, 90, 110)
	got := maxDrawdown(eq)
	want := 0.25
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("maxDrawdown = %v, want %v", got, want)
	}
}

func TestMaxDrawdownMonotonic(t *testing.T) {
	// Strictly rising curve → no drawdown.
	if got := maxDrawdown(eqCurve(100, 101, 102, 103)); got != 0 {
		t.Fatalf("maxDrawdown on rising curve = %v, want 0", got)
	}
}

func TestSharpeZeroVariance(t *testing.T) {
	// Constant returns → std dev 0 → Sharpe defined as 0 (no risk info).
	if got := sharpe([]float64{0.01, 0.01, 0.01}, 525600); got != 0 {
		t.Fatalf("sharpe with zero variance = %v, want 0", got)
	}
}

func TestSharpePositive(t *testing.T) {
	// Mostly positive, low-variance returns should yield a positive Sharpe.
	r := []float64{0.01, 0.02, 0.015, 0.005, 0.012}
	if got := sharpe(r, 525600); got <= 0 {
		t.Fatalf("sharpe = %v, want > 0", got)
	}
}

func TestComputeReturnsAndTrades(t *testing.T) {
	res := Compute(eqCurve(1000, 1100), 4, "1")
	if math.Abs(res.TotalReturn-0.1) > 1e-9 {
		t.Fatalf("TotalReturn = %v, want 0.1", res.TotalReturn)
	}
	if res.NumTrades != 4 {
		t.Fatalf("NumTrades = %d, want 4", res.NumTrades)
	}
	if res.FinalEquity != 1100 {
		t.Fatalf("FinalEquity = %v, want 1100", res.FinalEquity)
	}
}
