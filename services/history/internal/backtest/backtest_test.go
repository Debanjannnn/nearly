package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

func d(s string) time.Time { t, _ := time.Parse(Day, s); return t }

// flat fills a series with v for n days from start.
func flat(start string, n int, v func(i int) float64) map[string]float64 {
	m := map[string]float64{}
	for i := range n {
		m[d(start).AddDate(0, 0, i).Format(Day)] = v(i)
	}
	return m
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func sumContrib(r *Result) float64 {
	s := 0.0
	for _, v := range r.Contributions {
		s += v
	}
	return s
}

func TestLendingOnlyEarnsAPYMinusFee(t *testing.T) {
	s := Series{APY: map[string]map[string]float64{"lending:usdc": flat("2026-01-01", 366, func(int) float64 { return 0.10 })}}
	in := Input{Basket: planner.Basket{Legs: []planner.Leg{{Token: "usdc", Source: "lending", WeightBps: 10_000}}},
		Amount: 1000, From: d("2026-01-01"), To: d("2027-01-01"), SwapBps: 30}
	r, err := Run(in, s)
	if err != nil {
		t.Fatal(err)
	}
	if r.Days != 365 || r.To != "2027-01-01" {
		t.Fatalf("window %s..%s days %d", r.From, r.To, r.Days)
	}
	// 1000 - 0.4% fee, then 10% for a year
	if want := 996 * 1.10; !near(r.FinalUSD, want, 0.01) {
		t.Fatalf("final %v want %v", r.FinalUSD, want)
	}
	if r.MaxDrawdown > 0.0041 || r.CostsUSD != 4 || r.Rebalances != 0 {
		t.Fatalf("dd %v costs %v reb %d", r.MaxDrawdown, r.CostsUSD, r.Rebalances)
	}
	if !near(sumContrib(r), r.FinalUSD-r.InitialUSD, 1e-6) {
		t.Fatalf("contributions %v != pnl %v", sumContrib(r), r.FinalUSD-r.InitialUSD)
	}
}

func TestPriceDrawdownAndRebalance(t *testing.T) {
	// wnear halves over 10 days, then recovers to start
	px := flat("2026-03-01", 21, func(i int) float64 {
		if i <= 10 {
			return 4 - 0.2*float64(i)
		}
		return 2 + 0.2*float64(i-10)
	})
	s := Series{
		Price: map[string]map[string]float64{"wnear": px},
		APY: map[string]map[string]float64{
			"staking:wnear": flat("2026-03-01", 21, func(int) float64 { return 0.08 }),
		},
	}
	b := planner.Basket{DriftBps: 500, Legs: []planner.Leg{
		{Token: "usdc", Source: "hold", WeightBps: 5000},
		{Token: "wnear", Source: "staking", WeightBps: 5000},
	}}
	r, err := Run(Input{Basket: b, Amount: 1000, From: d("2026-02-01"), To: d("2026-04-01"), SwapBps: 30}, s)
	if err != nil {
		t.Fatal(err)
	}
	if r.From != "2026-03-01" || r.To != "2026-03-21" || len(r.Curve) != 21 {
		t.Fatalf("clamp %s..%s curve %d", r.From, r.To, len(r.Curve))
	}
	if len(r.Coverage) != 2 || r.Coverage[0].Note == "" {
		t.Fatalf("coverage %+v", r.Coverage)
	}
	if r.Rebalances == 0 {
		t.Fatal("drift band should trigger rebalances")
	}
	if r.MaxDrawdown < 0.2 || r.MaxDrawdown > 0.3 {
		t.Fatalf("drawdown %v", r.MaxDrawdown)
	}
	if !near(sumContrib(r), r.FinalUSD-r.InitialUSD, 1e-6) {
		t.Fatalf("contributions %v != pnl %v", sumContrib(r), r.FinalUSD-r.InitialUSD)
	}
	if r.Contributions["staking:wnear"] <= 0 {
		t.Fatal("staking yield missing")
	}
}

func TestErrors(t *testing.T) {
	b := planner.Basket{Legs: []planner.Leg{{Token: "wnear", Source: "lending", WeightBps: 10_000}}}
	in := Input{Basket: b, Amount: 1, From: d("2026-01-01"), To: d("2026-02-01")}
	if _, err := Run(in, Series{}); err == nil || err.(*Error).Code != "no_data" {
		t.Fatalf("want no_data, got %v", err)
	}
	for _, bad := range []Input{{Amount: 0}, {Amount: math.Inf(1)}, {Amount: 1, SwapBps: 1e4}} {
		bad.Basket, bad.From, bad.To = b, in.From, in.To
		if _, err := Run(bad, Series{}); err == nil || err.(*Error).Code != "bad_request" {
			t.Fatalf("%+v: want bad_request, got %v", bad, err)
		}
	}
}
