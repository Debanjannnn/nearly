// Package poll reads every venue on a timer, caches results in Redis and pushes them to WebSocket topics.
package poll

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"maps"
	"math/big"
	"slices"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/services/market-data/internal/quote"
	"github.com/Debanjannnn/nearly/services/market-data/internal/store"
	"github.com/Debanjannnn/nearly/services/market-data/internal/venue"
)

const stakeWindow = 7 * 24 * time.Hour

type Poller struct {
	Venue   venue.Reader
	Dep     *config.Deployments
	PoolIDs []uint64
	Store   store.Store
	Publish func(topic string, v any)
	// StakeEvery spaces out the archival reads behind the staking APY.
	StakeEvery time.Duration
	Now        func() time.Time

	// last good values; a failed read keeps the old one with its old ts
	pools map[uint64]quote.Pool
	rates map[string]store.Rate
	meta  map[string]venue.Meta
}

func (p *Poller) Run(ctx context.Context, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := p.Once(ctx); err != nil {
			slog.Warn("poll", "err", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// Once polls everything; venue failures are logged and skipped, only Redis errors are returned.
func (p *Poller) Once(ctx context.Context) error {
	if p.pools == nil {
		p.pools, p.rates, p.meta = map[uint64]quote.Pool{}, map[string]store.Rate{}, map[string]venue.Meta{}
	}
	now := p.Now().UnixMilli()
	var errs []error
	for _, id := range p.PoolIDs {
		pl, err := p.Venue.Pool(ctx, p.Dep.Venues.Dex, id)
		if err != nil {
			slog.Warn("pool", "id", id, "err", err)
			continue
		}
		pl.Ts = now
		p.pools[id] = pl
		errs = append(errs, p.Store.Put(ctx, store.KeyPool(id), pl))
	}
	p.pollRates(ctx, now)
	rates := slices.SortedFunc(maps.Values(p.rates), func(a, b store.Rate) int {
		return cmp.Compare(a.Venue+a.Token, b.Venue+b.Token)
	})
	prices := p.prices(ctx, now)
	errs = append(errs, p.Store.Put(ctx, store.KeyRates, rates), p.Store.Put(ctx, store.KeyPrices, prices))
	if err := errors.Join(errs...); err != nil {
		return err
	}
	p.Publish("rates", rates)
	p.Publish("prices", prices)
	return nil
}

func (p *Poller) pollRates(ctx context.Context, now int64) {
	lending := p.Dep.Venues.Lending
	for _, name := range slices.Sorted(maps.Keys(p.Dep.Tokens)) {
		tok := p.Dep.Tokens[name]
		apy, err := p.Venue.SupplyAPY(ctx, lending, tok)
		if err != nil {
			slog.Warn("supply apy", "token", tok, "err", err)
			continue
		}
		p.rates["lending"+tok] = store.Rate{Venue: "lending", Contract: lending, Token: tok, APY: apy, Ts: now}
	}
	lst := p.Dep.Venues.Staking
	if r, ok := p.rates["staking"+lst]; ok && now-r.Ts < p.StakeEvery.Milliseconds() {
		return
	}
	apy, err := p.Venue.StakeAPY(ctx, lst, stakeWindow)
	if err != nil {
		slog.Warn("stake apy", "err", err)
		return
	}
	// the LST contract is also its FT, so token == contract
	p.rates["staking"+lst] = store.Rate{Venue: "staking", Contract: lst, Token: lst, APY: apy, Ts: now}
}

// prices: USDC pegged at 1, wNEAR from the USDC/wNEAR pool, LiNEAR = ft_price × wNEAR.
func (p *Poller) prices(ctx context.Context, now int64) []store.Price {
	usdc, wnear, lst := p.Dep.Tokens["usdc"], p.Dep.Tokens["wnear"], p.Dep.Venues.Staking
	mu, err1 := p.metaOf(ctx, usdc)
	mw, err2 := p.metaOf(ctx, wnear)
	if err := errors.Join(err1, err2); err != nil {
		slog.Warn("ft_metadata", "err", err)
		return []store.Price{}
	}
	// shortcut: USDC treated as exactly $1, swap for an oracle if a depeg matters
	out := []store.Price{{Token: usdc, Symbol: mu.Symbol, USD: 1, Ts: now}}
	var near store.Price
	for _, id := range p.PoolIDs {
		pl, ok := p.pools[id]
		if !ok || !pl.Has(usdc, wnear) {
			continue
		}
		if usd, ok := pl.SpotPrice(wnear, usdc, mw.Decimals, mu.Decimals); ok {
			near = store.Price{Token: wnear, Symbol: mw.Symbol, USD: usd, Ts: pl.Ts}
			out = append(out, near)
			break
		}
	}
	if near.Ts == 0 || lst == "" {
		return out
	}
	ml, err := p.metaOf(ctx, lst)
	if err != nil {
		slog.Warn("ft_metadata", "token", lst, "err", err)
		return out
	}
	fp, err := venue.FtPrice(ctx, p.Venue.RPC, 0, lst)
	if err != nil {
		slog.Warn("ft_price", "err", err)
		return out
	}
	r, _ := new(big.Float).Quo(new(big.Float).SetInt(fp), big.NewFloat(1e24)).Float64()
	return append(out, store.Price{Token: lst, Symbol: ml.Symbol, USD: r * near.USD, Ts: min(near.Ts, now)})
}

func (p *Poller) metaOf(ctx context.Context, tok string) (venue.Meta, error) {
	if m, ok := p.meta[tok]; ok {
		return m, nil
	}
	m, err := p.Venue.Meta(ctx, tok)
	if err == nil {
		p.meta[tok] = m
	}
	return m, err
}
