// Package reconcile matches open positions' per-leg held amounts to the account contract ledger.
package reconcile

import (
	"context"
	"fmt"
	"math/big"

	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

type Position struct {
	ID   string
	Legs []planner.Leg
	Held []*big.Int // same order as Legs
}

// Slot is the ledger key a leg's funds sit under in the account contract.
func Slot(l planner.Leg, tokens map[string]string) string {
	switch l.Source {
	case planner.SourceLending:
		return "lending:" + tokens[l.Token]
	case planner.SourceStaking:
		return "staking"
	}
	return "balance:" + tokens[l.Token]
}

// Ledger reads get_balances + get_positions from an account contract as slot -> atoms.
func Ledger(ctx context.Context, rpc *near.Client, account string) (map[string]*big.Int, error) {
	out := map[string]*big.Int{}
	for method, prefix := range map[string]string{"get_balances": "balance:", "get_positions": ""} {
		var rows [][2]string
		if err := rpc.View(ctx, account, method, struct{}{}, &rows); err != nil {
			return nil, fmt.Errorf("%s %s: %w", account, method, err)
		}
		for _, r := range rows {
			v, ok := new(big.Int).SetString(r[1], 10)
			if !ok {
				return nil, fmt.Errorf("%s %s: bad amount %q", account, method, r[1])
			}
			out[prefix+r[0]] = v
		}
	}
	return out, nil
}

// Apply returns corrected held per position and on-chain surplus in no position (idle).
// A short slot scales its claims down pro rata; a surplus (undeployed deposit, failed deploy) stays idle.
func Apply(ledger map[string]*big.Int, ps []Position, tokens map[string]string) (held [][]*big.Int, idle map[string]*big.Int) {
	claimed := map[string]*big.Int{}
	held = make([][]*big.Int, len(ps))
	for i, p := range ps {
		held[i] = make([]*big.Int, len(p.Held))
		for j, h := range p.Held {
			held[i][j] = new(big.Int).Set(h)
			s := Slot(p.Legs[j], tokens)
			if claimed[s] == nil {
				claimed[s] = new(big.Int)
			}
			claimed[s].Add(claimed[s], h)
		}
	}
	idle = map[string]*big.Int{}
	for s, c := range ledger {
		if d := new(big.Int).Sub(c, orZero(claimed[s])); d.Sign() > 0 {
			idle[s] = d
		}
	}
	for s, want := range claimed {
		have := orZero(ledger[s])
		if have.Cmp(want) >= 0 {
			continue
		}
		// floor each share; the last leg in the slot takes the rounding so the slot sums exactly
		left, li, lj := new(big.Int).Set(have), -1, -1
		for i, p := range ps {
			for j := range p.Held {
				if Slot(p.Legs[j], tokens) != s || p.Held[j].Sign() == 0 {
					continue
				}
				held[i][j] = new(big.Int).Quo(new(big.Int).Mul(p.Held[j], have), want)
				left.Sub(left, held[i][j])
				li, lj = i, j
			}
		}
		if li >= 0 {
			held[li][lj].Add(held[li][lj], left)
		}
	}
	return held, idle
}

func orZero(v *big.Int) *big.Int {
	if v == nil {
		return new(big.Int)
	}
	return v
}
