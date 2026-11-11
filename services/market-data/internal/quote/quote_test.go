package quote

import (
	"errors"
	"math/big"
	"testing"
)

const usdc, wnear = "17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1", "wrap.near"

// mainnet pool 4512 at block 224097939
var pool = Pool{ID: 4512, Kind: "SIMPLE_POOL", Tokens: []string{usdc, wnear},
	Amounts: []string{"981514891976", "260301774254716729876081792862"}, TotalFee: 30, Ts: 7}

func amt(s string) *big.Int { v, _ := new(big.Int).SetString(s, 10); return v }

func TestSimpleMatchesGetReturn(t *testing.T) {
	// want = live get_return at the same block
	for _, c := range []struct{ in, out, amount, want, minOut string }{
		{usdc, wnear, "1000000000", "264140180949883041241856786", "262819480045133626035647502"},
		{wnear, usdc, "100000000000000000000000000", "375792936", "373913971"},
	} {
		q, err := Simple(pool, c.in, c.out, amt(c.amount), 50, 100)
		if err != nil || q.AmountOut != c.want || q.MinOut != c.minOut || q.Ts != 7 {
			t.Fatal(q, err)
		}
	}
}

func TestMaxSafeIn(t *testing.T) {
	q, _ := Simple(pool, usdc, wnear, amt("1"), 0, 100)
	// 1% impact: rin*100/9900
	if q.MaxSafeIn != "9914291838" {
		t.Fatal(q.MaxSafeIn)
	}
	at, _ := Simple(pool, usdc, wnear, amt(q.MaxSafeIn), 0, 100)
	over, _ := Simple(pool, usdc, wnear, amt("19914291838"), 0, 100)
	if at.ImpactBps > 100 || over.ImpactBps <= 100 {
		t.Fatal(at.ImpactBps, over.ImpactBps)
	}
}

func TestSimpleRejects(t *testing.T) {
	stable := pool
	stable.Kind = "RATED_SWAP"
	empty := pool
	empty.Amounts = []string{"0", "1"}
	for name, c := range map[string]struct {
		p    Pool
		in   string
		a    string
		slip int64
		want error
	}{
		"kind":  {stable, usdc, "1", 0, ErrUnsupported},
		"token": {pool, "x.near", "1", 0, ErrBadInput},
		"same":  {pool, wnear, "1", 0, ErrBadInput},
		"zero":  {pool, usdc, "0", 0, ErrBadInput},
		"slip":  {pool, usdc, "1", 10_001, ErrBadInput},
		"empty": {empty, usdc, "1", 0, ErrNoLiquidity},
	} {
		if _, err := Simple(c.p, c.in, wnear, amt(c.a), c.slip, 100); !errors.Is(err, c.want) {
			t.Error(name, err)
		}
	}
}

func TestSpotPrice(t *testing.T) {
	p, ok := pool.SpotPrice(wnear, usdc, 24, 6)
	if !ok || p < 3.77 || p > 3.78 {
		t.Fatal(p, ok)
	}
}
