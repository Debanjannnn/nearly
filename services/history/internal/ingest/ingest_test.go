package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/near"
)

func TestParseLlama(t *testing.T) {
	b, err := os.ReadFile("testdata/llama_near.json")
	if err != nil {
		t.Fatal(err)
	}
	var c llamaChart
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	got := parseLlama(c, Coins["wnear"])
	if len(got) != 4 || math.Abs(got["2026-11-07"]-3.7103689940305995) > 1e-12 {
		t.Fatalf("got %v", got)
	}
}

func TestStakeAPY(t *testing.T) {
	r := map[string]float64{"2026-01-01": 1.0, "2026-01-08": 1.001, "2026-01-09": 1.002}
	out := stakeAPY(r)
	if len(out) != 1 || out[0].day != "2026-01-08" || out[0].source != "staking" {
		t.Fatalf("got %+v", out)
	}
	if want := math.Pow(1.001, 365.0/7) - 1; math.Abs(out[0].apy-want) > 1e-12 {
		t.Fatalf("apy %v want %v", out[0].apy, want)
	}
}

// fakeRPC is an archival node whose block h is at t0 + h*0.6s.
func fakeRPC(t *testing.T, t0 time.Time) *near.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Params struct {
				BlockID uint64 `json:"block_id"`
				Account string `json:"account_id"`
				Method  string `json:"method_name"`
				Args    string `json:"args_base64"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		reply := func(v any) { json.NewEncoder(w).Encode(map[string]any{"result": v}) }
		p := req.Params
		switch {
		case req.Method == "block":
			h := p.BlockID
			if h == 0 {
				h = 1_000_000
			}
			reply(map[string]any{"header": map[string]any{"height": h, "timestamp": t0.UnixNano() + int64(h)*600_000_000}})
		case p.Method == "get_asset":
			args, _ := base64.StdEncoding.DecodeString(p.Args)
			if string(args) == `{"token_id":"wrap.testnet"}` {
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": -32000, "message": "Server error",
					"cause": map[string]string{"name": "CONTRACT_EXECUTION_ERROR"}}})
				return
			}
			reply(map[string]any{"result": bytesJSON(`{"supply_apr":"0.1"}`)})
		case p.Method == "ft_price":
			reply(map[string]any{"result": bytesJSON(`"1400000000000000000000000"`)})
		}
	}))
	t.Cleanup(srv.Close)
	return near.New(srv.URL)
}

func bytesJSON(s string) []int {
	out := make([]int, len(s))
	for i := range s {
		out[i] = int(s[i])
	}
	return out
}

func TestSampleFindsMidnightBlock(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rpc := fakeRPC(t, t0)
	dep := &config.Deployments{Tokens: map[string]string{"usdc": "usdc.testnet", "wnear": "wrap.testnet"}}
	dep.Venues.Lending, dep.Venues.Staking = "burrow.testnet", "linear.testnet"
	g := &Ingester{Archival: rpc, Dep: dep}
	ctx := context.Background()
	head, err := rpc.Block(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	s, err := g.sample(ctx, head, day)
	if err != nil {
		t.Fatal(err)
	}
	at := t0.Add(time.Duration(s.Height) * 600 * time.Millisecond)
	if d := at.Sub(day); d < -10*time.Minute || d > 10*time.Minute {
		t.Fatalf("block %d at %s, want near %s", s.Height, at, day)
	}
	if s.Day != "2026-01-05" || math.Abs(s.Lending["usdc"]-math.Expm1(0.1)) > 1e-12 || math.Abs(s.Ratio-1.4) > 1e-12 {
		t.Fatalf("sample %+v", s)
	}
	if _, ok := s.Lending["wnear"]; ok {
		t.Fatal("a view that cannot answer must be skipped, not stored")
	}
}
