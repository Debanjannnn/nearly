package ingest

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/jackc/pgx/v5"
)

const (
	blockSec   = 0.6 // only seeds the height guess; blockAt corrects from real timestamps
	stakeWin   = 7   // days of LST share-price growth per staking APY point
	cursorName = "chain"
)

// Sample is the venue state at the first block of a UTC day.
type Sample struct {
	Day     string
	Height  uint64
	Lending map[string]float64 // token symbol -> supply APY
	Ratio   float64            // NEAR per LST unit; 0 = not available that day
}

// permanent reports a view that can never answer at this height (contract missing, method absent).
func permanent(err error) bool {
	var re *near.RPCError
	if !errors.As(err, &re) {
		return err != nil // query errors come back inside result as plain strings
	}
	switch re.Cause.Name {
	case "UNKNOWN_ACCOUNT", "NO_CONTRACT_CODE", "CONTRACT_EXECUTION_ERROR":
		return true
	}
	return false
}

func (g *Ingester) blockNear(ctx context.Context, h uint64) (near.BlockHeader, error) {
	var err error
	var b near.BlockHeader
	// some heights have no block; take the next one that exists
	for i := range uint64(10) {
		if b, err = g.Archival.Block(ctx, h+i); err == nil {
			return b, nil
		}
	}
	return b, err
}

// blockAt finds a block within 10 minutes of target by stepping from timestamp error.
func (g *Ingester) blockAt(ctx context.Context, head near.BlockHeader, target time.Time) (near.BlockHeader, error) {
	b := head
	for range 5 {
		diff := float64(int64(b.Timestamp)-target.UnixNano()) / 1e9
		if math.Abs(diff) < 600 {
			return b, nil
		}
		h := int64(b.Height) - int64(diff/blockSec)
		if h < 1 {
			h = 1
		}
		var err error
		if b, err = g.blockNear(ctx, uint64(h)); err != nil {
			return b, err
		}
	}
	return b, fmt.Errorf("no block near %s", target.Format(time.RFC3339))
}

func (g *Ingester) sample(ctx context.Context, head near.BlockHeader, day time.Time) (Sample, error) {
	b, err := g.blockAt(ctx, head, day)
	if err != nil {
		return Sample{}, err
	}
	s := Sample{Day: day.Format(Day), Height: b.Height, Lending: map[string]float64{}}
	for sym, token := range g.Dep.Tokens {
		var a struct {
			SupplyAPR string `json:"supply_apr"`
		}
		err := g.Archival.ViewAt(ctx, b.Height, g.Dep.Venues.Lending, "get_asset", map[string]string{"token_id": token}, &a)
		if permanent(err) {
			continue
		}
		if err != nil {
			return s, fmt.Errorf("get_asset %s @%d: %w", sym, b.Height, err)
		}
		apr, err := strconv.ParseFloat(a.SupplyAPR, 64)
		if err != nil {
			return s, fmt.Errorf("get_asset %s supply_apr %q: %w", sym, a.SupplyAPR, err)
		}
		s.Lending[sym] = math.Expm1(apr) // continuous APR -> APY; farm rewards not included
	}
	var p string
	err = g.Archival.ViewAt(ctx, b.Height, g.Dep.Venues.Staking, "ft_price", struct{}{}, &p)
	if err != nil && !permanent(err) {
		return s, fmt.Errorf("ft_price @%d: %w", b.Height, err)
	}
	if v, ok := new(big.Float).SetString(p); err == nil && ok {
		s.Ratio, _ = v.Quo(v, big.NewFloat(1e24)).Float64()
	}
	return s, nil
}

// Chain samples every day after the cursor up to today, then rebuilds the staking APY series.
func (g *Ingester) Chain(ctx context.Context) error {
	from := g.today().AddDate(0, 0, -g.Backfill)
	var last time.Time
	err := g.DB.QueryRow(ctx, `select day from history_cursor where name = $1`, cursorName).Scan(&last)
	switch {
	case err == nil:
		from = last.AddDate(0, 0, 1)
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	head, err := g.Archival.Block(ctx, 0)
	if err != nil {
		return err
	}
	for d := from; !d.After(g.today()); d = d.AddDate(0, 0, 1) {
		s, err := g.sample(ctx, head, d)
		// free archival RPC rate-limits (429): back off and retry the day a few times
		for try := 1; err != nil && try <= 3; try++ {
			if err := sleep(ctx, time.Duration(try)*10*g.Pause); err != nil {
				return err
			}
			s, err = g.sample(ctx, head, d)
		}
		if err != nil {
			return err
		}
		if err := g.putSample(ctx, s); err != nil {
			return err
		}
		if err := sleep(ctx, g.Pause); err != nil {
			return err
		}
	}
	return g.stakeRates(ctx)
}

func (g *Ingester) putSample(ctx context.Context, s Sample) error {
	return pgx.BeginFunc(ctx, g.DB, func(tx pgx.Tx) error {
		b := &pgx.Batch{}
		for sym, apy := range s.Lending {
			b.Queue(`insert into series_rate (source, token, day, apy) values ('lending', $1, $2, $3)
				on conflict (source, token, day) do update set apy = excluded.apy`, sym, s.Day, apy)
		}
		if s.Ratio > 0 {
			b.Queue(`insert into series_lst (day, ratio, height) values ($1, $2, $3)
				on conflict (day) do update set ratio = excluded.ratio, height = excluded.height`, s.Day, s.Ratio, s.Height)
		}
		b.Queue(`insert into history_cursor (name, day) values ($1, $2)
			on conflict (name) do update set day = excluded.day`, cursorName, s.Day)
		return tx.SendBatch(ctx, b).Close()
	})
}

func (g *Ingester) stakeRates(ctx context.Context) error {
	rows, err := g.DB.Query(ctx, `select to_char(day, 'YYYY-MM-DD'), ratio::float8 from series_lst`)
	if err != nil {
		return err
	}
	ratios := map[string]float64{}
	var d string
	var r float64
	if _, err := pgx.ForEachRow(rows, []any{&d, &r}, func() error { ratios[d] = r; return nil }); err != nil {
		return err
	}
	return g.putRates(ctx, stakeAPY(ratios))
}

// stakeAPY annualises LST share-price growth over the trailing stakeWin days.
func stakeAPY(ratios map[string]float64) []rate {
	var out []rate
	for d, r := range ratios {
		t, _ := time.Parse(Day, d)
		r0, ok := ratios[t.AddDate(0, 0, -stakeWin).Format(Day)]
		if !ok || r0 <= 0 {
			continue
		}
		out = append(out, rate{"staking", "wnear", d, math.Pow(r/r0, 365.0/stakeWin) - 1})
	}
	return out
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
