// Package ingest pulls daily history into Postgres: Rhea lending APY and LST share price sampled from
// archival RPC once per day, NEAR USD price from DefiLlama. Every write upserts, so reruns are safe.
package ingest

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Day = "2006-01-02"

type Ingester struct {
	DB       *pgxpool.Pool
	Archival *near.Client
	HTTP     *http.Client
	Dep      *config.Deployments
	Backfill int           // days of history to fetch on first run
	Pause    time.Duration // between archival day samples, to stay under free RPC limits
	Now      func() time.Time
}

// RunAll runs every job once; one failing source does not stop the other.
func (g *Ingester) RunAll(ctx context.Context) {
	for _, j := range []struct {
		name string
		fn   func(context.Context) error
	}{{"prices", g.Prices}, {"chain", g.Chain}} {
		t := time.Now()
		if err := j.fn(ctx); err != nil {
			slog.Error("ingest", "job", j.name, "err", err)
			continue
		}
		slog.Info("ingest", "job", j.name, "took", time.Since(t).String())
	}
}

// today is the current UTC day at 00:00.
func (g *Ingester) today() time.Time { return g.Now().UTC().Truncate(24 * time.Hour) }

type rate struct {
	source, token, day string
	apy                float64
}

func (g *Ingester) putRates(ctx context.Context, rs []rate) error {
	b := &pgx.Batch{}
	for _, r := range rs {
		b.Queue(`insert into series_rate (source, token, day, apy) values ($1, $2, $3, $4)
			on conflict (source, token, day) do update set apy = excluded.apy`, r.source, r.token, r.day, r.apy)
	}
	return g.DB.SendBatch(ctx, b).Close()
}
