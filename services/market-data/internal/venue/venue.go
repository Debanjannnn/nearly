// Package venue reads Rhea DEX pools, Rhea lending (Burrow) rates and LiNEAR share price over RPC.
package venue

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/services/market-data/internal/quote"
)

const (
	year = 365 * 24 * time.Hour
	// only picks the past block; the APY uses exact block timestamps
	blockSec = 0.6
)

type Reader struct {
	RPC *near.Client
	// Archival serves heights older than plain RPC keeps (~1 day).
	Archival *near.Client
}

func (r Reader) Pool(ctx context.Context, dex string, id uint64) (quote.Pool, error) {
	var p quote.Pool
	err := r.RPC.View(ctx, dex, "get_pool", map[string]uint64{"pool_id": id}, &p)
	p.ID = id
	return p, err
}

type Meta struct {
	Decimals int    `json:"decimals"`
	Symbol   string `json:"symbol"`
}

func (r Reader) Meta(ctx context.Context, token string) (Meta, error) {
	var m Meta
	return m, r.RPC.View(ctx, token, "ft_metadata", struct{}{}, &m)
}

// SupplyAPY is Burrow's supply_apr compounded continuously; farm rewards not included.
func (r Reader) SupplyAPY(ctx context.Context, burrow, token string) (float64, error) {
	var a struct {
		SupplyAPR string `json:"supply_apr"`
	}
	if err := r.RPC.View(ctx, burrow, "get_asset", map[string]string{"token_id": token}, &a); err != nil {
		return 0, err
	}
	apr, err := strconv.ParseFloat(a.SupplyAPR, 64)
	if err != nil {
		return 0, fmt.Errorf("get_asset %s supply_apr %q: %w", token, a.SupplyAPR, err)
	}
	return math.Expm1(apr), nil
}

// FtPrice is LiNEAR's ft_price: yoctoNEAR per 1e24 LiNEAR units.
func FtPrice(ctx context.Context, c *near.Client, height uint64, linear string) (*big.Int, error) {
	var s string
	if err := c.ViewAt(ctx, height, linear, "ft_price", struct{}{}, &s); err != nil {
		return nil, err
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("ft_price %q", s)
	}
	return v, nil
}

// StakeAPY annualises LiNEAR ft_price growth over the trailing window.
func (r Reader) StakeAPY(ctx context.Context, linear string, window time.Duration) (float64, error) {
	head, err := r.RPC.Block(ctx, 0)
	if err != nil {
		return 0, err
	}
	p1, err := FtPrice(ctx, r.RPC, head.Height, linear)
	if err != nil {
		return 0, err
	}
	target := head.Height - uint64(window.Seconds()/blockSec)
	var past near.BlockHeader
	// some heights have no block; take the next one that exists
	for h := target; h < target+10; h++ {
		if past, err = r.Archival.Block(ctx, h); err == nil {
			break
		}
	}
	if err != nil {
		return 0, err
	}
	p0, err := FtPrice(ctx, r.Archival, past.Height, linear)
	if err != nil {
		return 0, err
	}
	return TrailingAPY(p0, p1, time.Duration(head.Timestamp-past.Timestamp)), nil
}

func TrailingAPY(p0, p1 *big.Int, dt time.Duration) float64 {
	if p0.Sign() <= 0 || dt <= 0 {
		return 0
	}
	r, _ := new(big.Float).Quo(new(big.Float).SetInt(p1), new(big.Float).SetInt(p0)).Float64()
	return math.Expm1(math.Log(r) * float64(year) / float64(dt))
}
