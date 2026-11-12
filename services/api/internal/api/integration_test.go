//go:build integration

package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/internal/shared/sweep"
	"github.com/Debanjannnn/nearly/services/api/internal/auth"
)

// testDB applies every migration into a throwaway schema and returns a pool pinned to it.
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

// fakeChain: full-access keys, factory accounts and executor keys are plain maps; swaps quote 1 USDC = 0.1 NEAR.
type fakeChain struct {
	mu       sync.Mutex
	full     map[string]string // account -> full-access public key
	accounts map[string]string // owner -> sub
	execOn   map[string]bool   // sub -> executor key present
	live     planner.Live
	bals     map[string]string // ledger per token key; nil = plenty of usdc
}

func (f *fakeChain) Quote(_ context.Context, _ uint64, _, _ string, in *big.Int) (*big.Int, error) {
	return new(big.Int).Mul(in, big.NewInt(1e17)), nil // 1 USDC = 0.1 NEAR, near real rate
}
func (f *fakeChain) FullAccess(_ context.Context, a, pk string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.full[a] == pk, nil
}
func (f *fakeChain) ExecutorKeyActive(_ context.Context, sub string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.execOn[sub], nil
}
func (f *fakeChain) FactoryAccount(_ context.Context, owner string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.accounts[owner], nil
}
func (f *fakeChain) RequiredDeposit(context.Context) (string, error) {
	return "2000000000000000000000000", nil
}
func (f *fakeChain) Balances(context.Context, string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bals == nil {
		return map[string]string{"usdc": "1000000000000", "wnear": "6"}, nil
	}
	return f.bals, nil
}
func (f *fakeChain) Live(context.Context, string, []planner.Step) (planner.Live, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live, nil
}

// fakeExec fails at step failAt (-1 = never) and records every request.
type fakeExec struct {
	mu     sync.Mutex
	failAt int
	errMsg string // raw failure text; "" = venue_reverted
	calls  [][]planner.Step
}

func (f *fakeExec) handler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AccountID   string         `json:"account_id"`
		ExecutionID string         `json:"execution_id"`
		Steps       []planner.Step `json:"steps"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	f.calls = append(f.calls, in.Steps)
	failAt, msg := f.failAt, f.errMsg
	f.mu.Unlock()
	if msg == "" {
		msg = "venue_reverted"
	}
	var res []executor.Result
	for i := range in.Steps {
		if i == failAt {
			res = append(res, executor.Result{Index: i, Status: "failed", Error: msg})
			break
		}
		res = append(res, executor.Result{Index: i, Status: "ok", TxHash: fmt.Sprintf("tx%d", i)})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"results": res})
}

type env struct {
	t     *testing.T
	srv   *Server
	http  *httptest.Server
	chain *fakeChain
	exec  *fakeExec
	db    *pgxpool.Pool
}

func setup(t *testing.T) *env {
	db := testDB(t)
	fc := &fakeChain{full: map[string]string{}, accounts: map[string]string{}, execOn: map[string]bool{}}
	fe := &fakeExec{failAt: -1}
	exSrv := httptest.NewServer(httpx.RequireInternal("sec", "api")(http.HandlerFunc(fe.handler)))
	t.Cleanup(exSrv.Close)
	dep := &config.Deployments{Factory: "factory.t", Tokens: map[string]string{"usdc": "usdc.t", "wnear": "wrap.t"}}
	s := &Server{
		Store: Store{DB: db}, Chain: fc,
		Exec:     &executor.Client{URL: exSrv.URL, Secret: "sec", Caller: "api", HTTP: http.DefaultClient},
		Sessions: &auth.Sessions{Secret: []byte("test-secret"), TTL: time.Hour},
		Plan:     &planner.Planner{Tokens: dep.Tokens, Q: fc, SlippageBps: 100},
		Dep:      dep, ExecutorKey: "ed25519:exec",
	}
	mux := httpx.NewMux(nil)
	s.Routes(mux)
	h := httptest.NewServer(mux)
	t.Cleanup(h.Close)
	return &env{t, s, h, fc, fe, db}
}

func (e *env) do(method, path, tok string, body any, want int) map[string]any {
	e.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, e.http.URL+path, rd)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	raw := new(bytes.Buffer)
	raw.ReadFrom(res.Body)
	json.Unmarshal(raw.Bytes(), &out)
	if res.StatusCode != want {
		e.t.Fatalf("%s %s: status %d want %d: %s", method, path, res.StatusCode, want, raw)
	}
	if out == nil { // list replies
		out = map[string]any{"list": json.RawMessage(raw.Bytes())}
	}
	return out
}

// signed registers a fresh full-access key for acct and signs the login message over nonce.
func (e *env) signed(acct string, nonce []byte) map[string]string {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pk := auth.EncodeKey(pub)
	e.chain.mu.Lock()
	e.chain.full[acct] = pk
	e.chain.mu.Unlock()
	return map[string]string{
		"account_id": acct, "public_key": pk, "nonce": base64.StdEncoding.EncodeToString(nonce),
		"signature": base64.StdEncoding.EncodeToString(ed25519.Sign(priv, auth.Hash(auth.Message, auth.Recipient, nonce))),
	}
}

func (e *env) issue() []byte {
	n, err := base64.StdEncoding.DecodeString(e.do("GET", "/api/v1/auth/nonce", "", nil, 200)["nonce"].(string))
	if err != nil || len(n) != 32 {
		e.t.Fatal("bad nonce", err)
	}
	return n
}

// login signs a server-issued nonce; a replay of the same body is rejected.
func (e *env) login(acct string) string {
	body := e.signed(acct, e.issue())
	tok := e.do("POST", "/api/v1/auth", "", body, 200)["token"].(string)
	if out := e.do("POST", "/api/v1/auth", "", body, 401); out["error"] != "invalid_nonce" {
		e.t.Fatal("replay", out)
	}
	return tok
}

// onboard logs in and walks the factory flow to an active sub-account.
func (e *env) onboard(acct string) string {
	tok := e.login(acct)
	out := e.do("POST", "/api/v1/me/account", tok, nil, 202)
	if out["tx"] == nil {
		e.t.Fatal("want create_account tx", out)
	}
	e.chain.mu.Lock()
	e.chain.accounts[acct] = acct + ".factory.t" // mapped, create batch not landed yet
	e.chain.mu.Unlock()
	if out := e.do("POST", "/api/v1/me/account", tok, nil, 202); out["account_id"] != nil {
		e.t.Fatal("in-flight create must stay pending", out)
	}
	e.chain.mu.Lock()
	e.chain.execOn[acct+".factory.t"] = true
	e.chain.mu.Unlock()
	if out := e.do("POST", "/api/v1/me/account", tok, nil, 200); out["account_id"] != acct+".factory.t" || out["key_status"] != "active" {
		e.t.Fatal(out)
	}
	return tok
}

func TestAuth(t *testing.T) {
	e := setup(t)
	pub, priv, _ := ed25519.GenerateKey(nil)
	nonce := make([]byte, 32)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, auth.Hash(auth.Message, auth.Recipient, nonce)))
	body := map[string]string{"account_id": "eve.testnet", "public_key": auth.EncodeKey(pub), "nonce": base64.StdEncoding.EncodeToString(nonce), "signature": sig}
	e.do("POST", "/api/v1/auth", "", body, 401) // valid sig but not a full-access key of eve
	e.do("GET", "/api/v1/me", "", nil, 401)
	e.do("GET", "/api/v1/me", "garbage", nil, 401)
	if out := e.do("POST", "/api/v1/auth", "", e.signed("eve.testnet", auth.NewNonce()), 401); out["error"] != "invalid_nonce" {
		t.Fatal("unknown nonce", out)
	}
	old := e.issue()
	if _, err := e.db.Exec(context.Background(), `update auth_nonces set expires_at = now() - interval '1 second' where nonce=$1`, old); err != nil {
		t.Fatal(err)
	}
	if out := e.do("POST", "/api/v1/auth", "", e.signed("eve.testnet", old), 401); out["error"] != "invalid_nonce" {
		t.Fatal("expired nonce", out)
	}
	if out := e.do("POST", "/api/v1/auth", "", e.signed("eve.testnet", e.issue()), 200); out["token"] == nil {
		t.Fatal(out)
	}
	tok := e.login("alice.testnet")
	if out := e.do("GET", "/api/v1/me", tok, nil, 200); out["account_id"] != "alice.testnet" || out["key_status"] != "pending" && out["key_status"] != "none" {
		t.Fatal(out)
	}
}

var basket = map[string]any{
	"name":   "half half",
	"public": true,
	"definition": map[string]any{"legs": []map[string]any{
		{"token": "usdc", "source": "lending", "weight_bps": 5000},
		{"token": "wnear", "source": "staking", "weight_bps": 5000, "pool_id": 1900},
	}},
}

func (e *env) position(tok, id string) map[string]any {
	return e.do("GET", "/api/v1/me/positions/"+id, tok, nil, 200)
}

func TestDeployWithdrawResume(t *testing.T) {
	e := setup(t)
	creator := e.onboard("carol.testnet")
	b := e.do("POST", "/api/v1/me/baskets", creator, basket, 201)
	bid := b["id"].(string)
	e.do("POST", "/api/v1/me/baskets", creator, map[string]any{"name": "bad", "definition": map[string]any{"legs": []any{}}}, 400)

	tok := e.onboard("alice.testnet")
	e.do("GET", "/api/v1/baskets/"+bid, "", nil, 200)
	e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "-1"}, 400)

	// deploy 1000 USDC (6 dec): fee 4 USDC, half to creator
	out := e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "1000000000"}, 202)
	e.srv.Wait()
	pid, xid := out["position_id"].(string), out["execution_id"].(string)
	if out["fee"] != "4000000" {
		t.Fatal(out)
	}
	pos := e.position(tok, pid)
	if pos["state"] != "open" || fmt.Sprint(pos["held"]) != "[498000000 49302000000000000000000000]" {
		t.Fatal(pos)
	}
	x := e.do("GET", "/api/v1/me/executions/"+xid, tok, nil, 200)
	if x["status"] != "succeeded" || len(x["steps"].([]any)) != 4 {
		t.Fatal(x)
	}
	var amount, share, creatorID string
	if err := e.db.QueryRow(context.Background(), `select amount::text, creator_share::text, creator from fee_ledger where execution_id=$1`, xid).
		Scan(&amount, &share, &creatorID); err != nil || amount != "4000000" || share != "2000000" || creatorID != "carol.testnet" {
		t.Fatal(amount, share, creatorID, err)
	}

	// partial withdraw 50%
	e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, map[string]int{"bps": 5000}, 202)
	e.srv.Wait()
	if pos := e.position(tok, pid); pos["state"] != "open" || fmt.Sprint(pos["held"]) != "[249000000 24651000000000000000000000]" {
		t.Fatal(pos)
	}

	// full withdraw fails at step 1 (unstake): position stays withdrawing
	e.exec.failAt = 1
	e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, nil, 202)
	e.srv.Wait()
	if pos := e.position(tok, pid); pos["state"] != "withdrawing" {
		t.Fatal(pos)
	}
	// resume: lending already withdrawn, staking still there, usdc sits in the account
	e.exec.failAt = -1
	e.chain.live = planner.Live{
		Lent: map[string]string{}, Staked: "24651000000000000000000000",
		Wallet: map[string]string{"usdc.t": "249000000", "wrap.t": "0"},
	}
	out = e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, nil, 202)
	e.srv.Wait()
	if out["resumed"] != true {
		t.Fatal(out)
	}
	// no lst_pool: staked part unstakes (NEAR comes later via claim_unstaked), only usdc to owner
	if last := e.exec.calls[len(e.exec.calls)-1]; fmt.Sprint(last) != fmt.Sprint([]planner.Step{
		{Kind: planner.KindUnstake, Amount: "24651000000000000000000000"},
		{Kind: planner.KindToOwner, Token: "usdc.t", Amount: "249000000"},
	}) {
		t.Fatalf("resume steps %+v", last)
	}
	if pos := e.position(tok, pid); pos["state"] != "closed" {
		t.Fatal(pos)
	}
	e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, nil, 409)
	if h := e.do("GET", "/api/v1/me/holdings", tok, nil, 200); h["account_id"] != "alice.testnet.factory.t" {
		t.Fatal(h)
	}
}

// hold-leg tokens sit in the account ledger and in position.held: holdings and deploy see only the rest
func TestHoldLegsNotFree(t *testing.T) {
	e := setup(t)
	tok := e.onboard("alice.testnet")
	bid := e.do("POST", "/api/v1/me/baskets", tok, map[string]any{"name": "hold", "definition": map[string]any{"legs": []map[string]any{
		{"token": "usdc", "source": "hold", "weight_bps": 5000},
		{"token": "wnear", "source": "hold", "weight_bps": 5000, "pool_id": 1900},
	}}}, 201)["id"].(string)
	e.chain.mu.Lock()
	e.chain.bals = map[string]string{"usdc": "200000000", "wnear": "6"}
	e.chain.mu.Unlock()
	// 100 USDC: fee 0.4, 49.8 USDC stays as the usdc hold leg
	pid := e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "100000000"}, 202)["position_id"].(string)
	e.srv.Wait()
	if pos := e.position(tok, pid); pos["state"] != "open" || pos["held"].([]any)[0] != "49800000" {
		t.Fatal(pos)
	}
	h := e.do("GET", "/api/v1/me/holdings", tok, nil, 200)["tokens"].(map[string]any)
	if u, w := h["usdc"].(map[string]any)["balance"], h["wnear"].(map[string]any)["balance"]; u != "150200000" || w != "0" {
		t.Fatal(h)
	}
	if out := e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "150200001"}, 422); out["error"] != "insufficient_balance" {
		t.Fatal(out)
	}
	e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "150200000"}, 202)
	e.srv.Wait()
}

func jsonStr(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestWithdrawResumeLSTPool(t *testing.T) {
	e := setup(t)
	pool := uint64(7)
	e.srv.Plan.LST, e.srv.Plan.LSTPool = "lst.t", &pool
	tok := e.onboard("alice.testnet")
	bid := e.do("POST", "/api/v1/me/baskets", tok, basket, 201)["id"].(string)
	pid := e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "1000000000"}, 202)["position_id"].(string)
	e.srv.Wait()

	e.exec.failAt = 1 // the LiNEAR->wNEAR swap
	e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, nil, 202)
	e.srv.Wait()
	e.exec.failAt = -1
	e.chain.live = planner.Live{
		Lent: map[string]string{}, Staked: "49302000000000000000000000",
		Wallet: map[string]string{"usdc.t": "498000000", "wrap.t": "0"},
	}
	if out := e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, nil, 202); out["resumed"] != true {
		t.Fatal(out)
	}
	e.srv.Wait()
	// with a pool the swap lands wNEAR now (1% slippage floor), so it goes to the owner too
	if last := e.exec.calls[len(e.exec.calls)-1]; jsonStr(last) != jsonStr([]planner.Step{
		{Kind: planner.KindSwap, PoolID: &pool, TokenIn: "lst.t", TokenOut: "wrap.t", AmountIn: "49302000000000000000000000", MinOut: "48808980000000000000000000"},
		{Kind: planner.KindToOwner, Token: "usdc.t", Amount: "498000000"},
		{Kind: planner.KindToOwner, Token: "wrap.t", Amount: "48808980000000000000000000"},
	}) {
		t.Fatalf("resume steps %+v", last)
	}
	if pos := e.position(tok, pid); pos["state"] != "closed" {
		t.Fatal(pos)
	}
}

func TestSchedulesRevoke(t *testing.T) {
	e := setup(t)
	tok := e.onboard("alice.testnet")
	bid := e.do("POST", "/api/v1/me/baskets", tok, basket, 201)["id"].(string)
	in := map[string]any{"kind": "dca", "basket_id": bid, "params": map[string]string{"amount": "1000000"}, "every_seconds": 3600}
	sid := e.do("POST", "/api/v1/me/schedules", tok, in, 201)["id"].(string)
	e.do("POST", "/api/v1/me/schedules", tok, map[string]any{"kind": "dca", "basket_id": bid, "every_seconds": 10}, 400)
	if out := e.do("PATCH", "/api/v1/me/schedules/"+sid, tok, map[string]any{"every_seconds": 7200}, 200); out["every_seconds"] != 7200.0 {
		t.Fatal(out)
	}
	e.do("POST", "/api/v1/me/schedules", tok, in, 201)

	e.do("POST", "/api/v1/me/revoke", tok, nil, 409) // key still on chain
	e.chain.mu.Lock()
	e.chain.execOn["alice.testnet.factory.t"] = false
	e.chain.mu.Unlock()
	if out := e.do("POST", "/api/v1/me/revoke", tok, nil, 200); out["schedules_deactivated"] != 2.0 {
		t.Fatal(out)
	}
	if out := e.do("GET", "/api/v1/me/schedules/"+sid, tok, nil, 200); out["active"] != false {
		t.Fatal(out)
	}
	e.do("PATCH", "/api/v1/me/schedules/"+sid, tok, map[string]any{"active": true}, 409)
	e.do("POST", "/api/v1/me/schedules", tok, in, 409)
	e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "1000000"}, 409)
	e.do("DELETE", "/api/v1/me/schedules/"+sid, tok, nil, 200)
	e.do("DELETE", "/api/v1/me/schedules/"+sid, tok, nil, 404)
}

// Owner revoked on chain without calling /me/revoke: a run hitting AccessKeyNotFound stops schedules itself.
func TestKeyGoneMidRunRevokes(t *testing.T) {
	e := setup(t)
	tok := e.onboard("alice.testnet")
	bid := e.do("POST", "/api/v1/me/baskets", tok, basket, 201)["id"].(string)
	in := map[string]any{"kind": "dca", "basket_id": bid, "params": map[string]string{"amount": "1000000"}, "every_seconds": 60}
	sid := e.do("POST", "/api/v1/me/schedules", tok, in, 201)["id"].(string)
	e.chain.mu.Lock()
	e.chain.execOn["alice.testnet.factory.t"] = false
	e.chain.mu.Unlock()
	e.exec.failAt = 0
	e.exec.errMsg = `rpc: {"data":{"TxExecutionError":{"InvalidTxError":{"InvalidAccessKeyError":{"AccessKeyNotFound":{"account_id":"alice.testnet.factory.t"}}}}},"name":"HANDLER_ERROR"}`
	out := e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "1000000000"}, 202)
	e.srv.Wait()
	if pos := e.position(tok, out["position_id"].(string)); pos["state"] != "closed" {
		t.Fatal(pos)
	}
	x := e.do("GET", "/api/v1/me/executions/"+out["execution_id"].(string), tok, nil, 200)
	if x["status"] != "failed" || x["error"] != "executor_revoked" || x["steps"].([]any)[0].(map[string]any)["error"] != "executor_revoked" {
		t.Fatal(x)
	}
	if s := e.do("GET", "/api/v1/me/schedules/"+sid, tok, nil, 200); s["active"] != false {
		t.Fatal(s)
	}
	e.do("POST", "/api/v1/me/baskets/"+bid+"/deploy", tok, map[string]string{"amount": "1000000"}, 409)
}

// A deploy killed mid-run is swept to withdrawing; the owner's withdraw unwinds what is live.
func TestSweptDeployRecovers(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tok := e.onboard("alice.testnet")
	bid := e.do("POST", "/api/v1/me/baskets", tok, basket, 201)["id"].(string)
	b, err := e.srv.Store.Basket(ctx, bid, "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := e.srv.Plan.Deploy(ctx, b.Definition, "1000000000")
	if err != nil {
		t.Fatal(err)
	}
	pid, xid, err := e.srv.Store.OpenPosition(ctx, "alice.testnet", bid, ExecParams{Amount: "1000000000", Held: plan.Held}, plan.Steps)
	if err != nil {
		t.Fatal(err)
	}
	e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, nil, 409) // run still in flight
	if _, err := e.db.Exec(ctx, `update executions set updated_at=now() - interval '1 hour' where id=$1`, xid); err != nil {
		t.Fatal(err)
	}
	if n, err := sweep.Once(ctx, e.db); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err := e.srv.Store.Finish(ctx, xid, nil, 0, "succeeded", ""); !errors.Is(err, sweep.ErrSwept) {
		t.Fatal("late finish must not overwrite a swept run", err)
	}
	if pos := e.position(tok, pid); pos["state"] != "withdrawing" {
		t.Fatal(pos)
	}
	// died after lending: usdc lent, nothing staked
	e.chain.live = planner.Live{Lent: map[string]string{"usdc.t": "498000000"}, Staked: "0", Wallet: map[string]string{"usdc.t": "0", "wrap.t": "0"}}
	e.do("POST", "/api/v1/me/positions/"+pid+"/withdraw", tok, map[string]int{"bps": 100}, 202)
	e.srv.Wait()
	last := e.exec.calls[len(e.exec.calls)-1]
	if len(last) != 2 || last[0].Kind != planner.KindWithdrawLend || last[0].Amount != "498000000" || last[1].Kind != planner.KindToOwner {
		t.Fatalf("recovery steps %+v", last)
	}
	if pos := e.position(tok, pid); pos["state"] != "closed" {
		t.Fatal(pos)
	}
}
