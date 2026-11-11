package sched

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

type Store struct{ DB *pgxpool.Pool }

// Due is an active schedule whose slot has come, for a user with a live executor key.
type Due struct {
	ID, Account, Sub, Kind string
	BasketID, PositionID   *string
	Params                 json.RawMessage
	Every                  int64 // seconds
	Next                   time.Time
}

func (p Store) Due(ctx context.Context, now time.Time) ([]Due, error) {
	rows, err := p.DB.Query(ctx, `select s.id::text, s.account_id, u.sub_account, s.kind, s.basket_id::text, s.position_id::text,
  s.params, extract(epoch from s.every)::bigint, s.next_run_at
from schedules s join users u on u.account_id = s.account_id
where s.active and s.next_run_at <= $1 and u.key_status = 'active' and u.sub_account is not null
order by s.next_run_at limit 500`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (d Due, err error) {
		return d, r.Scan(&d.ID, &d.Account, &d.Sub, &d.Kind, &d.BasketID, &d.PositionID, &d.Params, &d.Every, &d.Next)
	})
}

// Revoke marks the executor key gone and stops every schedule of account.
func (p Store) Revoke(ctx context.Context, account string) error {
	return pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update users set key_status='revoked', updated_at=now() where account_id=$1`, account); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `update schedules set active=false where account_id=$1 and active`, account)
		return err
	})
}

func (p Store) Basket(ctx context.Context, id string) (def planner.Basket, creator string, err error) {
	err = p.DB.QueryRow(ctx, `select definition, creator from baskets where id=$1`, id).Scan(&def, &creator)
	return
}

// Position is an open position with no run in flight; ok false otherwise.
func (p Store) Position(ctx context.Context, id string) (def planner.Basket, held []string, ok bool, err error) {
	err = p.DB.QueryRow(ctx, `select b.definition, p.held from positions p join baskets b on b.id = p.basket_id
where p.id=$1 and p.state='open' and not exists(select 1 from executions where position_id=p.id and status in ('pending','running'))`, id).
		Scan(&def, &held)
	if errors.Is(err, pgx.ErrNoRows) {
		return def, nil, false, nil
	}
	return def, held, err == nil, err
}

// Job is one execution a schedule fires.
type Job struct {
	Kind       string   // executions.kind: dca | deploy | rebalance
	PositionID string   // rebalance target; "" otherwise
	Held       []string // rebalance: held the plan was built on; changed since = skip
	Open       string   // recurring buy: deposit atoms of the position this deploy opens
	Params     map[string]any
	Steps      []planner.Step
}

// Claim takes slot d.Next by moving next_run_at to next, and inserts the job's execution, in one tx.
// execID "" means another instance won the slot or the position got busy: nothing to run.
func (p Store) Claim(ctx context.Context, d Due, next time.Time, j *Job) (execID string, err error) {
	err = pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		if j.PositionID != "" {
			// same row lock as the api withdraw claim, so the two never overlap
			var busy bool
			err := tx.QueryRow(ctx, `select exists(select 1 from executions where position_id=$1 and status in ('pending','running'))
from positions where id=$1 and state='open' and held=$2::jsonb for update`, j.PositionID, j.Held).Scan(&busy)
			if errors.Is(err, pgx.ErrNoRows) || busy {
				return nil
			}
			if err != nil {
				return err
			}
		}
		t, err := tx.Exec(ctx, `update schedules set next_run_at=$2 where id=$1 and active and next_run_at=$3`, d.ID, next, d.Next)
		if err != nil || t.RowsAffected() == 0 {
			return err
		}
		if j.Open != "" {
			if err := tx.QueryRow(ctx, `insert into positions (account_id, basket_id, deposit) values ($1,$2,$3) returning id::text`,
				d.Account, d.BasketID, j.Open).Scan(&j.PositionID); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `insert into executions (account_id, position_id, kind, status, params) values ($1,nullif($2,'')::uuid,$3,'running',$4)
returning id::text`, d.Account, j.PositionID, j.Kind, j.Params).Scan(&execID); err != nil {
			return err
		}
		for i, st := range j.Steps {
			if _, err := tx.Exec(ctx, `insert into execution_steps (execution_id, idx, step) values ($1,$2,$3)`, execID, i, st); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		execID = ""
	}
	return
}

// Finish records executor results; done is how many leading steps succeeded.
func (p Store) Finish(ctx context.Context, execID string, results []executor.Result, done int, status, errMsg string) error {
	return pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		for _, r := range results {
			if _, err := tx.Exec(ctx, `update execution_steps set status=$3, tx_hash=nullif($4,''), error=nullif($5,''), updated_at=now()
where execution_id=$1 and idx=$2`, execID, r.Index, r.Status, r.TxHash, r.Error); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `update executions set status=$2, steps_done=$3, error=nullif($4,''), updated_at=now() where id=$1`,
			execID, status, done, errMsg)
		return err
	})
}

// Settle applies a finished run: held set on success (nil keeps it), state when non-empty, fee booked once paid.
func (p Store) Settle(ctx context.Context, posID, state string, held []string, fee *Fee) error {
	return pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		if posID != "" && (state != "" || held != nil) {
			var h any // untyped nil = SQL null, keeps held
			if held != nil {
				h = held
			}
			if _, err := tx.Exec(ctx, `update positions set state=coalesce(nullif($2,''), state), held=coalesce($3::jsonb, held), updated_at=now() where id=$1`,
				posID, state, h); err != nil {
				return err
			}
		}
		if fee == nil {
			return nil
		}
		_, err := tx.Exec(ctx, `insert into fee_ledger (execution_id, position_id, token, amount, creator, creator_share)
values ($1,nullif($2,'')::uuid,$3,$4,$5,$6) on conflict do nothing`, fee.ExecID, posID, fee.Token, fee.Amount, fee.Creator, fee.CreatorShare)
		return err
	})
}

type Fee struct {
	ExecID, Token, Amount, CreatorShare string
	Creator                             *string
}
