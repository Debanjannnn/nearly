// Package market reads the market-data service: snapshot freshness and pool quotes.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Debanjannnn/nearly/internal/shared/planner"
)

type Client struct {
	URL  string
	HTTP *http.Client
}

var ErrStale = errors.New("market data stale")

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.URL + path
	if q != nil {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("market-data %s: http %d", path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Fresh errors unless both prices and rates snapshots are fresh.
func (c *Client) Fresh(ctx context.Context) error {
	for _, p := range []string{"/api/v1/prices", "/api/v1/rates"} {
		var v struct {
			Stale *bool `json:"stale"`
		}
		if err := c.get(ctx, p, nil, &v); err != nil {
			return err
		}
		if v.Stale == nil || *v.Stale {
			return fmt.Errorf("%s: %w", p, ErrStale)
		}
	}
	return nil
}

// Quote is the expected amount_out; market-data refuses stale pools. Satisfies planner.Quoter.
func (c *Client) Quote(ctx context.Context, pool uint64, in, out string, amount *big.Int) (*big.Int, error) {
	q := url.Values{"pool_id": {strconv.FormatUint(pool, 10)}, "token_in": {in}, "token_out": {out}, "amount_in": {amount.String()}}
	var r struct {
		AmountOut string `json:"amount_out"`
	}
	if err := c.get(ctx, "/api/v1/quote", q, &r); err != nil {
		return nil, &planner.Error{Code: "quote_unavailable", Detail: err.Error()}
	}
	v, ok := new(big.Int).SetString(r.AmountOut, 10)
	if !ok {
		return nil, &planner.Error{Code: "quote_unavailable", Detail: "bad amount_out " + r.AmountOut}
	}
	return v, nil
}
