package planner

import (
	"context"
	"math/big"
)

const (
	SourceLending = "lending" // Rhea lending supply
	SourceStaking = "staking" // liquid staking, wnear only
	SourceHold    = "hold"    // token stays in the account

	Base    = "usdc" // deposits and fees are in this token
	FeeBps  = 40     // open fee to treasury
	MaxLegs = 8
)

// Leg is one token x source with its share of the basket. A non-base token is bought via
// the Rhea pool PoolID (base<->token), so "swap" is implied by Token != Base.
type Leg struct {
	Token     string `json:"token"` // symbol in deployments tokens, e.g. "wnear"
	Source    string `json:"source"`
	WeightBps int    `json:"weight_bps"`
	PoolID    uint64 `json:"pool_id,omitempty"`
}

type Basket struct {
	Legs     []Leg `json:"legs"`
	DriftBps int   `json:"drift_bps,omitempty"` // rebalance band; 0 = never
}

// Plan.Held is what each leg holds afterwards (token atoms, guaranteed minimums), same order as legs.
type Plan struct {
	Steps []Step   `json:"steps"`
	Fee   string   `json:"fee,omitempty"`
	Held  []string `json:"held,omitempty"`
}

// Quoter returns the expected swap output (atoms) on a Rhea pool; token args are contract IDs.
type Quoter interface {
	Quote(ctx context.Context, poolID uint64, tokenIn, tokenOut string, amountIn *big.Int) (*big.Int, error)
}

type Planner struct {
	Tokens      map[string]string // symbol -> contract ID, from deployments tokens
	Q           Quoter
	SlippageBps int
	LST         string  // LiNEAR token = deployments venues.staking
	LSTPool     *uint64 // Rhea LiNEAR/wNEAR pool; nil = exit by delayed unstake
}

func (p *Planner) id(sym string) string { return p.Tokens[sym] }

func (p *Planner) Validate(b Basket) error {
	if len(b.Legs) == 0 || len(b.Legs) > MaxLegs {
		return fail("invalid_basket", "need 1..%d legs", MaxLegs)
	}
	if b.DriftBps < 0 || b.DriftBps > 10_000 {
		return fail("invalid_basket", "drift_bps %d", b.DriftBps)
	}
	sum, seen := 0, map[Leg]bool{}
	for _, l := range b.Legs {
		if p.id(l.Token) == "" {
			return fail("invalid_basket", "unknown token %q", l.Token)
		}
		switch l.Source {
		case SourceLending, SourceHold:
		case SourceStaking:
			if l.Token != "wnear" {
				return fail("invalid_basket", "staking takes wnear, not %s", l.Token)
			}
		default:
			return fail("invalid_basket", "unknown source %q", l.Source)
		}
		key := Leg{Token: l.Token, Source: l.Source}
		if l.WeightBps <= 0 || seen[key] {
			return fail("invalid_basket", "bad weight or duplicate %s/%s", l.Token, l.Source)
		}
		seen[key], sum = true, sum+l.WeightBps
	}
	if sum != 10_000 {
		return fail("invalid_basket", "weights sum %d, want 10000", sum)
	}
	return nil
}

// minOut quotes a swap and applies slippage.
func (p *Planner) minOut(ctx context.Context, pool uint64, in, out string, amountIn *big.Int) (*big.Int, error) {
	q, err := p.Q.Quote(ctx, pool, in, out, amountIn)
	if err != nil {
		return nil, err
	}
	m := mulDiv(q, int64(10_000-p.SlippageBps), 10_000)
	if m.Sign() <= 0 {
		return nil, fail("insufficient_liquidity", "swap %s %s->%s quotes nothing", amountIn, in, out)
	}
	return m, nil
}

// put moves amount of leg's token into its source; hold needs no step.
func (p *Planner) put(l Leg, amount *big.Int) []Step {
	switch {
	case amount.Sign() == 0 || l.Source == SourceHold:
		return nil
	case l.Source == SourceStaking:
		return []Step{{Kind: KindStake, Token: p.id("wnear"), Amount: amount.String()}}
	}
	return []Step{tokenStep(KindSupply, p.id(l.Token), amount)}
}

// take pulls amount of leg's token out of its source; got = what lands in the account now.
// Staking swaps LiNEAR to wNEAR; without a pool it unstakes and nothing lands for ~4 epochs.
func (p *Planner) take(l Leg, amount *big.Int) (steps []Step, got *big.Int) {
	switch {
	case amount.Sign() == 0 || l.Source == SourceHold:
		return nil, amount
	case l.Source == SourceStaking && p.LSTPool != nil:
		// amount is NEAR value and the pool is rated, so value minus slippage is the floor
		m := p.lstMinOut(amount)
		return []Step{swap(*p.LSTPool, p.LST, p.id("wnear"), amount, m)}, m
	case l.Source == SourceStaking:
		return []Step{tokenStep(KindUnstake, "", amount)}, new(big.Int)
	}
	return []Step{tokenStep(KindWithdrawLend, p.id(l.Token), amount)}, amount
}

func (p *Planner) lstMinOut(amount *big.Int) *big.Int {
	return mulDiv(amount, int64(10_000-p.SlippageBps), 10_000)
}
