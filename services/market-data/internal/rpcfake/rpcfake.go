// Package rpcfake serves recorded NEAR RPC replies (testdata/<net>.json) for tests.
package rpcfake

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Debanjannnn/nearly/internal/shared/near"
)

// New returns a client for the fixture; keys are "<acct> <method> <args>@<height>" and "block@<height>".
// Missing keys answer UNKNOWN_BLOCK so callers' error paths run.
func New(t testing.TB, network string) *near.Client {
	_, file, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "testdata", network+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx map[string]json.RawMessage
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Params struct {
				Account string `json:"account_id"`
				Method  string `json:"method_name"`
				Args    string `json:"args_base64"`
				BlockID uint64 `json:"block_id"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		p := req.Params
		key := fmt.Sprintf("block@%d", p.BlockID)
		if req.Method == "query" {
			args, _ := base64.StdEncoding.DecodeString(p.Args)
			key = fmt.Sprintf("%s %s %s@%d", p.Account, p.Method, args, p.BlockID)
		}
		v, ok := fx[key]
		if !ok || string(v) == "null" {
			w.Write([]byte(`{"error":{"code":-32000,"message":"Server error","cause":{"name":"UNKNOWN_BLOCK"},"data":"` + key + `"}}`))
			return
		}
		if req.Method == "block" {
			fmt.Fprintf(w, `{"result":{"header":%s}}`, v)
			return
		}
		raw := make([]int, len(v))
		for i, c := range []byte(v) {
			raw[i] = int(c)
		}
		out, _ := json.Marshal(map[string]any{"result": map[string]any{"result": raw}})
		w.Write(out)
	}))
	t.Cleanup(srv.Close)
	return near.New(srv.URL)
}
