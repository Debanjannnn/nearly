// Package intents is a NEAR Intents 1Click API client: tokens, quote, deposit submit, status.
package intents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const DefaultURL = "https://1click.chaindefuser.com"

type Client struct {
	URL  string
	JWT  string // optional partner JWT; without it 1Click adds a 0.25% fee
	HTTP *http.Client
}

func New(base, jwt string) *Client {
	return &Client{URL: base, JWT: jwt, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Error is a non-2xx 1Click reply.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("1click %d: %s", e.Status, e.Message) }

type Token struct {
	AssetID         string  `json:"assetId"`
	Decimals        int     `json:"decimals"`
	Blockchain      string  `json:"blockchain"`
	Symbol          string  `json:"symbol"`
	Price           float64 `json:"price"`
	ContractAddress string  `json:"contractAddress,omitempty"`
}

type QuoteRequest struct {
	Dry               bool      `json:"dry"`
	SwapType          string    `json:"swapType"`
	SlippageTolerance int       `json:"slippageTolerance"` // bps
	OriginAsset       string    `json:"originAsset"`
	DepositType       string    `json:"depositType"`
	DestinationAsset  string    `json:"destinationAsset"`
	Amount            string    `json:"amount"`
	RefundTo          string    `json:"refundTo"`
	RefundType        string    `json:"refundType"`
	Recipient         string    `json:"recipient"`
	RecipientType     string    `json:"recipientType"`
	Deadline          time.Time `json:"deadline"`
}

type Quote struct {
	DepositAddress     string `json:"depositAddress,omitempty"` // empty on dry quotes
	DepositMemo        string `json:"depositMemo,omitempty"`
	AmountIn           string `json:"amountIn"`
	AmountInFormatted  string `json:"amountInFormatted"`
	AmountInUsd        string `json:"amountInUsd"`
	MinAmountIn        string `json:"minAmountIn"`
	AmountOut          string `json:"amountOut"`
	AmountOutFormatted string `json:"amountOutFormatted"`
	AmountOutUsd       string `json:"amountOutUsd"`
	MinAmountOut       string `json:"minAmountOut"`
	Deadline           string `json:"deadline,omitempty"`
	TimeWhenInactive   string `json:"timeWhenInactive,omitempty"`
	TimeEstimate       int    `json:"timeEstimate"`
}

// QuoteResponse keeps the raw signed reply: 1Click says to store it to settle disputes.
type QuoteResponse struct {
	Quote     Quote           `json:"quote"`
	Signature string          `json:"signature"`
	Raw       json.RawMessage `json:"-"`
}

// Status is a swap's state; SwapDetails is passed through as 1Click sent it.
type Status struct {
	Status      string          `json:"status"`
	UpdatedAt   string          `json:"updatedAt"`
	SwapDetails json.RawMessage `json:"swapDetails"`
}

// Terminal is true once a swap can no longer change.
func Terminal(status string) bool {
	return status == "SUCCESS" || status == "REFUNDED" || status == "FAILED"
}

type SubmitRequest struct {
	TxHash            string `json:"txHash"`
	DepositAddress    string `json:"depositAddress"`
	NearSenderAccount string `json:"nearSenderAccount,omitempty"`
	Memo              string `json:"memo,omitempty"`
}

func (c *Client) Tokens(ctx context.Context) ([]Token, error) {
	var out []Token
	return out, c.do(ctx, http.MethodGet, "/v0/tokens", nil, &out)
}

func (c *Client) Quote(ctx context.Context, q QuoteRequest) (QuoteResponse, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodPost, "/v0/quote", q, &raw); err != nil {
		return QuoteResponse{}, err
	}
	out := QuoteResponse{Raw: raw}
	return out, json.Unmarshal(raw, &out)
}

func (c *Client) Status(ctx context.Context, depositAddress, memo string) (Status, error) {
	v := url.Values{"depositAddress": {depositAddress}}
	if memo != "" {
		v.Set("depositMemo", memo)
	}
	var out Status
	return out, c.do(ctx, http.MethodGet, "/v0/status?"+v.Encode(), nil, &out)
}

// Submit tells 1Click the deposit tx so it picks the swap up sooner.
func (c *Client) Submit(ctx context.Context, s SubmitRequest) (Status, error) {
	var out Status
	return out, c.do(ctx, http.MethodPost, "/v0/deposit/submit", s, &out)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.JWT != "" {
		req.Header.Set("Authorization", "Bearer "+c.JWT)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
		}
		json.Unmarshal(b, &e)
		return &Error{Status: res.StatusCode, Message: e.Message}
	}
	return json.Unmarshal(b, out)
}
