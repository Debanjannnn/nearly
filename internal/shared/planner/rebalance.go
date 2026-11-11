package planner

import (
	"context"
	"math/big"
)

// Rebalance restores weights once a leg drifts > DriftBps; values = held priced in base atoms.
// shortcut: routes all moves through base (no same-token netting); net it if swap fees bite.
func (p *Planner) Rebalance(ctx context.Context, b Basket, held, values []string) (Plan, error) {
	if err := p.Validate(b); err != nil {
		return Plan{}, err
	}
	h, err := p.heldFor(b, held)
	if err != nil {
		return Plan{}, err
	}
	v, err := p.heldFor(b, values)
	if err != nil {
		return Plan{}, err
	}
	total := new(big.Int)
	for _, x := range v {
		total.Add(total, x)
	}
	if b.DriftBps == 0 || total.Sign() == 0 || maxDrift(b, v, total) <= int64(b.DriftBps) {
		return Plan{Held: held}, nil
	}

	var steps []Step
	freed := new(big.Int)         // base atoms available for underweight legs
	sell := map[string]*big.Int{} // token -> atoms to sell
	var order []int               // first overweight leg per token, for its pool
	deficit := make([]*big.Int, len(b.Legs))
	for i, l := range b.Legs {
		target := mulDiv(total, int64(l.WeightBps), 10_000)
		deficit[i] = new(big.Int)
		if v[i].Cmp(target) <= 0 {
			deficit[i].Sub(target, v[i])
			continue
		}
		// excess in token atoms = held * (value - target) / value
		take := mulDivBig(h[i], new(big.Int).Sub(v[i], target), v[i])
		if take.Sign() == 0 {
			continue
		}
		st, got := p.take(l, take)
		steps = append(steps, st...)
		h[i].Sub(h[i], take)
		if l.Token == Base {
			freed.Add(freed, got)
			continue
		}
		if got.Sign() == 0 {
			continue
		}
		if sell[l.Token] == nil {
			sell[l.Token], order = new(big.Int), append(order, i)
		}
		sell[l.Token].Add(sell[l.Token], got)
	}
	for _, j := range order {
		l, in := b.Legs[j], sell[b.Legs[j].Token]
		got, err := p.minOut(ctx, l.PoolID, p.id(l.Token), p.id(Base), in)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, swap(l.PoolID, p.id(l.Token), p.id(Base), in, got))
		freed.Add(freed, got)
	}

	bought, err := p.buy(ctx, b.Legs, split(freed, deficit), &steps)
	if err != nil {
		return Plan{}, err
	}
	for i, l := range b.Legs {
		steps = append(steps, p.put(l, bought[i])...)
		h[i].Add(h[i], bought[i])
	}
	return Plan{Steps: steps, Held: strs(h)}, nil
}

// maxDrift is the largest |current bps - target bps| over legs.
func maxDrift(b Basket, values []*big.Int, total *big.Int) int64 {
	var worst int64
	for i, l := range b.Legs {
		d := mulDivBig(values[i], big.NewInt(10_000), total).Int64() - int64(l.WeightBps)
		worst = max(worst, d, -d)
	}
	return worst
}
