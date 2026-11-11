// Package store persists indexed events, the block cursor and reconciled account state in Postgres.
package store

import (
	"context"
	"errors"
	"math/big"

	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/services/indexer/internal/decode"
	"github.com/Debanjannnn/nearly/services/indexer/internal/reconcile"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const cursorName = "events"

type Store struct{ DB *pgxpool.Pool }

// Cursor is the last processed height; ok=false before the first block.
func (s Store) Cursor(ctx context.Context) (h uint64, ok bool, err error) {
	err = s.DB.QueryRow(ctx, `select height from indexer_cursor where name = $1`, cursorName).Scan(&h)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return h, err == nil, err
}

// Save stores a block's events and moves the cursor in one tx, so a crash never skips or doubles a block.
func (s Store) Save(ctx context.Context, height uint64, evs []decode.Event) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		b := &pgx.Batch{}
		for _, e := range evs {
			b.Queue(`insert into chain_events (receipt_id, log_index, block_height, block_time, tx_hash, account_id, event, data)
				values ($1, $2, $3, $4, nullif($5, ''), $6, $7, $8) on conflict do nothing`,
				e.ReceiptID, e.LogIndex, e.Height, e.Time, e.TxHash, e.Account, e.Event, e.Data)
			if e.Event == "revoke" {
				b.Queue(`update users set key_status = 'revoked', updated_at = now() where sub_account = $1`, e.Account)
			}
		}
		b.Queue(`insert into indexer_cursor (name, height) values ($1, $2)
			on conflict (name) do update set height = excluded.height`, cursorName, height)
		return tx.SendBatch(ctx, b).Close()
	})
}

// Idle lists sub-accounts with no pending or running execution; empty only = all of them.
func (s Store) Idle(ctx context.Context, only []string) ([]string, error) {
	rows, err := s.DB.Query(ctx, `select u.sub_account from users u
		where u.sub_account is not null and (coalesce(cardinality($1::text[]), 0) = 0 or u.sub_account = any($1))
		and not exists (select 1 from executions e where e.account_id = u.account_id and e.status in ('pending', 'running'))`, only)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// OpenPositions of a sub-account, with each basket's legs.
func (s Store) OpenPositions(ctx context.Context, sub string) ([]reconcile.Position, error) {
	rows, err := s.DB.Query(ctx, `select p.id::text, b.definition, p.held from positions p
		join baskets b on b.id = p.basket_id join users u on u.account_id = p.account_id
		where u.sub_account = $1 and p.state = 'open' order by p.created_at`, sub)
	if err != nil {
		return nil, err
	}
	var out []reconcile.Position
	var id string
	var def planner.Basket
	var held []string
	_, err = pgx.ForEachRow(rows, []any{&id, &def, &held}, func() error {
		p := reconcile.Position{ID: id, Legs: def.Legs}
		if len(held) != len(def.Legs) {
			return nil // never settled; nothing to correct
		}
		for _, h := range held {
			v, ok := new(big.Int).SetString(h, 10)
			if !ok {
				return errors.New("position " + id + ": bad held " + h)
			}
			p.Held = append(p.Held, v)
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

// Apply writes the ledger snapshot and corrected held. Held only changes if it still equals what
// was read, so a settlement the api wrote in between wins.
func (s Store) Apply(ctx context.Context, sub string, ledger, idle map[string]*big.Int, ps []reconcile.Position, held [][]*big.Int) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		b := &pgx.Batch{}
		b.Queue(`delete from account_slots where account_id = $1`, sub)
		for slot, v := range ledger {
			i := idle[slot]
			if i == nil {
				i = new(big.Int)
			}
			b.Queue(`insert into account_slots (account_id, slot, amount, idle) values ($1, $2, $3::numeric, $4::numeric)`,
				sub, slot, v.String(), i.String())
		}
		for i, p := range ps {
			b.Queue(`update positions set held = $2, updated_at = now() where id = $1 and state = 'open' and held = $3 and held <> $2`,
				p.ID, strs(held[i]), strs(p.Held))
		}
		return tx.SendBatch(ctx, b).Close()
	})
}

func strs(xs []*big.Int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.String()
	}
	return out
}
