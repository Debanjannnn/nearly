package planner

import (
	"context"
	"math/big"
)

// Deploy opens a position with amount base atoms: fee to treasury first, one swap per non-base
// token (Rhea pool), then supply/stake per leg.
func (p *Planner) Deploy(ctx context.Context, b Basket, amount string) (Plan, error) {
	if err := p.Validate(b); err != nil {
		return Plan{}, err
	}
	total, err := amt(amount)
	if err != nil {
		return Plan{}, err
	}
	fee := mulDiv(total, FeeBps, 10_000)
	net := new(big.Int).Sub(total, fee)
	if net.Sign() <= 0 {
		return Plan{}, fail("bad_request", "amount too small")
	}
	var steps []Step
	if fee.Sign() > 0 {
		steps = append(steps, tokenStep(KindFee, p.id(Base), fee))
	}
	ws := make([]*big.Int, len(b.Legs))
	for i, l := range b.Legs {
		ws[i] = big.NewInt(int64(l.WeightBps))
	}
	held, err := p.buy(ctx, b.Legs, split(net, ws), &steps)
	if err != nil {
		return Plan{}, err
	}
	for i, l := range b.Legs {
		steps = append(steps, p.put(l, held[i])...)
	}
	return Plan{Steps: steps, Fee: fee.String(), Held: strs(held)}, nil
}

// buy converts per-leg base amounts into leg tokens: one swap per token, output split back
// over that token's legs by their base input. Appends swaps to steps; returns token amounts.
func (p *Planner) buy(ctx context.Context, legs []Leg, base []*big.Int, steps *[]Step) ([]*big.Int, error) {
	out := make([]*big.Int, len(legs))
	done := map[string]bool{}
	for i, l := range legs {
		if l.Token == Base {
			out[i] = base[i]
			continue
		}
		if done[l.Token] {
			continue
		}
		done[l.Token] = true
		var idx []int
		var in []*big.Int
		sum := new(big.Int)
		for j := i; j < len(legs); j++ {
			if legs[j].Token == l.Token {
				idx, in = append(idx, j), append(in, base[j])
				sum.Add(sum, base[j])
			}
		}
		if sum.Sign() == 0 {
			for _, j := range idx {
				out[j] = new(big.Int)
			}
			continue
		}
		got, err := p.minOut(ctx, l.PoolID, p.id(Base), p.id(l.Token), sum)
		if err != nil {
			return nil, err
		}
		*steps = append(*steps, swap(l.PoolID, p.id(Base), p.id(l.Token), sum, got))
		for k, v := range split(got, in) {
			out[idx[k]] = v
		}
	}
	return out, nil
}
