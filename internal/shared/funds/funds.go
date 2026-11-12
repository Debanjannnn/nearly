// Package funds is what an account may spend: ledger minus what positions and in-flight buys keep.
package funds

import (
	"context"
	"errors"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

// ErrShort means the amount is more than the account's free base balance.
var ErrShort = errors.New("insufficient_balance")

type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Locked sums per token key what the account must not spend: hold legs of unclosed positions,
// deposits of positions still opening, and amounts of DCA runs in flight.
func Locked(ctx context.Context, db DB, account string) (map[string]*big.Int, error) {
	out := map[string]*big.Int{}
	add := func(k, s string) {
		if v, ok := new(big.Int).SetString(s, 10); ok {
			if out[k] == nil {
				out[k] = new(big.Int)
			}
			out[k].Add(out[k], v)
		}
	}
	var dca string
	if err := db.QueryRow(ctx, `select coalesce(sum((params->>'amount')::numeric), 0)::text from executions
		where account_id=$1 and kind='dca' and status in ('pending','running')`, account).Scan(&dca); err != nil {
		return nil, err
	}
	add(planner.Base, dca)
	rows, err := db.Query(ctx, `select b.definition, p.held, p.state, p.deposit::text from positions p join baskets b on b.id=p.basket_id
		where p.account_id=$1 and p.state <> 'closed'`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var def planner.Basket
		var held []string
		var state, deposit string
		if err := rows.Scan(&def, &held, &state, &deposit); err != nil {
			return nil, err
		}
		if state == "opening" { // held is set at settle; until then the deposit is spoken for
			add(planner.Base, deposit)
		}
		for i, l := range def.Legs {
			if l.Source == planner.SourceHold && i < len(held) {
				add(l.Token, held[i])
			}
		}
	}
	return out, rows.Err()
}

// Free is bals minus locked per token key, floored at 0; unparsable balances read as 0.
func Free(bals map[string]string, locked map[string]*big.Int) map[string]string {
	out := map[string]string{}
	for k, b := range bals {
		v, ok := new(big.Int).SetString(b, 10)
		if !ok {
			v = new(big.Int)
		}
		if h := locked[k]; h != nil {
			v.Sub(v, h)
		}
		if v.Sign() < 0 {
			v.SetInt64(0)
		}
		out[k] = v.String()
	}
	return out
}

// Reserve serializes base spends of account until tx ends, then errs ErrShort unless amount fits free base.
// Locked is read before the chain balance: a run settling in between only makes the check stricter.
func Reserve(ctx context.Context, tx pgx.Tx, account, amount string, base func(context.Context) (string, error)) error {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext('funds:' || $1))`, account); err != nil {
		return err
	}
	locked, err := Locked(ctx, tx, account)
	if err != nil {
		return err
	}
	b, err := base(ctx)
	if err != nil {
		return err
	}
	amt, ok := new(big.Int).SetString(amount, 10)
	have, _ := new(big.Int).SetString(Free(map[string]string{planner.Base: b}, locked)[planner.Base], 10)
	if !ok || amt.Cmp(have) > 0 {
		return ErrShort
	}
	return nil
}
