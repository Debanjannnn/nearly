package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/internal/shared/sweep"
)

type Store struct{ DB *pgxpool.Pool }

type User struct {
	AccountID  string  `json:"account_id"`
	SubAccount *string `json:"sub_account"`
	KeyStatus  string  `json:"key_status"`
}

type Basket struct {
	ID         string         `json:"id"`
	Creator    string         `json:"creator"`
	Name       string         `json:"name"`
	Definition planner.Basket `json:"definition"`
	Public     bool           `json:"public"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Position struct {
	ID         string    `json:"id"`
	BasketID   string    `json:"basket_id"`
	BasketName string    `json:"basket_name"`
	State      string    `json:"state"`
	Deposit    string    `json:"deposit"`
	Held       []string  `json:"held"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Execution struct {
	ID         string          `json:"id"`
	PositionID *string         `json:"position_id"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	Params     json.RawMessage `json:"params"`
	StepsDone  int             `json:"steps_done"`
	Error      *string         `json:"error"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Steps      []ExecStep      `json:"steps,omitempty"`
}

type ExecStep struct {
	Index  int          `json:"index"`
	Step   planner.Step `json:"step"`
	Status string       `json:"status"`
	TxHash *string      `json:"tx_hash"`
	Error  *string      `json:"error"`
}

type Schedule struct {
	ID           string          `json:"id"`
	BasketID     *string         `json:"basket_id"`
	PositionID   *string         `json:"position_id"`
	Kind         string          `json:"kind"`
	Params       json.RawMessage `json:"params"`
	EverySeconds int64           `json:"every_seconds"`
	NextRunAt    time.Time       `json:"next_run_at"`
	Active       bool            `json:"active"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ExecParams is executions.params for deploy and withdraw runs.
type ExecParams struct {
	Amount    string   `json:"amount,omitempty"`     // deploy: base atoms in
	Fee       string   `json:"fee,omitempty"`        // deploy: open fee atoms
	Held      []string `json:"held,omitempty"`       // deploy: per-leg held once done
	Bps       int      `json:"bps,omitempty"`        // withdraw: part of each leg
	HeldAfter []string `json:"held_after,omitempty"` // withdraw: per-leg held once done
	ResumeOf  string   `json:"resume_of,omitempty"`  // withdraw: failed run this one continues
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func collect[T any](rows pgx.Rows, err error, fn pgx.RowToFunc[T]) ([]T, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, fn)
	if out == nil {
		out = []T{}
	}
	return out, err
}

func one[T any](rows pgx.Rows, err error, fn pgx.RowToFunc[T]) (T, error) {
	if err != nil {
		var zero T
		return zero, err
	}
	v, err := pgx.CollectExactlyOneRow(rows, fn)
	return v, notFound(err)
}

// ---- users / auth

func (p Store) EnsureUser(ctx context.Context, a string) error {
	_, err := p.DB.Exec(ctx, `insert into users (account_id) values ($1) on conflict do nothing`, a)
	return err
}

// IssueNonce stores a fresh login nonce (ttl is the column default) and drops expired unused ones.
func (p Store) IssueNonce(ctx context.Context, nonce []byte) error {
	_, err := p.DB.Exec(ctx, `with gc as (delete from auth_nonces where account_id is null and expires_at < now())
		insert into auth_nonces (nonce) values ($1)`, nonce)
	return err
}

// UseNonce consumes an issued, unexpired, unused nonce for a; ErrNonce otherwise.
func (p Store) UseNonce(ctx context.Context, nonce []byte, a string) error {
	tag, err := p.DB.Exec(ctx, `update auth_nonces set account_id=$2
		where nonce=$1 and account_id is null and expires_at > now()`, nonce, a)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNonce
	}
	return err
}

func (p Store) User(ctx context.Context, a string) (User, error) {
	var u User
	err := p.DB.QueryRow(ctx, `select account_id, sub_account, key_status from users where account_id=$1`, a).
		Scan(&u.AccountID, &u.SubAccount, &u.KeyStatus)
	return u, notFound(err)
}

// SetAccount stores the factory sub-account (sub may be "" to only move key_status).
func (p Store) SetAccount(ctx context.Context, a, sub, status string) error {
	_, err := p.DB.Exec(ctx, `update users set sub_account=coalesce(nullif($2,''), sub_account), key_status=$3, updated_at=now()
where account_id=$1`, a, sub, status)
	return err
}

// Revoke marks the executor key gone and stops every schedule; returns schedules deactivated.
func (p Store) Revoke(ctx context.Context, a string) (n int64, err error) {
	err = pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `update users set key_status='revoked', updated_at=now() where account_id=$1`, a); err != nil {
			return err
		}
		t, err := tx.Exec(ctx, `update schedules set active=false where account_id=$1 and active`, a)
		n = t.RowsAffected()
		return err
	})
	return
}

// ---- baskets

const (
	basketFields = `id::text, creator, name, definition, public, created_at`
	basketCols   = basketFields + ` from baskets`
)

func scanBasket(r pgx.CollectableRow) (Basket, error) {
	var b Basket
	return b, r.Scan(&b.ID, &b.Creator, &b.Name, &b.Definition, &b.Public, &b.CreatedAt)
}

func (p Store) CreateBasket(ctx context.Context, creator, name string, def planner.Basket, public bool) (Basket, error) {
	rows, err := p.DB.Query(ctx, `insert into baskets (creator, name, definition, public) values ($1,$2,$3,$4) returning `+basketFields,
		creator, name, def, public)
	return one(rows, err, scanBasket)
}

func (p Store) PublicBaskets(ctx context.Context) ([]Basket, error) {
	rows, err := p.DB.Query(ctx, `select `+basketCols+` where public order by created_at desc limit 200`)
	return collect(rows, err, scanBasket)
}

func (p Store) MyBaskets(ctx context.Context, a string) ([]Basket, error) {
	rows, err := p.DB.Query(ctx, `select `+basketCols+` where creator=$1 order by created_at desc limit 200`, a)
	return collect(rows, err, scanBasket)
}

// Basket returns a public basket, or a private one to its creator (viewer may be "").
func (p Store) Basket(ctx context.Context, id, viewer string) (Basket, error) {
	rows, err := p.DB.Query(ctx, `select `+basketCols+` where id=$1 and (public or creator=$2)`, id, viewer)
	return one(rows, err, scanBasket)
}

// ---- positions + executions

const positionCols = `p.id::text, p.basket_id::text, b.name, p.state, p.deposit::text, p.held, p.created_at, p.updated_at
from positions p join baskets b on b.id=p.basket_id`

func scanPosition(r pgx.CollectableRow) (Position, error) {
	var x Position
	return x, r.Scan(&x.ID, &x.BasketID, &x.BasketName, &x.State, &x.Deposit, &x.Held, &x.CreatedAt, &x.UpdatedAt)
}

func (p Store) Positions(ctx context.Context, a string) ([]Position, error) {
	rows, err := p.DB.Query(ctx, `select `+positionCols+` where p.account_id=$1 order by p.created_at desc limit 200`, a)
	return collect(rows, err, scanPosition)
}

func (p Store) Position(ctx context.Context, a, id string) (Position, error) {
	rows, err := p.DB.Query(ctx, `select `+positionCols+` where p.account_id=$1 and p.id=$2`, a, id)
	return one(rows, err, scanPosition)
}

func insertExec(ctx context.Context, tx pgx.Tx, a string, posID, kind string, params ExecParams, steps []planner.Step) (string, error) {
	var id string
	if err := tx.QueryRow(ctx, `insert into executions (account_id, position_id, kind, status, params) values ($1,$2,$3,'running',$4) returning id::text`,
		a, posID, kind, params).Scan(&id); err != nil {
		return "", err
	}
	for i, st := range steps {
		if _, err := tx.Exec(ctx, `insert into execution_steps (execution_id, idx, step) values ($1,$2,$3)`, id, i, st); err != nil {
			return "", err
		}
	}
	return id, nil
}

// OpenPosition inserts an opening position and its running deploy execution.
func (p Store) OpenPosition(ctx context.Context, a, basketID string, params ExecParams, steps []planner.Step) (posID, execID string, err error) {
	err = pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `insert into positions (account_id, basket_id, deposit) values ($1,$2,$3) returning id::text`,
			a, basketID, params.Amount).Scan(&posID); err != nil {
			return err
		}
		execID, err = insertExec(ctx, tx, a, posID, "deploy", params, steps)
		return err
	})
	return
}

// LastWithdraw is a position's latest withdraw run.
type LastWithdraw struct {
	ID        string
	Status    string
	Params    ExecParams
	Steps     []planner.Step
	StepsDone int
}

// WithdrawPlan builds the run for a locked position; nil steps with nil error = nothing left on chain.
type WithdrawPlan func(pos Position, def planner.Basket, last *LastWithdraw) ([]planner.Step, ExecParams, error)

// ClaimWithdraw locks the position, plans via build, and starts the run; execID "" means nothing to run
// (position settled to params.HeldAfter instead).
func (p Store) ClaimWithdraw(ctx context.Context, a, posID string, build WithdrawPlan) (execID string, steps []planner.Step, err error) {
	err = pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		var def planner.Basket
		rows, err := tx.Query(ctx, `select `+positionCols+` where p.account_id=$1 and p.id=$2 for update of p`, a, posID)
		pos, err := one(rows, err, scanPosition)
		if err != nil {
			return err
		}
		var busy bool
		if err := tx.QueryRow(ctx, `select b.definition, exists(select 1 from executions where position_id=$1 and status in ('pending','running'))
from positions p join baskets b on b.id=p.basket_id where p.id=$1`, posID).Scan(&def, &busy); err != nil {
			return err
		}
		if busy {
			return ErrBusy
		}
		var last *LastWithdraw
		switch pos.State {
		case "open":
		case "withdrawing":
			// no withdraw run yet = a dead deploy the sweep handed over for unwinding
			if last, err = lastWithdraw(ctx, tx, posID); errors.Is(err, ErrNotFound) {
				last, err = nil, nil
			}
			if err != nil {
				return err
			}
		default:
			return ErrNotOpen
		}
		var params ExecParams
		if steps, params, err = build(pos, def, last); err != nil {
			return err
		}
		if len(steps) == 0 {
			return settleWithdraw(ctx, tx, posID, params.HeldAfter)
		}
		if _, err := tx.Exec(ctx, `update positions set state='withdrawing', updated_at=now() where id=$1`, posID); err != nil {
			return err
		}
		execID, err = insertExec(ctx, tx, a, posID, "withdraw", params, steps)
		return err
	})
	return
}

func lastWithdraw(ctx context.Context, tx pgx.Tx, posID string) (*LastWithdraw, error) {
	var w LastWithdraw
	err := tx.QueryRow(ctx, `select id::text, status, params, steps_done from executions where position_id=$1 and kind='withdraw'
order by created_at desc limit 1`, posID).Scan(&w.ID, &w.Status, &w.Params, &w.StepsDone)
	if err != nil {
		return nil, notFound(err)
	}
	rows, err := tx.Query(ctx, `select step from execution_steps where execution_id=$1 order by idx`, w.ID)
	w.Steps, err = collect(rows, err, pgx.RowTo[planner.Step])
	return &w, err
}

// settleWithdraw sets held; all-zero held closes the position.
func settleWithdraw(ctx context.Context, tx pgx.Tx, posID string, held []string) error {
	_, err := tx.Exec(ctx, `update positions set held=$2, updated_at=now(),
  state=case when not exists(select 1 from jsonb_array_elements_text($2::jsonb) v where v <> '0') then 'closed' else 'open' end
where id=$1`, posID, held)
	return err
}

// Finish records executor results; done is how many leading steps succeeded. sweep.ErrSwept = sweep got there first.
func (p Store) Finish(ctx context.Context, execID string, results []executor.Result, done int, status, errMsg string) error {
	return pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `update executions set status=$2, steps_done=$3, error=nullif($4,''), updated_at=now() where id=$1 and status='running'`,
			execID, status, done, errMsg)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return sweep.ErrSwept
		}
		for _, r := range results {
			if _, err := tx.Exec(ctx, `update execution_steps set status=$3, tx_hash=nullif($4,''), error=nullif($5,''), updated_at=now()
where execution_id=$1 and idx=$2`, execID, r.Index, r.Status, r.TxHash, r.Error); err != nil {
				return err
			}
		}
		return nil
	})
}

// SettleDeploy opens the position with held, or closes it on failure; fee lands in fee_ledger once paid.
// stranded = a failed deploy already moved funds into venues; it stays open so withdraw (capped by live
// balances on resume) pulls them out. Otherwise a failed deploy closes and funds sit in the account wallet.
func (p Store) SettleDeploy(ctx context.Context, posID, execID string, ok, stranded bool, held []string, fee *FeeRow) error {
	return pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error {
		state, h := "closed", []string{}
		if ok || stranded {
			state, h = "open", held
		}
		if _, err := tx.Exec(ctx, `update positions set state=$2, held=$3, updated_at=now() where id=$1`, posID, state, h); err != nil {
			return err
		}
		if fee == nil {
			return nil
		}
		_, err := tx.Exec(ctx, `insert into fee_ledger (execution_id, position_id, token, amount, creator, creator_share)
values ($1,$2,$3,$4,$5,$6) on conflict do nothing`, execID, posID, fee.Token, fee.Amount, fee.Creator, fee.CreatorShare)
		return err
	})
}

// SettleWithdraw applies a finished withdraw: done sets held, failed leaves it withdrawing for a resume.
func (p Store) SettleWithdraw(ctx context.Context, posID string, ok bool, heldAfter []string) error {
	if !ok {
		return nil
	}
	return pgx.BeginFunc(ctx, p.DB, func(tx pgx.Tx) error { return settleWithdraw(ctx, tx, posID, heldAfter) })
}

type FeeRow struct {
	Token, Amount, CreatorShare string
	Creator                     *string
}

const execCols = `id::text, position_id::text, kind, status, params, steps_done, error, created_at, updated_at from executions`

func scanExec(r pgx.CollectableRow) (Execution, error) {
	var x Execution
	return x, r.Scan(&x.ID, &x.PositionID, &x.Kind, &x.Status, &x.Params, &x.StepsDone, &x.Error, &x.CreatedAt, &x.UpdatedAt)
}

// Executions lists newest first; positionID "" = all.
func (p Store) Executions(ctx context.Context, a, positionID string) ([]Execution, error) {
	rows, err := p.DB.Query(ctx, `select `+execCols+` where account_id=$1 and ($2='' or position_id::text=$2)
order by created_at desc limit 200`, a, positionID)
	return collect(rows, err, scanExec)
}

func (p Store) Execution(ctx context.Context, a, id string) (Execution, error) {
	rows, err := p.DB.Query(ctx, `select `+execCols+` where account_id=$1 and id=$2`, a, id)
	x, err := one(rows, err, scanExec)
	if err != nil {
		return x, err
	}
	rows, err = p.DB.Query(ctx, `select idx, step, status, tx_hash, error from execution_steps where execution_id=$1 order by idx`, id)
	x.Steps, err = collect(rows, err, func(r pgx.CollectableRow) (ExecStep, error) {
		var s ExecStep
		return s, r.Scan(&s.Index, &s.Step, &s.Status, &s.TxHash, &s.Error)
	})
	return x, err
}

// ---- schedules

const scheduleCols = `id::text, basket_id::text, position_id::text, kind, params, extract(epoch from every)::bigint,
next_run_at, active, created_at from schedules`

func scanSchedule(r pgx.CollectableRow) (Schedule, error) {
	var x Schedule
	return x, r.Scan(&x.ID, &x.BasketID, &x.PositionID, &x.Kind, &x.Params, &x.EverySeconds, &x.NextRunAt, &x.Active, &x.CreatedAt)
}

func (p Store) Schedules(ctx context.Context, a string) ([]Schedule, error) {
	rows, err := p.DB.Query(ctx, `select `+scheduleCols+` where account_id=$1 order by created_at desc`, a)
	return collect(rows, err, scanSchedule)
}

func (p Store) Schedule(ctx context.Context, a, id string) (Schedule, error) {
	rows, err := p.DB.Query(ctx, `select `+scheduleCols+` where account_id=$1 and id=$2`, a, id)
	return one(rows, err, scanSchedule)
}

func (p Store) CreateSchedule(ctx context.Context, a string, in ScheduleInput) (Schedule, error) {
	var id string
	err := p.DB.QueryRow(ctx, `insert into schedules (account_id, basket_id, position_id, kind, params, every, next_run_at)
values ($1,$2,$3,$4,$5, make_interval(secs => $6), now() + make_interval(secs => $6)) returning id::text`,
		a, in.BasketID, in.PositionID, in.Kind, in.Params, in.EverySeconds).Scan(&id)
	if err != nil {
		return Schedule{}, err
	}
	return p.Schedule(ctx, a, id)
}

// UpdateSchedule changes every and/or active; a new every restarts the countdown.
func (p Store) UpdateSchedule(ctx context.Context, a, id string, every *int64, active *bool) (Schedule, error) {
	t, err := p.DB.Exec(ctx, `update schedules set
  every=coalesce(make_interval(secs => $3::bigint), every),
  next_run_at=case when $3::bigint is null then next_run_at else now() + make_interval(secs => $3::bigint) end,
  active=coalesce($4, active)
where account_id=$1 and id=$2`, a, id, every, active)
	if err == nil && t.RowsAffected() == 0 {
		err = ErrNotFound
	}
	if err != nil {
		return Schedule{}, err
	}
	return p.Schedule(ctx, a, id)
}

func (p Store) DeleteSchedule(ctx context.Context, a, id string) error {
	t, err := p.DB.Exec(ctx, `delete from schedules where account_id=$1 and id=$2`, a, id)
	if err == nil && t.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}
