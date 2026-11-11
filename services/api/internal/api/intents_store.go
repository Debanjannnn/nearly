package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type Intent struct {
	DepositAddress string          `json:"deposit_address"`
	DepositMemo    string          `json:"deposit_memo,omitempty"`
	Kind           string          `json:"kind"`
	Status         string          `json:"status"`
	DepositTx      *string         `json:"deposit_tx"`
	Quote          json.RawMessage `json:"quote"`
	SwapDetails    json.RawMessage `json:"swap_details"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

const intentCols = `deposit_address, deposit_memo, kind, status, deposit_tx, quote, swap_details, created_at, updated_at from intents`

func scanIntent(r pgx.CollectableRow) (Intent, error) {
	var x Intent
	return x, r.Scan(&x.DepositAddress, &x.DepositMemo, &x.Kind, &x.Status, &x.DepositTx, &x.Quote, &x.SwapDetails, &x.CreatedAt, &x.UpdatedAt)
}

func (p Store) AddIntent(ctx context.Context, a, kind, addr, memo string, quote json.RawMessage) error {
	_, err := p.DB.Exec(ctx, `insert into intents (account_id, kind, deposit_address, deposit_memo, quote) values ($1,$2,$3,$4,$5)`,
		a, kind, addr, memo, quote)
	return err
}

// Intents lists the user's swaps, newest first; addr "" means all.
func (p Store) Intents(ctx context.Context, a, addr string) ([]Intent, error) {
	rows, err := p.DB.Query(ctx, `select `+intentCols+` where account_id=$1 and ($2='' or deposit_address=$2)
order by created_at desc limit 50`, a, addr)
	return collect(rows, err, scanIntent)
}

func (p Store) Intent(ctx context.Context, a, addr, memo string) (Intent, error) {
	rows, err := p.DB.Query(ctx, `select `+intentCols+` where account_id=$1 and deposit_address=$2 and deposit_memo=$3`, a, addr, memo)
	return one(rows, err, scanIntent)
}

// SetIntentStatus stores a 1Click status; tx "" keeps the known deposit tx.
func (p Store) SetIntentStatus(ctx context.Context, x *Intent, status, tx string, details json.RawMessage) error {
	return p.DB.QueryRow(ctx, `update intents set status=$3, deposit_tx=coalesce(nullif($4,''), deposit_tx),
swap_details=coalesce($5, swap_details), updated_at=now() where deposit_address=$1 and deposit_memo=$2
returning status, deposit_tx, swap_details, updated_at`, x.DepositAddress, x.DepositMemo, status, tx, details).
		Scan(&x.Status, &x.DepositTx, &x.SwapDetails, &x.UpdatedAt)
}
