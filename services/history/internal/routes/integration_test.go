//go:build integration

package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/pg/pgtest"
)

func TestSeriesAndBacktest(t *testing.T) {
	db := pgtest.DB(t)
	ctx := context.Background()
	for _, q := range []string{
		`insert into series_rate (source, token, day, apy)
			select 'lending', 'usdc', d, 0.1 from generate_series('2026-01-01'::date, '2026-03-01', '1 day') d`,
		`insert into series_rate (source, token, day, apy)
			select 'staking', 'wnear', d, 0.08 from generate_series('2026-01-01'::date, '2026-03-01', '1 day') d`,
		`insert into series_price (token, day, usd)
			select 'wnear', d, 3 + extract(day from d) / 100 from generate_series('2026-01-01'::date, '2026-03-01', '1 day') d`,
	} {
		if _, err := db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{DB: db, Tokens: map[string]string{"usdc": "usdc.testnet", "wnear": "wrap.testnet"},
		Now: func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }}
	mux := http.NewServeMux()
	s.Routes(mux)
	call := func(method, path, body string) (int, map[string]any) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	code, out := call("GET", "/api/v1/series/sources", "")
	if code != 200 || len(out["data"].([]any)) != 3 {
		t.Fatalf("sources %d %v", code, out)
	}
	code, out = call("GET", "/api/v1/series/rates?source=lending&token=usdc&from=2026-02-01&to=2026-02-10", "")
	if code != 200 || len(out["data"].([]any)) != 10 {
		t.Fatalf("rates %d %v", code, out)
	}
	code, out = call("GET", "/api/v1/series/prices?token=wnear&from=2026-02-01&to=2026-02-03", "")
	if code != 200 || len(out["data"].([]any)) != 3 {
		t.Fatalf("prices %d %v", code, out)
	}

	basket := `{"drift_bps":500,"legs":[{"token":"usdc","source":"lending","weight_bps":6000},{"token":"wnear","source":"staking","weight_bps":4000,"pool_id":1900}]}`
	code, out = call("POST", "/api/v1/backtest", `{"basket":`+basket+`,"amount":"1000","from":"2026-01-01","to":"2026-03-01"}`)
	if code != 200 || out["days"].(float64) != 59 || len(out["curve"].([]any)) != 60 || out["apy"] == nil || out["max_drawdown"] == nil {
		t.Fatalf("backtest %d %v", code, out)
	}
	code, out = call("POST", "/api/v1/backtest", `{"basket":{"legs":[{"token":"usdc","source":"staking","weight_bps":10000}]},"amount":"1"}`)
	if code != 400 || out["error"] != "invalid_basket" {
		t.Fatalf("bad basket %d %v", code, out)
	}
	code, out = call("POST", "/api/v1/backtest", `{"basket":{"legs":[{"token":"wnear","source":"lending","weight_bps":10000}]},"amount":"1"}`)
	if code != 422 || out["error"] != "no_data" {
		t.Fatalf("no data %d %v", code, out)
	}
}
