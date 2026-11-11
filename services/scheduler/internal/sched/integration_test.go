//go:build integration

package sched

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

func testDB(t *testing.T) *pgxpool.Pool {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	schema := fmt.Sprintf("t_%d", time.Now().UnixNano())
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "drop schema "+schema+" cascade"); pool.Close() })
	files, _ := filepath.Glob("../../../../migrations/*.sql")
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no migrations")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	return pool
}

// fakeMarket: 1 usdc atom = 1e12 wnear atoms both ways.
type fakeMarket struct{ stale bool }

func (m *fakeMarket) Fresh(context.Context) error {
	if m.stale {
		return errors.New("stale")
	}
	return nil
}

func (m *fakeMarket) Quote(_ context.Context, _ uint64, in, _ string, a *big.Int) (*big.Int, error) {
	if in == "usdc.t" {
		return new(big.Int).Mul(a, big.NewInt(1e12)), nil
	}
	return new(big.Int).Quo(a, big.NewInt(1e12)), nil
}

type fakeKeys struct {
	mu     sync.Mutex
	active map[string]bool
}

func (k *fakeKeys) Active(_ context.Context, a string) (bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.active[a], nil
}

type fakeExec struct {
	mu    sync.Mutex
	calls []string // execution ids
}

func (f *fakeExec) Execute(_ context.Context, _, id string, steps []planner.Step) ([]executor.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, id)
	f.mu.Unlock()
	out := make([]executor.Result, len(steps))
	for i := range steps {
		out[i] = executor.Result{Index: i, Status: "ok", TxHash: fmt.Sprint("tx", i)}
	}
	return out, nil
}

func (f *fakeExec) n() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.calls) }

type env struct {
	t    *testing.T
	db   *pgxpool.Pool
	md   *fakeMarket
	keys *fakeKeys
	ex   *fakeExec
	now  time.Time
}

func newEnv(t *testing.T) *env {
	e := &env{t: t, db: testDB(t), md: &fakeMarket{}, keys: &fakeKeys{active: map[string]bool{"alice.root.testnet": true}},
		ex: &fakeExec{}, now: time.Now().Truncate(time.Second)}
	e.exec(`insert into users (account_id, sub_account, key_status) values ('alice.testnet', 'alice.root.testnet', 'active')`)
	return e
}

func (e *env) sched() *Scheduler {
	return &Scheduler{Store: Store{DB: e.db}, Market: e.md, Keys: e.keys, Exec: e.ex,
		Plan: &planner.Planner{Tokens: map[string]string{"usdc": "usdc.t", "wnear": "wnear.t"}, Q: e.md, SlippageBps: 50}}
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
}

func (e *env) id(sql string, args ...any) string {
	e.t.Helper()
	var id string
	if err := e.db.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
	return id
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

const basketDef = `{"legs":[{"token":"usdc","source":"lending","weight_bps":5000},{"token":"wnear","source":"hold","weight_bps":5000,"pool_id":1900}],"drift_bps":500}`

// seed: one schedule of each kind, all due one second ago; the rebalance position sits at 20/80.
func (e *env) seed() (basket, pos string) {
	basket = e.id(`insert into baskets (creator, name, definition) values ('alice.testnet','b',$1) returning id::text`, basketDef)
	pos = e.id(`insert into positions (account_id, basket_id, deposit, state, held) values ('alice.testnet',$1,'500000000','open',
  '["100000000","400000000000000000000"]') returning id::text`, basket)
	due := e.now.Add(-time.Second)
	for _, k := range []string{"dca", "recurring_buy"} {
		e.exec(`insert into schedules (account_id, basket_id, kind, params, every, next_run_at) values ('alice.testnet',$1,$2,'{"amount":"10000000"}','1 hour',$3)`,
			basket, k, due)
	}
	e.exec(`insert into schedules (account_id, position_id, kind, every, next_run_at) values ('alice.testnet',$1,'rebalance','1 hour',$2)`, pos, due)
	return
}

func TestEachKindFiresOnceAcrossInstances(t *testing.T) {
	e := newEnv(t)
	_, pos := e.seed()
	a, b := e.sched(), e.sched()
	var wg sync.WaitGroup
	for _, s := range []*Scheduler{a, b, a, b} {
		wg.Add(1)
		go func() { defer wg.Done(); s.Once(context.Background(), e.now) }()
	}
	wg.Wait()
	a.Wait()
	b.Wait()
	if n := e.ex.n(); n != 3 {
		t.Fatalf("executor calls = %d, want 3", n)
	}
	for kind, want := range map[string]int{"dca": 1, "deploy": 1, "rebalance": 1} {
		if n := e.count(`select count(*) from executions where kind=$1 and status='succeeded'`, kind); n != want {
			t.Errorf("%s executions = %d, want %d", kind, n, want)
		}
	}
	if n := e.count(`select count(*) from positions where state='open' and id <> $1`, pos); n != 1 {
		t.Errorf("recurring buy open positions = %d, want 1", n)
	}
	if n := e.count(`select count(*) from executions where kind='dca' and position_id is null`); n != 1 {
		t.Error("dca must not open a position")
	}
	if n := e.count(`select count(*) from fee_ledger`); n != 2 {
		t.Errorf("fee rows = %d, want 2 (dca + buy)", n)
	}
	if n := e.count(`select count(*) from schedules where next_run_at <= $1`, e.now); n != 0 {
		t.Error("every slot must advance")
	}
	if n := e.count(`select count(*) from positions where id=$1 and held <> '["100000000","400000000000000000000"]'`, pos); n != 1 {
		t.Error("rebalance must update held")
	}

	// next slot: dca + buy fire again, rebalanced position is inside the band
	later := e.now.Add(time.Hour)
	a.Once(context.Background(), later)
	a.Wait()
	if n := e.ex.n(); n != 5 {
		t.Fatalf("executor calls after next slot = %d, want 5", n)
	}
	if n := e.count(`select count(*) from executions where kind='rebalance'`); n != 1 {
		t.Error("balanced position must not rebalance again")
	}
	// same slot again: nothing
	b.Once(context.Background(), later)
	b.Wait()
	if n := e.ex.n(); n != 5 {
		t.Fatalf("slot fired twice: %d calls", n)
	}
}

func TestRebalanceMinInterval(t *testing.T) {
	e := newEnv(t)
	_, pos := e.seed()
	e.exec(`update schedules set active = kind = 'rebalance'`)
	// slot missed by 59m: the fire still waits a full hour, not the 1m left in the grid
	e.exec(`update schedules set next_run_at=$1`, e.now.Add(-59*time.Minute))
	s := e.sched()
	s.Once(context.Background(), e.now)
	s.Wait()
	// drift again right away: still waits for the interval
	e.exec(`update positions set held='["100000000","400000000000000000000"]' where id=$1`, pos)
	s.Once(context.Background(), e.now.Add(time.Minute))
	s.Wait()
	if n := e.ex.n(); n != 1 {
		t.Fatalf("calls = %d, want 1 inside min interval", n)
	}
	s.Once(context.Background(), e.now.Add(time.Hour))
	s.Wait()
	if n := e.ex.n(); n != 2 {
		t.Fatalf("calls = %d, want 2 after interval", n)
	}
}

func TestStaleMarketFiresNothing(t *testing.T) {
	e := newEnv(t)
	e.seed()
	e.md.stale = true
	s := e.sched()
	s.Once(context.Background(), e.now)
	s.Wait()
	if n := e.ex.n(); n != 0 {
		t.Fatalf("calls = %d on stale data", n)
	}
	if n := e.count(`select count(*) from schedules where next_run_at <= $1`, e.now); n != 3 {
		t.Error("stale tick must not consume slots")
	}
}

func TestRevokeStopsEverything(t *testing.T) {
	e := newEnv(t)
	e.seed()
	e.keys.active["alice.root.testnet"] = false
	s := e.sched()
	s.Once(context.Background(), e.now)
	s.Wait()
	if n := e.ex.n(); n != 0 {
		t.Fatalf("calls = %d after revoke", n)
	}
	if n := e.count(`select count(*) from schedules where active`); n != 0 {
		t.Errorf("active schedules = %d, want 0", n)
	}
	if n := e.count(`select count(*) from users where key_status='revoked'`); n != 1 {
		t.Error("user must be marked revoked")
	}
	// key back on chain does not restart: the user must re-enable through the api
	e.keys.active["alice.root.testnet"] = true
	s.Once(context.Background(), e.now.Add(2*time.Hour))
	s.Wait()
	if n := e.ex.n(); n != 0 {
		t.Fatalf("calls = %d after revoke", n)
	}
}
