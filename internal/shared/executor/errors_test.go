package executor

import "testing"

func TestClean(t *testing.T) {
	for raw, want := range map[string]string{
		// live testnet failure after an on-chain revoke
		`rpc: {"cause":{"info":{},"name":"INVALID_TRANSACTION"},"code":-32000,"data":{"TxExecutionError":{"InvalidTxError":{"InvalidAccessKeyError":{"AccessKeyNotFound":{"account_id":"a.factory.t","public_key":"ed25519:F2pU"}}}}},"message":"Server error","name":"HANDLER_ERROR"}`: Revoked,
		`rpc: {"cause":{"info":{"public_key":"ed25519:x"},"name":"UNKNOWN_ACCESS_KEY"},"name":"HANDLER_ERROR"}`:                                        Revoked,
		`rpc: {"data":{"TxExecutionError":{"InvalidTxError":{"InvalidAccessKeyError":{"NotEnoughAllowance":{"allowance":"0"}}}}}}`:                     "executor key gas allowance used up",
		`rpc: {"data":{"TxExecutionError":{"InvalidTxError":{"InvalidAccessKeyError":{"MethodNameMismatch":{}}}}}}`:                                    "executor key rejected",
		`rpc: {"data":{"TxExecutionError":{"InvalidTxError":{"NotEnoughBalance":{"balance":"1"}}}}}`:                                                   "not enough NEAR for gas",
		`{"ActionError":{"index":0,"kind":{"LackBalanceForState":{"amount":"1"}}}}`:                                                                    "not enough NEAR for storage",
		`{"ActionError":{"index":0,"kind":{"FunctionCallError":{"ExecutionError":"Exceeded the prepaid gas."}}}}`:                                      "ran out of gas",
		`{"ActionError":{"index":0,"kind":{"FunctionCallError":{"ExecutionError":"Smart contract panicked: E22: not enough \"tokens\" in deposit"}}}}`: `contract: E22: not enough "tokens" in deposit`,
		`outcome unknown: rpc transport: timeout`:                "transaction outcome unknown, check the explorer",
		`rpc transport: connection refused`:                      "rpc_unavailable",
		`rpc: {"name":"HANDLER_ERROR","cause":{"name":"WEIRD"}}`: "transaction failed on chain",
		`venue failed, refunded`:                                 "venue failed, refunded",
	} {
		if got := Clean(raw); got != want {
			t.Errorf("Clean(%s) = %q, want %q", raw, got, want)
		}
	}
}

func TestTidy(t *testing.T) {
	rs := []Result{
		{Index: 0, Status: "ok"},
		{Index: 1, Status: "failed", Error: `rpc: {"InvalidAccessKeyError":{"AccessKeyNotFound":{}}}`},
		{Index: 2, Status: "failed", Error: "skipped"},
	}
	if !Tidy("x", rs) || rs[1].Error != Revoked || rs[2].Error != "skipped" {
		t.Fatal(rs)
	}
	if Tidy("x", []Result{{Status: "failed", Error: "venue failed, refunded"}}) {
		t.Fatal("not a revoke")
	}
}
