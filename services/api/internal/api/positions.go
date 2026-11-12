package api

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/internal/shared/sweep"
)

func (s *Server) deploy(w http.ResponseWriter, r *http.Request) {
	bid, ok := pathID(w, r)
	var in struct {
		Amount string `json:"amount"` // base token atoms
	}
	if !ok || !decode(w, r, &in) {
		return
	}
	if !positiveU128(in.Amount) {
		fail(w, "deploy", ErrBadRequest)
		return
	}
	ctx, me := r.Context(), account(r)
	sub, err := s.ready(r)
	var b Basket
	if err == nil {
		b, err = s.Store.Basket(ctx, bid, me)
	}
	var plan planner.Plan
	if err == nil {
		plan, err = s.Plan.Deploy(ctx, b.Definition, in.Amount)
	}
	if err != nil {
		fail(w, "deploy", err)
		return
	}
	params := ExecParams{Amount: in.Amount, Fee: plan.Fee, Held: plan.Held}
	posID, execID, err := s.Store.OpenPosition(ctx, me, bid, params, plan.Steps)
	if err != nil {
		fail(w, "open position", err)
		return
	}
	fee := s.feeRow(plan, b.Creator, me)
	s.run(execID, me, sub, plan.Steps, func(ctx context.Context, done int, ok bool) {
		if done == 0 {
			fee = nil // fee step is first; nothing paid
		}
		stranded := !ok && done > 1 // past the fee step, something sits in a venue
		if err := s.Store.SettleDeploy(ctx, posID, execID, ok, stranded, plan.Held, fee); err != nil {
			slog.Error("settle deploy", "position", posID, "err", err)
		}
	})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"execution_id": execID, "position_id": posID, "fee": plan.Fee, "steps": plan.Steps})
}

// feeRow books the open fee; half goes to the basket creator unless they deploy their own basket.
func (s *Server) feeRow(plan planner.Plan, creator, me string) *FeeRow {
	fee, ok := new(big.Int).SetString(plan.Fee, 10)
	if !ok || fee.Sign() == 0 {
		return nil
	}
	row := &FeeRow{Token: s.Dep.Tokens[planner.Base], Amount: plan.Fee, CreatorShare: "0"}
	if creator != me {
		row.Creator, row.CreatorShare = &creator, new(big.Int).Quo(fee, big.NewInt(2)).String()
	}
	return row
}

func (s *Server) positions(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.Positions(r.Context(), account(r))
	reply(w, "positions", v, err)
}

func (s *Server) position(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.Store.Position(r.Context(), account(r), id)
		reply(w, "position", v, err)
	}
}

// withdraw unwinds bps of an open position, or resumes a failed withdraw from its steps_done with every
// step capped by live chain state (planner.ResumeWithdraw), so a retry never withdraws twice.
func (s *Server) withdraw(w http.ResponseWriter, r *http.Request) {
	pid, ok := pathID(w, r)
	var in struct {
		Bps *int `json:"bps"`
	}
	if !ok || (r.ContentLength != 0 && !decode(w, r, &in)) {
		return
	}
	bps := 10_000
	if in.Bps != nil {
		bps = *in.Bps
	}
	if bps < 1 || bps > 10_000 {
		fail(w, "withdraw", ErrBadRequest)
		return
	}
	ctx, me := r.Context(), account(r)
	sub, err := s.ready(r)
	if err != nil {
		fail(w, "withdraw", err)
		return
	}
	var params ExecParams
	execID, steps, err := s.Store.ClaimWithdraw(ctx, me, pid, func(pos Position, def planner.Basket, last *LastWithdraw) ([]planner.Step, ExecParams, error) {
		steps, done := []planner.Step(nil), 0
		if last == nil {
			if pos.State == "withdrawing" {
				bps = 10_000 // swept dead deploy: unwind it all, capped by live state below
			}
			plan, err := s.Plan.Withdraw(def, pos.Held, bps)
			params = ExecParams{Bps: bps, HeldAfter: plan.Held}
			if err != nil || pos.State == "open" {
				return plan.Steps, params, err
			}
			steps = plan.Steps
		} else {
			params = ExecParams{Bps: last.Params.Bps, HeldAfter: last.Params.HeldAfter, ResumeOf: last.ID}
			steps, done = last.Steps, last.StepsDone
		}
		live, err := s.Chain.Live(ctx, sub, steps[min(done, len(steps)):])
		if err != nil {
			return nil, params, rpc("live state", err)
		}
		steps, err = s.Plan.ResumeWithdraw(steps, done, live)
		return steps, params, err
	})
	if err != nil {
		fail(w, "withdraw", err)
		return
	}
	if execID == "" {
		// nothing left on chain: position already settled to held_after
		httpx.JSON(w, http.StatusOK, map[string]any{"position_id": pid, "steps": []planner.Step{}})
		return
	}
	s.run(execID, me, sub, steps, func(ctx context.Context, _ int, ok bool) {
		if err := s.Store.SettleWithdraw(ctx, pid, ok, params.HeldAfter); err != nil {
			slog.Error("settle withdraw", "position", pid, "err", err)
		}
	})
	httpx.JSON(w, http.StatusAccepted, map[string]any{"execution_id": execID, "position_id": pid, "resumed": params.ResumeOf != "", "steps": steps})
}

// run sends steps to the executor in the background, records results, then calls after.
func (s *Server) run(execID, me, sub string, steps []planner.Step, after func(ctx context.Context, done int, ok bool)) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		ectx, cancel := context.WithTimeout(context.Background(), executor.MaxRun)
		results, err := s.Exec.Execute(ectx, sub, execID, steps)
		cancel()
		// own ctx: an executor timeout must not leave the run stuck in 'running'
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if executor.Tidy(execID, results) {
			s.keyGone(ctx, me, sub)
		}
		done, msg := 0, ""
		for _, res := range results {
			if res.Status != "ok" || res.Index != done {
				msg = res.Error
				break
			}
			done++
		}
		if err != nil {
			slog.Error("executor", "execution", execID, "err", err)
			msg = "executor_unavailable"
		}
		ok := err == nil && done == len(steps)
		if !ok && msg == "" {
			msg = "incomplete"
		}
		status := "failed"
		if ok {
			status, msg = "succeeded", ""
		}
		if err := s.Store.Finish(ctx, execID, results, done, status, msg); errors.Is(err, sweep.ErrSwept) {
			slog.Warn("run outlived sweep, results dropped", "execution", execID)
			return
		} else if err != nil {
			slog.Error("finish execution", "execution", execID, "err", err)
		}
		after(ctx, done, ok)
	}()
}

func (s *Server) executions(w http.ResponseWriter, r *http.Request) {
	pos := r.URL.Query().Get("position_id")
	if pos != "" && !isUUID(pos) {
		fail(w, "executions", ErrBadRequest)
		return
	}
	v, err := s.Store.Executions(r.Context(), account(r), pos)
	reply(w, "executions", v, err)
}

func (s *Server) execution(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		v, err := s.Store.Execution(r.Context(), account(r), id)
		reply(w, "execution", v, err)
	}
}
