package poll

import (
	"math"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/services/market-data/internal/quote"
	"github.com/Debanjannnn/nearly/services/market-data/internal/rpcfake"
	"github.com/Debanjannnn/nearly/services/market-data/internal/store"
	"github.com/Debanjannnn/nearly/services/market-data/internal/venue"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

const usdc = "17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1"

func mainnet() *config.Deployments {
	d := &config.Deployments{Tokens: map[string]string{"usdc": usdc, "wnear": "wrap.near"}}
	d.Venues.Lending, d.Venues.Dex, d.Venues.Staking = "contract.main.burrow.near", "v2.ref-finance.near", "linear-protocol.near"
	return d
}

func TestOnce(t *testing.T) {
	ctx := t.Context()
	c := rpcfake.New(t, "mainnet")
	st := store.Store{RDB: redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()}), Net: "mainnet"}
	pub := map[string]any{}
	now := time.UnixMilli(1_000_000)
	p := &Poller{Venue: venue.Reader{RPC: c, Archival: c}, Dep: mainnet(), PoolIDs: []uint64{4512, 9}, Store: st,
		Publish: func(tp string, v any) { pub[tp] = v }, StakeEvery: time.Hour, Now: func() time.Time { return now }}
	if err := p.Once(ctx); err != nil {
		t.Fatal(err)
	}

	var rates []store.Rate
	if err := st.Get(ctx, store.KeyRates, &rates); err != nil || len(rates) != 3 {
		t.Fatal(rates, err)
	}
	if r := rates[0]; r.Venue != "lending" || r.Token != usdc || math.Abs(r.APY-0.110120) > 1e-5 || r.Ts != now.UnixMilli() {
		t.Fatal(r)
	}
	if r := rates[2]; r.Venue != "staking" || r.Token != "linear-protocol.near" || r.APY < 0.04 || r.APY > 0.045 {
		t.Fatal(r)
	}
	var prices []store.Price
	if err := st.Get(ctx, store.KeyPrices, &prices); err != nil || len(prices) != 3 {
		t.Fatal(prices, err)
	}
	near, lin := prices[1].USD, prices[2].USD
	if prices[0].USD != 1 || near < 3.77 || near > 3.78 || math.Abs(lin/near-1.4307587) > 1e-6 || prices[2].Symbol != "LINEAR" {
		t.Fatal(prices)
	}
	var pl quote.Pool
	if err := st.Get(ctx, store.KeyPool(4512), &pl); err != nil || pl.Ts != now.UnixMilli() {
		t.Fatal(pl, err)
	}
	if _, ok := pub["rates"]; !ok || pub["prices"] == nil {
		t.Fatal("not published")
	}

	// later poll with the RPC gone: values kept with their old ts
	now = now.Add(time.Minute)
	p.Venue = venue.Reader{RPC: rpcfake.New(t, "testnet"), Archival: rpcfake.New(t, "testnet")}
	if err := p.Once(ctx); err != nil {
		t.Fatal(err)
	}
	st.Get(ctx, store.KeyRates, &rates)
	if len(rates) != 3 || rates[0].Ts != 1_000_000 {
		t.Fatal(rates)
	}
}
