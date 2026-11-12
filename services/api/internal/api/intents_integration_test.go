//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/intents"
)

// fake1Click replays the recorded 1Click fixtures; live quotes get a fresh deposit address each.
type fake1Click struct {
	mu       sync.Mutex
	quotes   []intents.QuoteRequest
	submits  []intents.SubmitRequest
	statuses int
}

func fixture(t *testing.T, name string) map[string]any {
	b, err := os.ReadFile("../../../../internal/shared/intents/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	json.Unmarshal(b, &v)
	return v
}

func (f *fake1Click) serve(t *testing.T) *intents.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/v0/tokens":
			b, _ := os.ReadFile("../../../../internal/shared/intents/testdata/tokens.json")
			w.Write(b)
		case "/v0/quote":
			var q intents.QuoteRequest
			json.NewDecoder(r.Body).Decode(&q)
			f.quotes = append(f.quotes, q)
			if q.RefundTo == "bad" {
				httpx.JSON(w, 400, map[string]string{"message": "refundTo is not valid"})
				return
			}
			if q.Dry {
				httpx.JSON(w, 200, fixture(t, "quote_deposit.json"))
				return
			}
			v := fixture(t, "quote_live.json")
			v["quote"].(map[string]any)["depositAddress"] = fmt.Sprintf("0xdep%d", len(f.quotes))
			httpx.JSON(w, 200, v)
		case "/v0/status":
			f.statuses++
			httpx.JSON(w, 200, fixture(t, "status_success.json"))
		case "/v0/deposit/submit":
			var s intents.SubmitRequest
			json.NewDecoder(r.Body).Decode(&s)
			f.submits = append(f.submits, s)
			v := fixture(t, "status_success.json")
			v["status"] = "PROCESSING"
			httpx.JSON(w, 200, v)
		}
	}))
	t.Cleanup(srv.Close)
	return intents.New(srv.URL, "")
}

func TestIntents(t *testing.T) {
	e := setup(t)
	f := &fake1Click{}
	e.srv.Intents = f.serve(t)

	if list := e.do("GET", "/api/v1/intents/tokens", "", nil, 200)["list"]; !strings.Contains(string(list.(json.RawMessage)), "nep141:wrap.near") {
		t.Fatal(list)
	}
	dep := map[string]any{"origin_asset": "nep141:base-usdc", "amount": "5000000", "refund_to": "0xme", "dry": true}
	e.do("POST", "/api/v1/me/intents/deposit-quote", e.login("bob.t"), dep, 409) // no sub-account yet

	tok := e.onboard("alice.t")
	out := e.do("POST", "/api/v1/me/intents/deposit-quote", tok, dep, 200)
	q := f.quotes[len(f.quotes)-1]
	if q.Recipient != "alice.t.factory.t" || q.DestinationAsset != "nep141:usdc.t" || q.RefundTo != "0xme" ||
		q.RefundType != "ORIGIN_CHAIN" || q.RecipientType != "DESTINATION_CHAIN" || !q.Dry || q.SlippageTolerance != 100 {
		t.Fatalf("%+v", q)
	}
	if out["quote"].(map[string]any)["amountOut"] != "4999414" {
		t.Fatal(out)
	}
	if l := e.do("GET", "/api/v1/me/intents/status", tok, nil, 200)["list"]; string(l.(json.RawMessage)) != "[]\n" {
		t.Fatal("dry quote persisted", l)
	}

	dep["dry"] = false
	addr := e.do("POST", "/api/v1/me/intents/deposit-quote", tok, dep, 200)["quote"].(map[string]any)["depositAddress"].(string)
	var list []Intent
	json.Unmarshal(e.do("GET", "/api/v1/me/intents/status", tok, nil, 200)["list"].(json.RawMessage), &list)
	if len(list) != 1 || list[0].DepositAddress != addr || list[0].Kind != "deposit" || list[0].Status != "SUCCESS" || len(list[0].SwapDetails) == 0 {
		t.Fatalf("%+v", list)
	}
	e.do("GET", "/api/v1/me/intents/status", tok, nil, 200)
	if f.statuses != 1 {
		t.Fatal("terminal intent re-polled", f.statuses)
	}

	wd := map[string]any{"origin_asset": "base-usdc", "destination_asset": "nep141:base-usdc", "amount": "5000000", "recipient": "0xdest"}
	e.do("POST", "/api/v1/me/intents/withdraw-quote", tok, wd, 400) // origin must be a NEAR asset
	wd["origin_asset"] = "nep141:usdc.t"
	waddr := e.do("POST", "/api/v1/me/intents/withdraw-quote", tok, wd, 200)["quote"].(map[string]any)["depositAddress"].(string)
	if q := f.quotes[len(f.quotes)-1]; q.RefundTo != "alice.t" || q.Recipient != "0xdest" || q.OriginAsset != "nep141:usdc.t" {
		t.Fatalf("%+v", q)
	}
	sub := e.do("POST", "/api/v1/me/intents/submit", tok, map[string]string{"deposit_address": waddr, "tx_hash": "txw"}, 200)
	if sub["status"] != "PROCESSING" || sub["deposit_tx"] != "txw" || f.submits[0].NearSenderAccount != "alice.t" {
		t.Fatal(sub, f.submits)
	}
	e.do("POST", "/api/v1/me/intents/submit", e.login("eve.t"), map[string]string{"deposit_address": waddr, "tx_hash": "x"}, 404)

	dep["refund_to"] = "bad"
	if out := e.do("POST", "/api/v1/me/intents/deposit-quote", tok, dep, 400); out["error"] != "intents_rejected" || out["detail"] != "refundTo is not valid" {
		t.Fatal(out)
	}
}
