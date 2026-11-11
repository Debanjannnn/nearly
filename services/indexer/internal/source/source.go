// Package source reads final NEAR blocks with their receipt outcomes from neardata (FastNEAR, free, no key).
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	URL  string // e.g. https://testnet.neardata.xyz
	HTTP *http.Client
}

func New(url string) *Client {
	return &Client{URL: strings.TrimRight(url, "/"), HTTP: &http.Client{
		Timeout: 30 * time.Second,
		// last_block/final redirects to /v0/block/<h>; the height is all we need
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type Block struct {
	Block struct {
		Header struct {
			Height    uint64 `json:"height"`
			Timestamp uint64 `json:"timestamp"` // unix nanoseconds
		} `json:"header"`
	} `json:"block"`
	Shards []struct {
		Outcomes []Outcome `json:"receipt_execution_outcomes"`
	} `json:"shards"`
}

type Outcome struct {
	TxHash           string `json:"tx_hash"`
	ExecutionOutcome struct {
		ID      string `json:"id"` // receipt id
		Outcome struct {
			ExecutorID string                     `json:"executor_id"`
			Logs       []string                   `json:"logs"`
			Status     map[string]json.RawMessage `json:"status"`
		} `json:"outcome"`
	} `json:"execution_outcome"`
}

func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.HTTP.Do(req)
}

// Final is the height of the latest final block.
func (c *Client) Final(ctx context.Context) (uint64, error) {
	res, err := c.get(ctx, "/v0/last_block/final")
	if err != nil {
		return 0, err
	}
	res.Body.Close()
	loc := res.Header.Get("Location")
	h, err := strconv.ParseUint(loc[strings.LastIndexByte(loc, '/')+1:], 10, 64)
	if res.StatusCode/100 != 3 || err != nil {
		return 0, fmt.Errorf("last_block/final: %d location %q", res.StatusCode, loc)
	}
	return h, nil
}

// Block returns the block at h, or nil when that height was skipped.
func (c *Client) Block(ctx context.Context, h uint64) (*Block, error) {
	res, err := c.get(ctx, fmt.Sprintf("/v0/block/%d", h))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 200))
		return nil, fmt.Errorf("block %d: %d %s", h, res.StatusCode, b)
	}
	var b *Block
	if err := json.NewDecoder(res.Body).Decode(&b); err != nil {
		return nil, fmt.Errorf("block %d: %w", h, err)
	}
	return b, nil
}
