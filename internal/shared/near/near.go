// Package near is a minimal NEAR JSON-RPC client (read-only: views and tx status).
package near

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	URL  string
	HTTP *http.Client
}

func New(url string) *Client { return &Client{URL: url, HTTP: &http.Client{Timeout: 10 * time.Second}} }

// RPCError is a JSON-RPC error; Cause.Name is e.g. UNKNOWN_ACCESS_KEY, UNKNOWN_ACCOUNT.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Cause   struct {
		Name string `json:"name"`
	} `json:"cause"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("near rpc %d %s: %s %s", e.Code, e.Message, e.Cause.Name, e.Data)
}

// Call posts one JSON-RPC request and decodes result into out.
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "nearly", "method": method, "params": params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return fmt.Errorf("near rpc %s: http %d: %w", method, res.StatusCode, err)
	}
	if r.Error != nil {
		return r.Error
	}
	// query errors (unknown key, contract panic) come back inside result
	var qe struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(r.Result, &qe) == nil && qe.Error != "" {
		return fmt.Errorf("near rpc %s: %s", method, qe.Error)
	}
	return json.Unmarshal(r.Result, out)
}

func query(reqType, account string, extra map[string]any) map[string]any {
	p := map[string]any{"request_type": reqType, "finality": "final", "account_id": account}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

// View calls a view method with JSON args and decodes its JSON return into out.
func (c *Client) View(ctx context.Context, contract, method string, args, out any) error {
	return c.ViewAt(ctx, 0, contract, method, args, out)
}

// ViewAt is View at a block height (0 = final). Plain RPC nodes keep only ~1.5 days of state.
func (c *Client) ViewAt(ctx context.Context, height uint64, contract, method string, args, out any) error {
	a, err := json.Marshal(args)
	if err != nil {
		return err
	}
	p := query("call_function", contract, map[string]any{
		"method_name": method, "args_base64": base64.StdEncoding.EncodeToString(a),
	})
	if height > 0 {
		delete(p, "finality")
		p["block_id"] = height
	}
	// result is a JSON array of u8, which []byte would expect as base64
	var r struct {
		Raw []int `json:"result"`
	}
	if err := c.Call(ctx, "query", p, &r); err != nil {
		return err
	}
	b := make([]byte, len(r.Raw))
	for i, v := range r.Raw {
		b[i] = byte(v)
	}
	return json.Unmarshal(b, out)
}

type Account struct {
	Amount       string `json:"amount"`
	Locked       string `json:"locked"`
	CodeHash     string `json:"code_hash"`
	StorageUsage uint64 `json:"storage_usage"`
}

func (c *Client) ViewAccount(ctx context.Context, account string) (Account, error) {
	var a Account
	return a, c.Call(ctx, "query", query("view_account", account, nil), &a)
}

// AccessKey.Permission is the string "FullAccess" or {"FunctionCall": {...}}.
type AccessKey struct {
	Nonce      uint64          `json:"nonce"`
	Permission json.RawMessage `json:"permission"`
}

func (k AccessKey) FullAccess() bool { return string(k.Permission) == `"FullAccess"` }

// ViewAccessKey looks up publicKey ("ed25519:...") on account.
func (c *Client) ViewAccessKey(ctx context.Context, account, publicKey string) (AccessKey, error) {
	var k AccessKey
	return k, c.Call(ctx, "query", query("view_access_key", account, map[string]any{"public_key": publicKey}), &k)
}

// TxStatus is the final outcome of a tx; Status holds SuccessValue / SuccessReceiptId / Failure.
type TxStatus struct {
	Status map[string]json.RawMessage `json:"status"`
	// receipts_outcome kept raw: callers that need receipt detail decode it themselves
	ReceiptsOutcome json.RawMessage `json:"receipts_outcome"`
}

// BlockHeader.Timestamp is unix nanoseconds.
type BlockHeader struct {
	Height    uint64 `json:"height"`
	Timestamp uint64 `json:"timestamp"`
}

// Block returns the header at height (0 = final).
func (c *Client) Block(ctx context.Context, height uint64) (BlockHeader, error) {
	p := map[string]any{"finality": "final"}
	if height > 0 {
		p = map[string]any{"block_id": height}
	}
	var b struct {
		Header BlockHeader `json:"header"`
	}
	return b.Header, c.Call(ctx, "block", p, &b)
}

func (s TxStatus) Failed() bool { _, ok := s.Status["Failure"]; return ok }

// TxStatus waits until the tx is final (RPC side) and returns its outcome.
func (c *Client) TxStatus(ctx context.Context, hash, sender string) (TxStatus, error) {
	var s TxStatus
	return s, c.Call(ctx, "tx", map[string]any{"tx_hash": hash, "sender_account_id": sender, "wait_until": "FINAL"}, &s)
}
