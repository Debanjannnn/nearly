package near

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fake answers by request_type (or method for tx) with a canned result/error body.
func fake(t *testing.T, replies map[string]string) *Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		key := req.Method
		if rt, ok := req.Params["request_type"].(string); ok {
			key = rt
		}
		w.Write([]byte(`{"jsonrpc":"2.0","id":"nearly",` + replies[key] + `}`))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func TestView(t *testing.T) {
	// "42" = [52,50]
	c := fake(t, map[string]string{"call_function": `"result":{"result":[52,50],"logs":[],"block_height":1}`})
	var n int
	if err := c.View(context.Background(), "x.testnet", "get", map[string]string{}, &n); err != nil || n != 42 {
		t.Fatal(n, err)
	}
}

func TestAccessKey(t *testing.T) {
	c := fake(t, map[string]string{"view_access_key": `"result":{"nonce":7,"permission":"FullAccess","block_height":1}`})
	k, err := c.ViewAccessKey(context.Background(), "a.testnet", "ed25519:abc")
	if err != nil || !k.FullAccess() || k.Nonce != 7 {
		t.Fatal(k, err)
	}
	c = fake(t, map[string]string{"view_access_key": `"result":{"nonce":1,"permission":{"FunctionCall":{"receiver_id":"x","method_names":[]}}}`})
	if k, _ := c.ViewAccessKey(context.Background(), "a.testnet", "ed25519:abc"); k.FullAccess() {
		t.Fatal("function call key is not full access")
	}
	c = fake(t, map[string]string{"view_access_key": `"error":{"code":-32000,"message":"Server error","cause":{"name":"UNKNOWN_ACCESS_KEY"}}`})
	var re *RPCError
	if _, err := c.ViewAccessKey(context.Background(), "a.testnet", "ed25519:abc"); !errors.As(err, &re) || re.Cause.Name != "UNKNOWN_ACCESS_KEY" {
		t.Fatal(err)
	}
	c = fake(t, map[string]string{"view_access_key": `"result":{"error":"access key ed25519:abc does not exist","block_height":1}`})
	if _, err := c.ViewAccessKey(context.Background(), "a.testnet", "ed25519:abc"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatal(err)
	}
}

func TestAccountAndTx(t *testing.T) {
	c := fake(t, map[string]string{
		"view_account": `"result":{"amount":"100","locked":"0","code_hash":"11111111111111111111111111111111","storage_usage":182}`,
		"tx":           `"result":{"status":{"Failure":{"ActionError":{}}},"receipts_outcome":[]}`,
	})
	a, err := c.ViewAccount(context.Background(), "a.testnet")
	if err != nil || a.Amount != "100" || a.StorageUsage != 182 {
		t.Fatal(a, err)
	}
	s, err := c.TxStatus(context.Background(), "hash", "a.testnet")
	if err != nil || !s.Failed() {
		t.Fatal(s, err)
	}
}
