package planner

import (
	"context"
	"encoding/json"
	"flag"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/planner golden files")

const (
	usdc  = "usdc.fakes.testnet"
	wnear = "wrap.testnet"
)

// fakeQ prices 1 wnear (24 dec) at 3 usdc (6 dec).
type fakeQ struct{}

func (fakeQ) Quote(_ context.Context, _ uint64, in, _ string, a *big.Int) (*big.Int, error) {
	e18 := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	if in == usdc {
		return mulDivBig(a, e18, big.NewInt(3)), nil
	}
	return mulDivBig(a, big.NewInt(3), e18), nil
}

func newP() *Planner {
	return &Planner{Tokens: map[string]string{"usdc": usdc, "wnear": wnear}, Q: fakeQ{}, SlippageBps: 50}
}

var basket = Basket{DriftBps: 500, Legs: []Leg{
	{Token: "usdc", Source: SourceLending, WeightBps: 4000},
	{Token: "wnear", Source: SourceLending, WeightBps: 3000, PoolID: 0},
	{Token: "wnear", Source: SourceStaking, WeightBps: 2000, PoolID: 0},
	{Token: "wnear", Source: SourceHold, WeightBps: 1000, PoolID: 0},
}}

// golden is one shared planner<->executor fixture: input + the exact steps the executor will run.
type golden struct {
	Op     string   `json:"op"`
	Basket *Basket  `json:"basket,omitempty"`
	Amount string   `json:"amount,omitempty"`
	Held   []string `json:"held,omitempty"`
	Values []string `json:"values,omitempty"`
	Bps    int      `json:"bps,omitempty"`
	Plan   []Step   `json:"plan,omitempty"`
	Done   int      `json:"done,omitempty"`
	Live   *Live    `json:"live,omitempty"`
	Steps  []Step   `json:"steps"`
}

func (g golden) run(p *Planner) ([]Step, error) {
	switch g.Op {
	case "deploy":
		pl, err := p.Deploy(context.Background(), *g.Basket, g.Amount)
		return pl.Steps, err
	case "withdraw":
		pl, err := p.Withdraw(*g.Basket, g.Held, g.Bps)
		return pl.Steps, err
	case "resume":
		return p.ResumeWithdraw(g.Plan, g.Done, *g.Live)
	case "rebalance":
		pl, err := p.Rebalance(context.Background(), *g.Basket, g.Held, g.Values)
		return pl.Steps, err
	}
	panic(g.Op)
}

func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("../../../testdata/planner/*.json")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var g golden
			if err := json.Unmarshal(b, &g); err != nil {
				t.Fatal(err)
			}
			got, err := g.run(newP())
			if err != nil {
				t.Fatal(err)
			}
			if *update {
				g.Steps = got
				out, _ := json.MarshalIndent(g, "", "  ")
				os.WriteFile(f, append(out, '\n'), 0o644)
				return
			}
			if !reflect.DeepEqual(got, g.Steps) {
				gj, _ := json.MarshalIndent(got, "", "  ")
				t.Fatalf("steps differ, got:\n%s", gj)
			}
		})
	}
}
