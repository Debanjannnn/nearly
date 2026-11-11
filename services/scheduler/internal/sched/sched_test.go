package sched

import (
	"testing"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

func TestNextRun(t *testing.T) {
	t0 := time.Unix(1000, 0)
	for _, c := range []struct {
		now  int64
		want int64
	}{
		{999, 1060},  // early: one interval on
		{1000, 1060}, // on the slot
		{1059, 1060},
		{1060, 1120}, // missed slots are skipped
		{1290, 1300},
	} {
		if got := NextRun(t0, 60, time.Unix(c.now, 0)).Unix(); got != c.want {
			t.Errorf("now %d: got %d want %d", c.now, got, c.want)
		}
	}
}

func TestHoldLegs(t *testing.T) {
	got := holdLegs([]planner.Leg{
		{Token: "usdc", Source: planner.SourceLending, WeightBps: 3000},
		{Token: "wnear", Source: planner.SourceStaking, WeightBps: 4000, PoolID: 7},
		{Token: "usdc", Source: planner.SourceHold, WeightBps: 3000},
	})
	want := []planner.Leg{
		{Token: "usdc", Source: planner.SourceHold, WeightBps: 6000},
		{Token: "wnear", Source: planner.SourceHold, WeightBps: 4000, PoolID: 7},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}
	p := planner.Planner{Tokens: map[string]string{"usdc": "u", "wnear": "w"}}
	if err := p.Validate(planner.Basket{Legs: got}); err != nil {
		t.Fatal(err)
	}
}
