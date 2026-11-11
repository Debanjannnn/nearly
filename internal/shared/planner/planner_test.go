package planner

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
)

func sum(xs ...string) *big.Int {
	t := new(big.Int)
	for _, x := range xs {
		v, _ := amt(x)
		t.Add(t, v)
	}
	return t
}

func TestStepJSON(t *testing.T) {
	var pool uint64
	cases := map[string]Step{
		`{"kind":"supply","token":"t","amount":"1"}`:                                               tokenStep(KindSupply, "t", big.NewInt(1)),
		`{"kind":"stake","amount":"5"}`:                                                            tokenStep(KindStake, "t", big.NewInt(5)),
		`{"kind":"swap","pool_id":0,"token_in":"a","token_out":"b","amount_in":"2","min_out":"1"}`: swap(pool, "a", "b", big.NewInt(2), big.NewInt(1)),
	}
	for want, s := range cases {
		if b, _ := json.Marshal(s); string(b) != want {
			t.Errorf("got %s want %s", b, want)
		}
	}
}

func TestValidate(t *testing.T) {
	p := newP()
	bad := []Basket{
		{},
		{Legs: []Leg{{Token: "usdc", Source: SourceLending, WeightBps: 9000}}},
		{Legs: []Leg{{Token: "usdc", Source: SourceStaking, WeightBps: 10000}}},
		{Legs: []Leg{{Token: "eth", Source: SourceHold, WeightBps: 10000}}},
		{Legs: []Leg{{Token: "usdc", Source: SourceHold, WeightBps: 5000}, {Token: "usdc", Source: SourceHold, WeightBps: 5000}}},
	}
	for i, b := range bad {
		var e *Error
		if err := p.Validate(b); !errors.As(err, &e) || e.Code != "invalid_basket" {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if err := p.Validate(basket); err != nil {
		t.Fatal(err)
	}
}

func TestDeploy(t *testing.T) {
	pl, err := newP().Deploy(context.Background(), basket, "1000000000")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Steps[0].Kind != KindFee || pl.Steps[0].Amount != "4000000" || pl.Fee != "4000000" {
		t.Fatalf("fee first: %+v", pl.Steps[0])
	}
	// one swap for all three wnear legs
	swaps := 0
	for _, s := range pl.Steps {
		if s.Kind == KindSwap {
			swaps++
			if s.AmountIn != "597600000" {
				t.Fatalf("swap in %s", s.AmountIn)
			}
		}
	}
	if swaps != 1 {
		t.Fatalf("%d swaps", swaps)
	}
	if pl.Held[0] != "398400000" || sum(pl.Held[1:]...).String() != pl.Steps[1].MinOut {
		t.Fatalf("held %v", pl.Held)
	}
	if _, err := newP().Deploy(context.Background(), basket, "-1"); err == nil {
		t.Fatal("negative amount")
	}
}

func TestWithdrawPartial(t *testing.T) {
	held := []string{"1000", "3000", "2001", "500"}
	pl, err := newP().Withdraw(basket, held, 5000)
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		tokenStep(KindWithdrawLend, usdc, big.NewInt(500)),
		tokenStep(KindWithdrawLend, wnear, big.NewInt(1500)),
		tokenStep(KindUnstake, "", big.NewInt(1000)),
		tokenStep(KindToOwner, usdc, big.NewInt(500)),
		tokenStep(KindToOwner, wnear, big.NewInt(1750)), // unstaked NEAR is not back yet
	}
	if b, w := mustJSON(pl.Steps), mustJSON(want); b != w {
		t.Fatalf("got %s\nwant %s", b, w)
	}
	if strings.Join(pl.Held, ",") != "500,1500,1001,250" {
		t.Fatal(pl.Held)
	}
	if _, err := newP().Withdraw(basket, held, 0); err == nil {
		t.Fatal("bps 0")
	}
	full, _ := newP().Withdraw(basket, held, 10_000)
	if strings.Join(full.Held, ",") != "0,0,0,0" {
		t.Fatal(full.Held)
	}
}

func TestResumeWithdraw(t *testing.T) {
	plan, _ := newP().Withdraw(basket, []string{"1000", "3000", "2000", "500"}, 10_000)
	// step 0 done; step 1 landed on chain but was not recorded; unstake not run.
	live := Live{
		Lent:   map[string]string{usdc: "0", wnear: "0"},
		Staked: "2000",
		Wallet: map[string]string{usdc: "1000", wnear: "3500"},
	}
	got, err := newP().ResumeWithdraw(plan.Steps, 1, live)
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		tokenStep(KindUnstake, "", big.NewInt(2000)),
		tokenStep(KindToOwner, usdc, big.NewInt(1000)),
		tokenStep(KindToOwner, wnear, big.NewInt(3500)),
	}
	if b, w := mustJSON(got), mustJSON(want); b != w {
		t.Fatalf("got %s\nwant %s", b, w)
	}
	// everything already finished: nothing to redo
	got, _ = newP().ResumeWithdraw(plan.Steps, 0, Live{})
	if len(got) != 0 {
		t.Fatalf("empty chain must yield no steps: %+v", got)
	}
	if _, err := newP().ResumeWithdraw(plan.Steps, 9, live); err == nil {
		t.Fatal("done out of range")
	}
}

func lstP() *Planner {
	p, pool := newP(), uint64(3515)
	p.LSTPool = &pool
	return p
}

func TestWithdrawLSTExit(t *testing.T) {
	pl, err := lstP().Withdraw(basket, []string{"1000", "3000", "2000", "500"}, 5000)
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		tokenStep(KindWithdrawLend, usdc, big.NewInt(500)),
		tokenStep(KindWithdrawLend, wnear, big.NewInt(1500)),
		swap(3515, lst, wnear, big.NewInt(1000), big.NewInt(995)),
		tokenStep(KindToOwner, usdc, big.NewInt(500)),
		tokenStep(KindToOwner, wnear, big.NewInt(1500+995+250)),
	}
	if b, w := mustJSON(pl.Steps), mustJSON(want); b != w {
		t.Fatalf("got %s\nwant %s", b, w)
	}
	// resume: exit capped by what is still staked, min_out rescaled
	live := Live{Staked: "400", Wallet: map[string]string{usdc: "500", wnear: "1750"}}
	got, err := lstP().ResumeWithdraw(pl.Steps, 2, live)
	if err != nil {
		t.Fatal(err)
	}
	want = []Step{
		swap(3515, lst, wnear, big.NewInt(400), big.NewInt(398)),
		tokenStep(KindToOwner, usdc, big.NewInt(500)),
		tokenStep(KindToOwner, wnear, big.NewInt(1750+398)),
	}
	if b, w := mustJSON(got), mustJSON(want); b != w {
		t.Fatalf("got %s\nwant %s", b, w)
	}
}

func TestRebalance(t *testing.T) {
	e18 := "000000000000000000"
	held := []string{"300000000", "100000000" + e18, "60000000" + e18, "30000000" + e18}
	// usdc leg 300 of 870 = 34% vs 40% target -> out of 5% band
	values := []string{"300000000", "300000000", "180000000", "90000000"}
	pl, err := newP().Rebalance(context.Background(), basket, held, values)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{}
	for _, s := range pl.Steps {
		kinds = append(kinds, s.Kind)
	}
	if strings.Join(kinds, ",") != "withdraw_lend,unstake,swap,supply" {
		t.Fatal(kinds)
	}
	// in band: no steps
	pl, _ = newP().Rebalance(context.Background(), basket, held, []string{"400", "300", "200", "100"})
	if len(pl.Steps) != 0 {
		t.Fatal(pl.Steps)
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
