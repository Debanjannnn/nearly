package intents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Fixtures: tokens, dry quotes and the 404 are recorded from 1Click mainnet (dry only).
// quote_live and status_success add the fields the OpenAPI spec lists for live swaps.
func serve(t *testing.T, check func(*http.Request)) *Client {
	t.Helper()
	files := map[string]string{
		"/v0/tokens": "tokens.json", "/v0/quote": "quote_deposit.json",
		"/v0/status": "status_success.json", "/v0/deposit/submit": "status_success.json",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		f := files[r.URL.Path]
		if r.URL.Query().Get("depositAddress") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			f = "status_404.json"
		}
		b, err := os.ReadFile("testdata/" + f)
		if err != nil {
			t.Error(err)
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "")
}

func TestTokens(t *testing.T) {
	toks, err := serve(t, nil).Tokens(context.Background())
	if err != nil || len(toks) != 3 || toks[0].AssetID != "nep141:wrap.near" || toks[0].Decimals != 24 {
		t.Fatal(toks, err)
	}
}

func TestQuote(t *testing.T) {
	var got map[string]any
	var auth string
	c := serve(t, func(r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&got)
	})
	c.JWT = "jwt"
	q, err := c.Quote(context.Background(), QuoteRequest{Dry: true, SwapType: "EXACT_INPUT", Amount: "5000000",
		Recipient: "alice.near", Deadline: time.Date(2026, 11, 11, 19, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer jwt" || got["dry"] != true || got["recipient"] != "alice.near" || got["deadline"] != "2026-11-11T19:00:00Z" {
		t.Fatal(auth, got)
	}
	if q.Quote.AmountOut != "4999414" || q.Quote.DepositAddress != "" || q.Signature == "" || len(q.Raw) == 0 {
		t.Fatalf("%+v", q)
	}
}

func TestStatus(t *testing.T) {
	var query string
	c := serve(t, func(r *http.Request) { query = r.URL.RawQuery })
	st, err := c.Status(context.Background(), "0xabc", "m1")
	if err != nil || st.Status != "SUCCESS" || !Terminal(st.Status) || len(st.SwapDetails) == 0 {
		t.Fatal(st, err)
	}
	if query != "depositAddress=0xabc&depositMemo=m1" {
		t.Fatal(query)
	}
	_, err = c.Status(context.Background(), "missing", "")
	var e *Error
	if !errors.As(err, &e) || e.Status != 404 || e.Message == "" {
		t.Fatal(err)
	}
}

func TestSubmit(t *testing.T) {
	var got SubmitRequest
	c := serve(t, func(r *http.Request) { json.NewDecoder(r.Body).Decode(&got) })
	st, err := c.Submit(context.Background(), SubmitRequest{TxHash: "tx", DepositAddress: "0xabc", NearSenderAccount: "alice.near"})
	if err != nil || st.Status != "SUCCESS" || got.TxHash != "tx" || got.NearSenderAccount != "alice.near" {
		t.Fatal(st, got, err)
	}
}

func TestTerminal(t *testing.T) {
	for s, want := range map[string]bool{"SUCCESS": true, "REFUNDED": true, "FAILED": true, "PROCESSING": false, "PENDING_DEPOSIT": false} {
		if Terminal(s) != want {
			t.Error(s)
		}
	}
}
