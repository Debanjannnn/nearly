package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/services/market-data/internal/hub"
	"github.com/Debanjannnn/nearly/services/market-data/internal/quote"
	"github.com/Debanjannnn/nearly/services/market-data/internal/store"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

const usdc, wnear = "usdcc.ft.ref-labs.testnet", "wrap.testnet"

func server(t *testing.T) (*Server, http.Handler) {
	st := store.Store{RDB: redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()}), Net: "testnet"}
	now := time.UnixMilli(1_000_000)
	s := &Server{Store: st, Hub: hub.New(), PoolIDs: []uint64{1900, 7}, MaxAge: time.Minute, Now: func() time.Time { return now }}
	ctx := t.Context()
	// testnet pool 1900 as recorded
	st.Put(ctx, store.KeyPool(1900), quote.Pool{ID: 1900, Kind: "SIMPLE_POOL", Tokens: []string{wnear, usdc},
		Amounts: []string{"75199883753165716516172173", "1129094461"}, TotalFee: 30, Ts: now.UnixMilli()})
	st.Put(ctx, store.KeyPool(7), quote.Pool{ID: 7, Kind: "SIMPLE_POOL", Tokens: []string{wnear, "old.testnet"},
		Amounts: []string{"1", "1"}, TotalFee: 30, Ts: now.Add(-time.Hour).UnixMilli()})
	st.Put(ctx, store.KeyRates, []store.Rate{{Venue: "lending", Token: usdc, APY: 0.0013, Ts: now.UnixMilli()},
		{Venue: "staking", Token: "linear-protocol.testnet", APY: 0.04, Ts: now.Add(-2 * time.Minute).UnixMilli()}})
	return s, s.Handler([]string{"*"}, nil)
}

func get(t *testing.T, h http.Handler, url string) (int, map[string]any) {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func TestQuote(t *testing.T) {
	_, h := server(t)
	// recorded get_return for 1 wNEAR on pool 1900: 14773664
	code, b := get(t, h, "/api/v1/quote?token_in="+wnear+"&token_out="+usdc+"&amount_in=1000000000000000000000000&slippage_bps=100")
	if code != 200 || b["amount_out"] != "14773664" || b["min_out"] != "14625927" || b["pool_id"] != 1900.0 || b["max_safe_in"] != "759594785385512288042143" {
		t.Fatal(code, b)
	}
	for url, want := range map[string]struct {
		code int
		err  string
	}{
		"/api/v1/quote?token_in=" + wnear + "&token_out=" + usdc + "&amount_in=abc":                  {400, "bad_request"},
		"/api/v1/quote?token_in=" + wnear + "&token_out=" + usdc + "&amount_in=1&slippage_bps=10001": {400, "bad_request"},
		"/api/v1/quote?token_in=" + wnear + "&token_out=" + usdc + "&amount_in=1&pool_id=5":          {400, "unknown_pool"},
		"/api/v1/quote?token_in=" + wnear + "&token_out=" + usdc + "&amount_in=1&max_impact_bps=0":   {400, "bad_request"},
		"/api/v1/quote?token_in=" + wnear + "&token_out=x.testnet&amount_in=1":                       {404, "no_pool"},
		"/api/v1/quote?token_in=" + wnear + "&token_out=old.testnet&amount_in=1":                     {503, "stale"},
		"/api/v1/quote?token_in=" + wnear + "&token_out=" + usdc + "&amount_in=1&pool_id=7":          {404, "no_pool"},
	} {
		if code, b := get(t, h, url); code != want.code || b["error"] != want.err {
			t.Error(url, code, b)
		}
	}
}

func TestRatesAndPrices(t *testing.T) {
	_, h := server(t)
	code, b := get(t, h, "/api/v1/rates")
	if code != 200 || len(b["rates"].([]any)) != 2 || b["stale"] != true {
		t.Fatal(code, b)
	}
	if code, b := get(t, h, "/api/v1/prices"); code != 503 || b["error"] != "unavailable" {
		t.Fatal(code, b)
	}
}
