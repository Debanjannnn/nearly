// Package backtest simulates a basket day by day over historical series. Pure: no I/O.
package backtest

import (
	"math"
	"strings"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

const Day = "2006-01-02"

type Input struct {
	Basket   planner.Basket
	Amount   float64 // USD, deposited in the base token
	From, To time.Time
	SwapBps  float64 // Rhea pool fee + slippage per swap leg
}

// Series is keyed by day "2006-01-02".
type Series struct {
	Price map[string]map[string]float64 // token symbol -> USD; base token absent (= 1)
	APY   map[string]map[string]float64 // "lending:<token>" | "staking:wnear" -> apy fraction
}

type Coverage struct {
	Series string `json:"series"`
	First  string `json:"first"`
	Last   string `json:"last"`
	Days   int    `json:"days"`
	Note   string `json:"note,omitempty"`
}

type Point struct {
	Day string  `json:"day"`
	USD float64 `json:"usd"`
}

type Result struct {
	From          string             `json:"from"` // effective window after clamping to data coverage
	To            string             `json:"to"`
	Days          int                `json:"days"`
	Coverage      []Coverage         `json:"coverage"`
	InitialUSD    float64            `json:"initial_usd"`
	FinalUSD      float64            `json:"final_usd"`
	APY           float64            `json:"apy"`
	MaxDrawdown   float64            `json:"max_drawdown"` // 0.2 = -20% peak to trough
	Rebalances    int                `json:"rebalances"`
	CostsUSD      float64            `json:"costs_usd"`     // open fee + swaps
	Contributions map[string]float64 `json:"contributions"` // per leg yield, plus "price" and "costs"; sums to final-initial
	Curve         []Point            `json:"curve"`
}

type Error struct{ Code, Msg string }

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

func legKey(l planner.Leg) string { return l.Source + ":" + l.Token }

func span(m map[string]float64) (first, last string, n int) {
	for d := range m {
		if first == "" || d < first {
			first = d
		}
		if d > last {
			last = d
		}
	}
	return first, last, len(m)
}

// dense forward-fills m over n days from start, seeded with the last value on or before start (NaN before any data).
func dense(m map[string]float64, start time.Time, n int) []float64 {
	out, cur, seedDay, s0 := make([]float64, n), math.NaN(), "", start.Format(Day)
	for d, v := range m {
		if d <= s0 && d > seedDay {
			seedDay, cur = d, v
		}
	}
	for i := range out {
		if v, ok := m[start.AddDate(0, 0, i).Format(Day)]; ok {
			cur = v
		}
		out[i] = cur
	}
	return out
}

// Run validates nothing about tokens: callers check the basket with planner.Validate first.
func Run(in Input, s Series) (*Result, error) {
	// NaN/Inf or a cost >= 100% would make the result unencodable as JSON
	if !(in.Amount > 0) || math.IsInf(in.Amount, 0) || in.From.After(in.To) || !(in.SwapBps >= 0 && in.SwapBps < 1e4) {
		return nil, &Error{"bad_request", "amount must be finite > 0, from <= to, 0 <= swap_bps < 10000"}
	}
	legs := in.Basket.Legs
	type req struct {
		name string
		m    map[string]float64
	}
	var reqs []req
	seen := map[string]bool{}
	for _, l := range legs {
		if l.Token != planner.Base && !seen["price:"+l.Token] {
			seen["price:"+l.Token] = true
			reqs = append(reqs, req{"price:" + l.Token, s.Price[l.Token]})
		}
		if l.Source != planner.SourceHold {
			reqs = append(reqs, req{"apy:" + legKey(l), s.APY[legKey(l)]})
		}
	}
	from, to := in.From.Format(Day), in.To.Format(Day)
	res := &Result{Contributions: map[string]float64{"price": 0, "costs": 0}}
	for _, r := range reqs {
		first, last, n := span(r.m)
		if n == 0 {
			return nil, &Error{"no_data", "no data for " + r.name}
		}
		c := Coverage{Series: r.name, First: first, Last: last, Days: n}
		if first > from {
			from, c.Note = first, "starts after requested window; window clamped"
		}
		if last < to {
			to, c.Note = last, strings.TrimPrefix(c.Note+"; ends before requested window; window clamped", "; ")
		}
		res.Coverage = append(res.Coverage, c)
	}
	if from > to {
		return nil, &Error{"no_data", "series do not overlap the requested window"}
	}
	start, _ := time.Parse(Day, from)
	end, _ := time.Parse(Day, to)
	N := int(end.Sub(start).Hours()/24) + 1
	res.From, res.To, res.Days = from, to, N-1

	price, apy := make([][]float64, len(legs)), make([][]float64, len(legs))
	target := make([]float64, len(legs))
	for i, l := range legs {
		target[i] = float64(l.WeightBps) / 1e4
		if l.Token != planner.Base {
			price[i] = dense(s.Price[l.Token], start, N)
		}
		if l.Source != planner.SourceHold {
			apy[i] = dense(s.APY[legKey(l)], start, N)
		}
	}
	px := func(i, d int) float64 {
		if price[i] == nil {
			return 1
		}
		return price[i][d]
	}
	daily := func(annual float64) float64 { return math.Pow(1+annual, 1.0/365) - 1 }
	pay := func(c float64) { res.CostsUSD += c; res.Contributions["costs"] -= c }

	// open: fee on the deposit, then a swap for every non-base leg
	pos := make([]float64, len(legs))
	fee := in.Amount * planner.FeeBps / 1e4
	pay(fee)
	for i, l := range legs {
		pos[i] = (in.Amount - fee) * target[i]
		if l.Token != planner.Base {
			c := pos[i] * in.SwapBps / 1e4
			pos[i] -= c
			pay(c)
		}
	}
	total := func() float64 {
		t := 0.0
		for _, v := range pos {
			t += v
		}
		return t
	}
	// rebalance pays a swap on the moved notional of every non-base leg
	rebalance := func() {
		t, cost := total(), 0.0
		for i, l := range legs {
			if l.Token != planner.Base {
				cost += math.Abs(target[i]*t-pos[i]) * in.SwapBps / 1e4
			}
		}
		pay(cost)
		for i := range pos {
			pos[i] = target[i] * (t - cost)
		}
	}

	res.InitialUSD = in.Amount
	for d := 0; d < N; d++ {
		if d > 0 {
			for i, l := range legs {
				v1 := pos[i] * px(i, d) / px(i, d-1)
				res.Contributions["price"] += v1 - pos[i]
				if a := apyAt(apy[i], d); a != 0 {
					y := v1 * daily(a)
					res.Contributions[legKey(l)] += y
					v1 += y
				}
				pos[i] = v1
			}
			if band := float64(in.Basket.DriftBps) / 1e4; band > 0 {
				t, drift := total(), 0.0
				for i := range pos {
					drift = math.Max(drift, math.Abs(pos[i]/t-target[i]))
				}
				if drift > band {
					rebalance()
					res.Rebalances++
				}
			}
		}
		res.Curve = append(res.Curve, Point{Day: start.AddDate(0, 0, d).Format(Day), USD: total()})
	}
	res.FinalUSD = total()
	if N > 1 {
		res.APY = math.Pow(res.FinalUSD/res.InitialUSD, 365/float64(N-1)) - 1
	}
	res.MaxDrawdown = maxDD(res.Curve)
	return res, nil
}

func apyAt(s []float64, d int) float64 {
	if s == nil || math.IsNaN(s[d]) {
		return 0
	}
	return s[d]
}

func maxDD(c []Point) float64 {
	peak, dd := math.Inf(-1), 0.0
	for _, p := range c {
		peak = math.Max(peak, p.USD)
		dd = math.Max(dd, 1-p.USD/peak)
	}
	return dd
}
