package reconcile

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

var tokens = map[string]string{"usdc": "usdc.t", "wnear": "wrap.t"}

func bi(s string) *big.Int { v, _ := new(big.Int).SetString(s, 10); return v }

var legs = []planner.Leg{
	{Token: "usdc", Source: "lending", WeightBps: 5000},
	{Token: "wnear", Source: "staking", WeightBps: 3000},
	{Token: "wnear", Source: "hold", WeightBps: 2000},
}

func TestMatchKeepsHeldAndReportsIdle(t *testing.T) {
	ps := []Position{{ID: "a", Legs: legs, Held: []*big.Int{bi("500"), bi("300"), bi("200")}}}
	ledger := map[string]*big.Int{"lending:usdc.t": bi("500"), "staking": bi("300"), "balance:wrap.t": bi("200"), "balance:usdc.t": bi("70")}
	held, idle := Apply(ledger, ps, tokens)
	if fmt.Sprint(held) != "[[500 300 200]]" {
		t.Fatalf("held %v", held)
	}
	if len(idle) != 1 || idle["balance:usdc.t"].String() != "70" {
		t.Fatalf("idle %v", idle)
	}
}

// A failed step refunded less than the db claims (e.g. stake never landed): shrink to chain.
func TestShortfallScalesDownProRata(t *testing.T) {
	ps := []Position{
		{ID: "a", Legs: legs, Held: []*big.Int{bi("500"), bi("300"), bi("200")}},
		{ID: "b", Legs: legs[:2], Held: []*big.Int{bi("100"), bi("100")}},
	}
	ledger := map[string]*big.Int{"lending:usdc.t": bi("600"), "staking": bi("201"), "balance:wrap.t": bi("200")}
	held, idle := Apply(ledger, ps, tokens)
	// staking claimed 400, chain 201: 300*201/400=150, last leg takes the rest = 51
	if fmt.Sprint(held) != "[[500 150 200] [100 51]]" {
		t.Fatalf("held %v", held)
	}
	if len(idle) != 0 {
		t.Fatalf("idle %v", idle)
	}
	if ps[0].Held[1].String() != "300" {
		t.Fatal("input must not be mutated")
	}
}

func TestEmptyLedgerZeroesPositions(t *testing.T) {
	ps := []Position{{ID: "a", Legs: legs[:1], Held: []*big.Int{bi("5")}}}
	held, _ := Apply(map[string]*big.Int{}, ps, tokens)
	if fmt.Sprint(held) != "[[0]]" {
		t.Fatalf("held %v", held)
	}
}
