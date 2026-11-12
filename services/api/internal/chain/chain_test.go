package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

// fakeRPC answers call_function by "<contract>.<method>" with a JSON value, and view_access_key by key.
func fakeRPC(t *testing.T, views map[string]string, keys map[string]string) *Chain {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params map[string]string `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		p := req.Params
		if p["request_type"] == "view_access_key" {
			if perm, ok := keys[p["account_id"]+"/"+p["public_key"]]; ok {
				fmt.Fprintf(w, `{"result":{"nonce":1,"permission":%s}}`, perm)
				return
			}
			w.Write([]byte(`{"error":{"code":-32000,"message":"Server error","cause":{"name":"UNKNOWN_ACCESS_KEY"}}}`))
			return
		}
		v, ok := views[p["account_id"]+"."+p["method_name"]]
		if !ok {
			t.Errorf("unexpected view %s.%s", p["account_id"], p["method_name"])
		}
		raw := []byte(v)
		ints := make([]int, len(raw))
		for i, b := range raw {
			ints[i] = int(b)
		}
		out, _ := json.Marshal(map[string]any{"result": map[string]any{"result": ints}})
		w.Write(out)
	}))
	t.Cleanup(srv.Close)
	dep := &config.Deployments{Factory: "f", Tokens: map[string]string{"usdc": "usdc.t", "wnear": "wrap.t"}}
	dep.Venues.Lending, dep.Venues.Dex, dep.Venues.Staking = "burrow", "ref", "linear"
	return &Chain{RPC: near.New(srv.URL), Dep: dep, ExecutorKey: "ed25519:exec"}
}

func TestKeys(t *testing.T) {
	c := fakeRPC(t, nil, map[string]string{
		"alice/ed25519:full": `"FullAccess"`,
		"alice/ed25519:fc":   `{"FunctionCall":{"receiver_id":"x","method_names":[]}}`,
		"sub/ed25519:exec":   `{"FunctionCall":{"receiver_id":"sub","method_names":["execute_step"]}}`,
	})
	ctx := context.Background()
	for key, want := range map[string]bool{"ed25519:full": true, "ed25519:fc": false, "ed25519:none": false} {
		if got, err := c.FullAccess(ctx, "alice", key); err != nil || got != want {
			t.Errorf("%s: %v %v", key, got, err)
		}
	}
	if on, err := c.ExecutorKeyActive(ctx, "sub"); err != nil || !on {
		t.Fatal(on, err)
	}
	if on, err := c.ExecutorKeyActive(ctx, "other"); err != nil || on {
		t.Fatal(on, err)
	}
}

func TestViews(t *testing.T) {
	c := fakeRPC(t, map[string]string{
		"ref.get_return":             `"990"`,
		"f.get_account":              `null`,
		"burrow.get_account":         `{"supplied":[{"token_id":"usdc.t","balance":"5000000000000000000","shares":"1"}]}`,
		"burrow.get_asset":           `{"config":{"extra_decimals":12}}`,
		"linear.get_account_details": `{"staked_balance":"77"}`,
		"usdc.t.ft_balance_of":       `"3"`,
	}, nil)
	ctx := context.Background()
	q, err := c.Quote(ctx, 1, "usdc.t", "wrap.t", big.NewInt(1000))
	if err != nil || q.String() != "990" {
		t.Fatal(q, err)
	}
	if id, err := c.FactoryAccount(ctx, "alice"); err != nil || id != "" {
		t.Fatal(id, err)
	}
	live, err := c.Live(ctx, "sub", []planner.Step{
		{Kind: planner.KindWithdrawLend, Token: "usdc.t", Amount: "1"},
		{Kind: planner.KindUnstake, Amount: "1"},
		{Kind: planner.KindToOwner, Token: "usdc.t", Amount: "1"},
	})
	if err != nil || live.Lent["usdc.t"] != "5000000" || live.Staked != "77" || live.Wallet["usdc.t"] != "3" {
		t.Fatalf("%+v %v", live, err)
	}
}
