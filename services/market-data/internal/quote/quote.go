// Package quote prices swaps on Rhea (Ref) SIMPLE pools from cached reserves.
package quote

import (
	"errors"
	"math/big"
	"slices"
)

const bps = 10_000

var (
	ErrBadInput    = errors.New("bad_request")
	ErrUnsupported = errors.New("unsupported_pool")
	ErrNoLiquidity = errors.New("no_liquidity")
)

// Pool is get_pool's shape plus the poll time (unix ms). TotalFee is in bps.
type Pool struct {
	ID       uint64   `json:"id"`
	Kind     string   `json:"pool_kind"`
	Tokens   []string `json:"token_account_ids"`
	Amounts  []string `json:"amounts"`
	TotalFee int64    `json:"total_fee"`
	Ts       int64    `json:"ts"`
}

func (p Pool) Has(a, b string) bool {
	return a != b && slices.Contains(p.Tokens, a) && slices.Contains(p.Tokens, b)
}

type Quote struct {
	PoolID    uint64 `json:"pool_id"`
	TokenIn   string `json:"token_in"`
	TokenOut  string `json:"token_out"`
	AmountIn  string `json:"amount_in"`
	AmountOut string `json:"amount_out"`
	MinOut    string `json:"min_out"`
	// MaxSafeIn is the largest amount_in whose price impact (ex fee) stays within max_impact_bps.
	MaxSafeIn string `json:"max_safe_in"`
	ImpactBps int64  `json:"impact_bps"`
	Ts        int64  `json:"ts"`
}

// Simple quotes amountIn on a SIMPLE_POOL with the contract's own integer formula.
func Simple(p Pool, tokenIn, tokenOut string, amountIn *big.Int, slipBps, maxImpactBps int64) (Quote, error) {
	if p.Kind != "SIMPLE_POOL" {
		return Quote{}, ErrUnsupported
	}
	i, o := slices.Index(p.Tokens, tokenIn), slices.Index(p.Tokens, tokenOut)
	if i < 0 || o < 0 || i == o || len(p.Amounts) != len(p.Tokens) || amountIn.Sign() <= 0 ||
		slipBps < 0 || slipBps > bps || maxImpactBps <= 0 || maxImpactBps >= bps || p.TotalFee < 0 || p.TotalFee >= bps {
		return Quote{}, ErrBadInput
	}
	rin, ok1 := new(big.Int).SetString(p.Amounts[i], 10)
	rout, ok2 := new(big.Int).SetString(p.Amounts[o], 10)
	if !ok1 || !ok2 || rin.Sign() <= 0 || rout.Sign() <= 0 {
		return Quote{}, ErrNoLiquidity
	}
	n := func(v int64) *big.Int { return big.NewInt(v) }
	// out = a*(B-fee)*rout / (B*rin + a*(B-fee)), as in ref-exchange simple_pool.rs
	withFee := new(big.Int).Mul(amountIn, n(bps-p.TotalFee))
	den := new(big.Int).Add(new(big.Int).Mul(rin, n(bps)), withFee)
	out := new(big.Int).Div(new(big.Int).Mul(withFee, rout), den)
	minOut := new(big.Int).Div(new(big.Int).Mul(out, n(bps-slipBps)), n(bps))
	// constant product, fee aside: impact = a/(rin+a), so max a = rin*m/(B-m)
	maxIn := new(big.Int).Div(new(big.Int).Mul(rin, n(maxImpactBps)), n(bps-maxImpactBps))
	impact := new(big.Int).Div(new(big.Int).Mul(amountIn, n(bps)), new(big.Int).Add(rin, amountIn))
	return Quote{
		PoolID: p.ID, TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: amountIn.String(), AmountOut: out.String(),
		MinOut: minOut.String(), MaxSafeIn: maxIn.String(), ImpactBps: impact.Int64(), Ts: p.Ts,
	}, nil
}

// SpotPrice is units of b per unit of a (decimals applied), from pool reserves.
func (p Pool) SpotPrice(a, b string, decA, decB int) (float64, bool) {
	i, j := slices.Index(p.Tokens, a), slices.Index(p.Tokens, b)
	if i < 0 || j < 0 || len(p.Amounts) != len(p.Tokens) {
		return 0, false
	}
	ra, ok1 := new(big.Float).SetString(p.Amounts[i])
	rb, ok2 := new(big.Float).SetString(p.Amounts[j])
	if !ok1 || !ok2 || ra.Sign() <= 0 {
		return 0, false
	}
	ra.Quo(ra, pow10(decA))
	rb.Quo(rb, pow10(decB))
	f, _ := rb.Quo(rb, ra).Float64()
	return f, true
}

func pow10(d int) *big.Float {
	return new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d)), nil))
}
