// Package sched fires due schedules (DCA, recurring basket buy, drift rebalance); the executor signs.
package sched

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

// Market gates every tick on fresh prices + rates and quotes swaps.
type Market interface {
	Fresh(ctx context.Context) error
	planner.Quoter
}

// Keys tells whether the executor key is still on a sub-account.
type Keys interface {
	Active(ctx context.Context, account string) (bool, error)
}

type Runner interface {
	Execute(ctx context.Context, account, executionID string, steps []planner.Step) ([]executor.Result, error)
}

type Scheduler struct {
	Store  Store
	Plan   *planner.Planner // Q should be Market
	Market Market
	Keys   Keys
	Exec   Runner
	Tick   time.Duration

	bg sync.WaitGroup
}

func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.Tick)
	defer t.Stop()
	for {
		s.Once(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Wait blocks until in-flight executor runs have recorded their results.
func (s *Scheduler) Wait() { s.bg.Wait() }

// Once fires every due schedule, but only on fresh market data.
func (s *Scheduler) Once(ctx context.Context, now time.Time) {
	if err := s.Market.Fresh(ctx); err != nil {
		slog.Warn("market data not fresh, skipping tick", "err", err)
		return
	}
	due, err := s.Store.Due(ctx, now)
	if err != nil {
		slog.Error("due schedules", "err", err)
		return
	}
	keyOK := map[string]bool{}
	for _, d := range due {
		ok, seen := keyOK[d.Sub]
		if !seen {
			if ok, err = s.Keys.Active(ctx, d.Sub); err != nil {
				slog.Warn("executor key check", "account", d.Sub, "err", err)
				continue
			}
			keyOK[d.Sub] = ok
			if !ok {
				slog.Info("executor key revoked, stopping schedules", "account", d.Account)
				if err := s.Store.Revoke(ctx, d.Account); err != nil {
					slog.Error("revoke", "account", d.Account, "err", err)
				}
			}
		}
		if !ok {
			continue
		}
		if err := s.fire(ctx, d, now); err != nil {
			slog.Warn("schedule skipped", "schedule", d.ID, "kind", d.Kind, "err", err)
		}
	}
}

// NextRun is the first next+k*every strictly after now (k>=1); missed slots are skipped, not backfilled.
func NextRun(next time.Time, every int64, now time.Time) time.Time {
	iv := time.Duration(every) * time.Second
	k := int64(1)
	if now.After(next) {
		k = int64(now.Sub(next)/iv) + 1
	}
	return next.Add(time.Duration(k) * iv)
}

type params struct {
	Amount   string `json:"amount"`
	DriftBps *int   `json:"drift_bps"` // rebalance: overrides the basket band
}

func (s *Scheduler) fire(ctx context.Context, d Due, now time.Time) error {
	var p params
	if err := json.Unmarshal(d.Params, &p); err != nil {
		return err
	}
	if d.Every <= 0 {
		return fmt.Errorf("every %d", d.Every)
	}
	var (
		j     Job
		after func(ctx context.Context, execID string, done int, ok bool)
	)
	switch d.Kind {
	case "dca", "recurring_buy":
		if d.BasketID == nil {
			return fmt.Errorf("no basket")
		}
		def, creator, err := s.Store.Basket(ctx, *d.BasketID)
		if err != nil {
			return err
		}
		if d.Kind == "dca" {
			def.Legs = holdLegs(def.Legs)
		}
		plan, err := s.Plan.Deploy(ctx, def, p.Amount)
		if err != nil {
			return err
		}
		j = Job{Kind: "deploy", Steps: plan.Steps, Params: map[string]any{"amount": p.Amount, "fee": plan.Fee, "held": plan.Held, "schedule_id": d.ID}}
		if d.Kind == "dca" {
			j.Kind = "dca"
		} else {
			j.Open = p.Amount
		}
		after = func(ctx context.Context, execID string, done int, ok bool) {
			var fee *Fee
			if done > 0 { // fee step is first
				fee = s.feeRow(execID, plan.Fee, creator, d.Account)
			}
			state, held := "", []string(nil)
			if j.Open != "" {
				// shortcut: a failed buy closes with held [], funds already moved stay in the account
				state, held = "closed", []string{}
				if ok {
					state, held = "open", plan.Held
				}
			}
			if err := s.Store.Settle(ctx, j.PositionID, state, held, fee); err != nil {
				slog.Error("settle", "execution", execID, "err", err)
			}
		}
	case "rebalance":
		if d.PositionID == nil {
			return fmt.Errorf("no position")
		}
		def, held, ok, err := s.Store.Position(ctx, *d.PositionID)
		if err != nil || !ok {
			return err
		}
		if p.DriftBps != nil {
			def.DriftBps = *p.DriftBps
		}
		values, err := s.values(ctx, def, held)
		if err != nil {
			return err
		}
		plan, err := s.Plan.Rebalance(ctx, def, held, values)
		if err != nil || len(plan.Steps) == 0 {
			return err // inside the band: slot stays due, rechecked next tick
		}
		j = Job{Kind: "rebalance", PositionID: *d.PositionID, Steps: plan.Steps, Params: map[string]any{"held_after": plan.Held, "schedule_id": d.ID}}
		after = func(ctx context.Context, execID string, _ int, ok bool) {
			if !ok {
				return // shortcut: a half-done rebalance keeps old held; read chain balances if that drifts
			}
			if err := s.Store.Settle(ctx, j.PositionID, "", plan.Held, nil); err != nil {
				slog.Error("settle rebalance", "execution", execID, "err", err)
			}
		}
	default:
		return fmt.Errorf("unknown kind %q", d.Kind)
	}
	// the slot advances only when something fires, so a rebalance waits a full interval after acting
	execID, err := s.Store.Claim(ctx, d, NextRun(d.Next, d.Every, now), &j)
	if err != nil || execID == "" {
		return err
	}
	slog.Info("fired", "schedule", d.ID, "kind", d.Kind, "execution", execID)
	s.run(d.Sub, execID, j.Steps, after)
	return nil
}

// holdLegs is DCA's basket: same tokens kept in the account (no venue, no position), one leg per token.
func holdLegs(legs []planner.Leg) []planner.Leg {
	var out []planner.Leg
	at := map[string]int{}
	for _, l := range legs {
		if i, ok := at[l.Token]; ok {
			out[i].WeightBps += l.WeightBps
			continue
		}
		at[l.Token] = len(out)
		l.Source = planner.SourceHold
		out = append(out, l)
	}
	return out
}

// values prices each leg's held atoms in base atoms via the leg's pool quote.
// shortcut: held is the planned minimum from the db, not live chain balances.
func (s *Scheduler) values(ctx context.Context, b planner.Basket, held []string) ([]string, error) {
	if len(held) != len(b.Legs) {
		return nil, fmt.Errorf("held has %d legs, basket %d", len(held), len(b.Legs))
	}
	out := make([]string, len(held))
	for i, l := range b.Legs {
		h, ok := new(big.Int).SetString(held[i], 10)
		if !ok {
			return nil, fmt.Errorf("bad held %q", held[i])
		}
		if l.Token == planner.Base || h.Sign() == 0 {
			out[i] = h.String()
			continue
		}
		v, err := s.Market.Quote(ctx, l.PoolID, s.Plan.Tokens[l.Token], s.Plan.Tokens[planner.Base], h)
		if err != nil {
			return nil, err
		}
		out[i] = v.String()
	}
	return out, nil
}

// feeRow books the fee; half goes to the basket creator unless the user bought their own basket.
func (s *Scheduler) feeRow(execID, amount, creator, me string) *Fee {
	fee, ok := new(big.Int).SetString(amount, 10)
	if !ok || fee.Sign() == 0 {
		return nil
	}
	f := &Fee{ExecID: execID, Token: s.Plan.Tokens[planner.Base], Amount: amount, CreatorShare: "0"}
	if creator != me {
		f.Creator, f.CreatorShare = &creator, new(big.Int).Quo(fee, big.NewInt(2)).String()
	}
	return f
}

// run hands the steps to the executor and records the outcome; executor down = failed.
func (s *Scheduler) run(sub, execID string, steps []planner.Step, after func(ctx context.Context, execID string, done int, ok bool)) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		results, err := s.Exec.Execute(ctx, sub, execID, steps)
		done, msg := 0, ""
		for _, r := range results {
			if r.Status != "ok" || r.Index != done {
				msg = r.Error
				break
			}
			done++
		}
		if err != nil {
			slog.Error("executor", "execution", execID, "err", err)
			msg = "executor_unavailable"
		}
		ok := err == nil && done == len(steps)
		status := "failed"
		if ok {
			status, msg = "succeeded", ""
		} else if msg == "" {
			msg = "incomplete"
		}
		if err := s.Store.Finish(ctx, execID, results, done, status, msg); err != nil {
			slog.Error("finish execution", "execution", execID, "err", err)
		}
		after(ctx, execID, done, ok)
	}()
}
