// Package loop follows final blocks, stores "nearly" events and reconciles the accounts they touch.
package loop

import (
	"context"
	"log/slog"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/services/indexer/internal/decode"
	"github.com/Debanjannnn/nearly/services/indexer/internal/reconcile"
	"github.com/Debanjannnn/nearly/services/indexer/internal/source"
	"github.com/Debanjannnn/nearly/services/indexer/internal/store"
)

type Indexer struct {
	Src     *source.Client
	RPC     *near.Client
	Store   store.Store
	Factory string            // sub-accounts are <name>.<factory>
	Tokens  map[string]string // deployments tokens
	Start   uint64            // first height when there is no cursor; 0 = latest final
	Batch   int               // max blocks per tick
}

// Run ticks every `every` and reconciles every idle account every `full`.
func (x *Indexer) Run(ctx context.Context, every, full time.Duration) error {
	lastFull := time.Time{}
	for {
		if err := x.Tick(ctx); err != nil {
			slog.Error("index tick", "err", err)
		}
		if time.Since(lastFull) >= full {
			if err := x.ReconcileAll(ctx, nil); err != nil {
				slog.Error("reconcile all", "err", err)
			}
			lastFull = time.Now()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
}

// Tick indexes up to Batch final blocks after the cursor, then reconciles the accounts they touched.
// Only final blocks are read, so nothing indexed can be reorged away.
func (x *Indexer) Tick(ctx context.Context) error {
	final, err := x.Src.Final(ctx)
	if err != nil {
		return err
	}
	cur, ok, err := x.Store.Cursor(ctx)
	if err != nil {
		return err
	}
	if !ok {
		cur = final
		if x.Start > 0 {
			cur = x.Start - 1
		}
		// pin the start so later ticks resume from here
		if err := x.Store.Save(ctx, cur, nil); err != nil {
			return err
		}
	}
	touched := map[string]bool{}
	for h := cur + 1; h <= final && h <= cur+uint64(x.Batch); h++ {
		b, err := x.Src.Block(ctx, h)
		if err != nil {
			return err
		}
		var evs []decode.Event
		if b != nil {
			evs = decode.Events(b, x.Factory)
		}
		if err := x.Store.Save(ctx, h, evs); err != nil {
			return err
		}
		for _, e := range evs {
			touched[e.Account] = true
		}
	}
	if len(touched) == 0 {
		return nil
	}
	accts := make([]string, 0, len(touched))
	for a := range touched {
		accts = append(accts, a)
	}
	return x.ReconcileAll(ctx, accts)
}

// ReconcileAll reconciles the given sub-accounts (nil = all) that have no execution in flight.
// shortcut: one RPC pair per account per pass; batch or spread out past a few hundred accounts.
func (x *Indexer) ReconcileAll(ctx context.Context, only []string) error {
	accts, err := x.Store.Idle(ctx, only)
	if err != nil {
		return err
	}
	for _, a := range accts {
		if err := x.reconcile(ctx, a); err != nil {
			slog.Error("reconcile", "account", a, "err", err)
		}
	}
	return nil
}

func (x *Indexer) reconcile(ctx context.Context, sub string) error {
	ledger, err := reconcile.Ledger(ctx, x.RPC, sub)
	if err != nil {
		return err
	}
	ps, err := x.Store.OpenPositions(ctx, sub)
	if err != nil {
		return err
	}
	held, idle := reconcile.Apply(ledger, ps, x.Tokens)
	return x.Store.Apply(ctx, sub, ledger, idle, ps, held)
}
