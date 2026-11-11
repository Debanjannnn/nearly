package planner

import "math/big"

// Withdraw unwinds part (bps, 10000 = all) of every leg and sends the tokens to the owner as they
// are (no swap back). held is per leg, token atoms. Plan.Held = what remains. No fee.
func (p *Planner) Withdraw(b Basket, held []string, part int) (Plan, error) {
	if err := p.Validate(b); err != nil {
		return Plan{}, err
	}
	if part <= 0 || part > 10_000 {
		return Plan{}, fail("bad_request", "bps %d, want 1..10000", part)
	}
	h, err := p.heldFor(b, held)
	if err != nil {
		return Plan{}, err
	}
	var steps []Step
	out, order := map[string]*big.Int{}, []string{}
	for i, l := range b.Legs {
		take := mulDiv(h[i], int64(part), 10_000)
		steps = append(steps, p.take(l, take)...)
		h[i].Sub(h[i], take)
		if out[l.Token] == nil {
			out[l.Token], order = new(big.Int), append(order, l.Token)
		}
		out[l.Token].Add(out[l.Token], take)
	}
	for _, sym := range order {
		if out[sym].Sign() > 0 {
			steps = append(steps, tokenStep(KindToOwner, p.id(sym), out[sym]))
		}
	}
	if len(steps) == 0 {
		return Plan{}, fail("nothing_to_withdraw", "position is empty")
	}
	return Plan{Steps: steps, Held: strs(h)}, nil
}

func (p *Planner) heldFor(b Basket, held []string) ([]*big.Int, error) {
	if len(held) != len(b.Legs) {
		return nil, fail("bad_request", "%d held entries for %d legs", len(held), len(b.Legs))
	}
	return amts(held)
}

// Live is what the account holds on chain right now, token contract ID -> atoms.
type Live struct {
	Lent   map[string]string `json:"lent"`   // supplied to lending
	Staked string            `json:"staked"` // wnear atoms in the staking venue
	Wallet map[string]string `json:"wallet"` // free in the account
}

// ResumeWithdraw rebuilds the unfinished tail steps[done:] of a failed withdraw. Each step is capped
// by what is still on chain and dropped when nothing is left, so a retry never withdraws twice.
func (p *Planner) ResumeWithdraw(steps []Step, done int, live Live) ([]Step, error) {
	if done < 0 || done > len(steps) {
		return nil, fail("bad_request", "done %d outside 0..%d", done, len(steps))
	}
	// missing or unreadable balance = nothing there
	or0 := func(s string) *big.Int {
		if v, err := amt(s); err == nil {
			return v
		}
		return new(big.Int)
	}
	staked := or0(live.Staked)
	// wallet simulates the account as kept steps run, so to_owner sees what earlier steps freed
	wallet := map[string]*big.Int{}
	for k, v := range live.Wallet {
		wallet[k] = or0(v)
	}
	credit := func(tok string, v *big.Int) {
		if wallet[tok] == nil {
			wallet[tok] = new(big.Int)
		}
		wallet[tok].Add(wallet[tok], v)
	}
	var out []Step
	for _, st := range steps[done:] {
		planned, err := amt(st.Amount)
		if err != nil {
			return nil, err
		}
		var have *big.Int
		switch st.Kind {
		case KindWithdrawLend:
			have = or0(live.Lent[st.Token])
		case KindUnstake:
			have = staked
		case KindToOwner:
			have = wallet[st.Token]
		default:
			return nil, fail("bad_request", "%s is not a withdraw step", st.Kind)
		}
		v := planned
		if have == nil {
			v = new(big.Int)
		} else if have.Cmp(v) < 0 {
			v = new(big.Int).Set(have)
		}
		if v.Sign() == 0 {
			continue
		}
		switch st.Kind {
		case KindWithdrawLend:
			credit(st.Token, v)
		case KindUnstake:
			staked.Sub(staked, v)
			credit(p.id("wnear"), v)
		case KindToOwner:
			wallet[st.Token].Sub(wallet[st.Token], v)
		}
		st.Amount = v.String()
		out = append(out, st)
	}
	return out, nil
}
