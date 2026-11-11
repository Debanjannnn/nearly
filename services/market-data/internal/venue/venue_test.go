package venue

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/services/market-data/internal/rpcfake"
)

const usdc = "17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1"

func reader(t *testing.T, net string) Reader {
	c := rpcfake.New(t, net)
	return Reader{RPC: c, Archival: c}
}

func TestMainnetFixture(t *testing.T) {
	r, ctx := reader(t, "mainnet"), t.Context()
	p, err := r.Pool(ctx, "v2.ref-finance.near", 4512)
	if err != nil || p.ID != 4512 || p.Kind != "SIMPLE_POOL" || p.TotalFee != 30 || !p.Has(usdc, "wrap.near") {
		t.Fatal(p, err)
	}
	apy, err := r.SupplyAPY(ctx, "contract.main.burrow.near", usdc)
	if err != nil || math.Abs(apy-math.Expm1(0.10446764241892509)) > 1e-12 {
		t.Fatal(apy, err)
	}
	m, err := r.Meta(ctx, usdc)
	if err != nil || m.Decimals != 6 || m.Symbol != "USDC" {
		t.Fatal(m, err)
	}
	// ft_price 7 days apart: ~4.3%
	s, err := r.StakeAPY(ctx, "linear-protocol.near", 7*24*time.Hour)
	if err != nil || s < 0.04 || s > 0.045 {
		t.Fatal(s, err)
	}
}

func TestStakeAPYErrors(t *testing.T) {
	r := reader(t, "testnet")
	// window not recorded: past block lookup fails
	if _, err := r.StakeAPY(t.Context(), "linear-protocol.testnet", time.Hour); err == nil {
		t.Fatal("want error")
	}
	if _, err := r.SupplyAPY(t.Context(), "burrow.services.ref-labs.testnet", "nope.testnet"); err == nil {
		t.Fatal("want error")
	}
}

func TestTrailingAPY(t *testing.T) {
	if got := TrailingAPY(big.NewInt(100), big.NewInt(101), year); math.Abs(got-0.01) > 1e-9 {
		t.Fatal(got)
	}
	if TrailingAPY(big.NewInt(0), big.NewInt(1), year) != 0 || TrailingAPY(big.NewInt(1), big.NewInt(2), 0) != 0 {
		t.Fatal("guard")
	}
}
