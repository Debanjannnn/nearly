// Package executor is the HTTP client for the executor's POST /v1/execute.
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

type Result struct {
	Index  int    `json:"index"`
	Status string `json:"status"` // ok | failed
	TxHash string `json:"tx_hash"`
	Error  string `json:"error"`
}

type Client struct {
	URL    string
	Secret string
	HTTP   *http.Client
}

// Execute runs steps in order on account; the executor stops at the first failure.
func (c *Client) Execute(ctx context.Context, account, executionID string, steps []planner.Step) ([]Result, error) {
	body, err := json.Marshal(map[string]any{"account_id": account, "execution_id": executionID, "steps": steps})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/v1/execute", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	httpx.SetInternal(req, c.Secret, "api")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("executor http %d", res.StatusCode)
	}
	var out struct {
		Results []Result `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("executor reply: %w", err)
	}
	return out.Results, nil
}
