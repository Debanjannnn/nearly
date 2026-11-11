// Package planner turns basket deploy / withdraw / rebalance requests into executor steps.
// No I/O of its own: swap quotes come through Quoter, token IDs from the deployments file.
package planner

import (
	"fmt"
	"math/big"
)

const (
	KindSupply       = "supply"
	KindWithdrawLend = "withdraw_lend"
	KindSwap         = "swap"
	KindStake        = "stake"
	KindUnstake      = "unstake"
	KindFee          = "fee"
	KindToOwner      = "to_owner"
)

// Step is the shared step JSON (serde tag "kind"); omitempty keeps each kind to its own fields.
// shortcut: stake/unstake amounts are wNEAR atoms (contract converts to LST); revisit if the LST step takes shares.
type Step struct {
	Kind     string  `json:"kind"`
	Token    string  `json:"token,omitempty"`
	Amount   string  `json:"amount,omitempty"`
	PoolID   *uint64 `json:"pool_id,omitempty"` // pointer: pool 0 is a real Rhea pool
	TokenIn  string  `json:"token_in,omitempty"`
	TokenOut string  `json:"token_out,omitempty"`
	AmountIn string  `json:"amount_in,omitempty"`
	MinOut   string  `json:"min_out,omitempty"`
}

func swap(pool uint64, in, out string, amountIn, minOut *big.Int) Step {
	return Step{Kind: KindSwap, PoolID: &pool, TokenIn: in, TokenOut: out, AmountIn: amountIn.String(), MinOut: minOut.String()}
}

func tokenStep(kind, token string, amount *big.Int) Step {
	s := Step{Kind: kind, Token: token, Amount: amount.String()}
	if kind == KindStake || kind == KindUnstake {
		s.Token = "" // staking venue holds one asset
	}
	return s
}

// Error carries an api error code.
type Error struct{ Code, Detail string }

func (e *Error) Error() string { return e.Code + ": " + e.Detail }

func fail(code, format string, a ...any) error { return &Error{code, fmt.Sprintf(format, a...)} }

// amt parses a u128 decimal string; negatives and junk are rejected.
func amt(s string) (*big.Int, error) {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok || v.Sign() < 0 || v.BitLen() > 128 {
		return nil, fail("bad_request", "amount %q is not a u128", s)
	}
	return v, nil
}

func amts(ss []string) ([]*big.Int, error) {
	out := make([]*big.Int, len(ss))
	for i, s := range ss {
		v, err := amt(s)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func strs(xs []*big.Int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.String()
	}
	return out
}

// mulDiv is floor(x*n/d).
func mulDiv(x *big.Int, n, d int64) *big.Int {
	v := new(big.Int).Mul(x, big.NewInt(n))
	return v.Quo(v, big.NewInt(d))
}

// mulDivBig is floor(x*n/d) with big n, d.
func mulDivBig(x, n, d *big.Int) *big.Int {
	v := new(big.Int).Mul(x, n)
	return v.Quo(v, d)
}

// split cuts total by weights (floor); the last nonzero weight takes the remainder so nothing
// is lost. All-zero weights get nothing.
func split(total *big.Int, weights []*big.Int) []*big.Int {
	sum, last := new(big.Int), -1
	for i, w := range weights {
		sum.Add(sum, w)
		if w.Sign() > 0 {
			last = i
		}
	}
	out, left := make([]*big.Int, len(weights)), new(big.Int).Set(total)
	for i, w := range weights {
		out[i] = new(big.Int)
		if w.Sign() > 0 && i != last {
			out[i] = mulDivBig(total, w, sum)
			left.Sub(left, out[i])
		}
	}
	if last >= 0 {
		out[last] = left
	}
	return out
}
