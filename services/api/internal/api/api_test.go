package api

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

func TestHelpers(t *testing.T) {
	if !isUUID("86aa7e04-d327-4585-a1a3-cd4289c4d751") || isUUID("86aa7e04d3274585a1a3cd4289c4d751xxxx") || isUUID("x") {
		t.Fatal("isUUID")
	}
	for s, want := range map[string]bool{"1": true, "0": false, "-1": false, "1.5": false, "340282366920938463463374607431768211456": false} {
		if positiveU128(s) != want {
			t.Errorf("positiveU128(%q)", s)
		}
	}
}

func TestFeeRow(t *testing.T) {
	s := &Server{Dep: &config.Deployments{Tokens: map[string]string{"usdc": "usdc.t"}}}
	r := s.feeRow(planner.Plan{Fee: "5"}, "carol", "alice")
	if r == nil || r.Token != "usdc.t" || r.CreatorShare != "2" || *r.Creator != "carol" {
		t.Fatalf("%+v", r)
	}
	if r := s.feeRow(planner.Plan{Fee: "5"}, "alice", "alice"); r.Creator != nil || r.CreatorShare != "0" {
		t.Fatalf("own basket: %+v", r)
	}
	if s.feeRow(planner.Plan{Fee: "0"}, "c", "a") != nil {
		t.Fatal("zero fee booked")
	}
}

func TestIntentRequest(t *testing.T) {
	ok := func(in intentQuoteIn) bool { _, ok := in.request("a", "b", "r", "f"); return ok }
	if !ok(intentQuoteIn{Amount: "1"}) || ok(intentQuoteIn{Amount: "0"}) || ok(intentQuoteIn{Amount: "1", SwapType: "ANY_INPUT"}) ||
		ok(intentQuoteIn{Amount: "1", SlippageBps: 5000}) {
		t.Fatal("validation")
	}
	in := intentQuoteIn{Amount: "1"}
	q, ok2 := in.request("a", "b", "r", "")
	if ok2 || q.SwapType != "EXACT_INPUT" || q.SlippageTolerance != 100 || q.Deadline.IsZero() {
		t.Fatalf("empty refund accepted or bad defaults: %+v", q)
	}
}

func TestAbandonedIntent(t *testing.T) {
	old, tx := time.Now().Add(-3*quoteTTL), "h"
	if !abandoned(&Intent{Status: "PENDING_DEPOSIT", CreatedAt: old}) ||
		abandoned(&Intent{Status: "PENDING_DEPOSIT", CreatedAt: time.Now()}) ||
		abandoned(&Intent{Status: "PENDING_DEPOSIT", CreatedAt: old, DepositTx: &tx}) ||
		abandoned(&Intent{Status: "PROCESSING", CreatedAt: old}) {
		t.Fatal("abandoned")
	}
}

func TestFailStatus(t *testing.T) {
	for err, want := range map[error]int{
		ErrNotFound: 404, ErrRevoked: 409, ErrRPC: 502,
		&planner.Error{Code: "invalid_basket"}:      400,
		&planner.Error{Code: "quote_unavailable"}:   502,
		&planner.Error{Code: "nothing_to_withdraw"}: 422,
		errors.New("boom"):                          500,
	} {
		w := httptest.NewRecorder()
		fail(w, "t", err)
		if w.Code != want {
			t.Errorf("%v: %d want %d", err, w.Code, want)
		}
	}
}
