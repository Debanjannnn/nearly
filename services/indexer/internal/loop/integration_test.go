//go:build integration

package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/pg/pgtest"
	"github.com/Debanjannnn/nearly/services/indexer/internal/source"
	"github.com/Debanjannnn/nearly/services/indexer/internal/store"
)

const (
	alice = "alice-testnet.factory.testnet"
	tip   = 277250759 // height of the fixture block; tip-1 is a skipped height
)

// fakeData serves neardata: tip is final, tip-1 is skipped (null).
func fakeData(t *testing.T) *source.Client {
	blk, err := os.ReadFile("../decode/testdata/block.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/last_block/final":
			http.Redirect(w, r, fmt.Sprintf("/v0/block/%d", tip), http.StatusFound)
		case fmt.Sprintf("/v0/block/%d", tip):
			w.Write(blk)
		case fmt.Sprintf("/v0/block/%d", tip-1):
			w.Write([]byte("null"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return source.New(srv.URL)
}

// fakeLedger answers get_balances / get_positions for alice from a mutable map.
type fakeLedger struct {
	mu   sync.Mutex
	rows map[string][][2]string
}

func (f *fakeLedger) client(t *testing.T) *near.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Account string `json:"account_id"`
				Method  string `json:"method_name"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		b, _ := json.Marshal(f.rows[req.Params.Method])
		f.mu.Unlock()
		raw := make([]int, len(b))
		for i := range b {
			raw[i] = int(b[i])
		}
		json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"result": raw}})
	}))
	t.Cleanup(srv.Close)
	return near.New(srv.URL)
}

func TestIndexAndReconcile(t *testing.T) {
	db := pgtest.DB(t)
	ctx := context.Background()
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	must(`insert into users (account_id, sub_account, key_status) values ('alice.testnet', $1, 'active')`, alice)
	var pos string
	def := `{"legs":[{"token":"usdc","source":"lending","weight_bps":5000},{"token":"wnear","source":"staking","weight_bps":3000,"pool_id":1900},{"token":"wnear","source":"hold","weight_bps":2000,"pool_id":1900}]}`
	if err := db.QueryRow(ctx, `with b as (insert into baskets (creator, name, definition) values ('alice.testnet', 'b', $1) returning id)
		insert into positions (account_id, basket_id, state, deposit, held) select 'alice.testnet', id, 'open', 1000, '["500","300","200"]' from b returning id`, def).Scan(&pos); err != nil {
		t.Fatal(err)
	}

	// the stake step failed and was refunded, so staking is empty; 70 usdc is an undeployed deposit
	led := &fakeLedger{rows: map[string][][2]string{
		"get_balances":  {{"usdcc.ft.ref-labs.testnet", "70"}, {"wrap.testnet", "200"}},
		"get_positions": {{"lending:usdcc.ft.ref-labs.testnet", "500"}},
	}}
	x := &Indexer{Src: fakeData(t), RPC: led.client(t), Store: store.Store{DB: db}, Factory: "factory.testnet",
		Tokens: map[string]string{"usdc": "usdcc.ft.ref-labs.testnet", "wnear": "wrap.testnet"}, Start: tip - 1, Batch: 10}
	if err := x.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	var status, held string
	db.QueryRow(ctx, `select count(*) from chain_events`).Scan(&n)
	db.QueryRow(ctx, `select key_status from users`).Scan(&status)
	if n != 3 || status != "revoked" {
		t.Fatalf("events %d key_status %s", n, status)
	}
	if h, _, _ := x.Store.Cursor(ctx); h != tip {
		t.Fatalf("cursor %d", h)
	}
	db.QueryRow(ctx, `select held::text from positions where id = $1`, pos).Scan(&held)
	if held != `["500", "0", "200"]` {
		t.Fatalf("held %s", held)
	}
	var idle string
	db.QueryRow(ctx, `select idle::text from account_slots where account_id = $1 and slot = 'balance:usdcc.ft.ref-labs.testnet'`, alice).Scan(&idle)
	if idle != "70" {
		t.Fatalf("idle %q", idle)
	}

	// replaying the same block is a no-op
	must(`update indexer_cursor set height = $1`, tip-1)
	if err := x.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if db.QueryRow(ctx, `select count(*) from chain_events`).Scan(&n); n != 3 {
		t.Fatalf("replay duplicated events: %d", n)
	}

	// an execution in flight owns the position: reconcile must leave it alone
	must(`insert into executions (account_id, position_id, kind, status) values ('alice.testnet', $1, 'withdraw', 'running')`, pos)
	led.mu.Lock()
	led.rows["get_positions"] = nil
	led.mu.Unlock()
	if err := x.ReconcileAll(ctx, nil); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(ctx, `select held::text from positions where id = $1`, pos).Scan(&held)
	if held != `["500", "0", "200"]` {
		t.Fatalf("busy account reconciled: %s", held)
	}

	// once it settles, a full withdraw on chain empties the lending leg
	must(`update executions set status = 'succeeded'`)
	if err := x.ReconcileAll(ctx, nil); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(ctx, `select held::text from positions where id = $1`, pos).Scan(&held)
	if held != `["0", "0", "200"]` {
		t.Fatalf("after withdraw: %s", held)
	}
}
