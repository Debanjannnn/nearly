package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const LlamaURL = "https://coins.llama.fi/chart/"

// Coins maps token symbol -> DefiLlama coin id; usdc is the base and is taken as 1 USD.
var Coins = map[string]string{"wnear": "coingecko:near"}

type llamaChart struct {
	Coins map[string]struct {
		Prices []struct {
			Timestamp int64   `json:"timestamp"`
			Price     float64 `json:"price"`
		} `json:"prices"`
	} `json:"coins"`
}

// parseLlama returns day -> USD; the last point of a day wins.
func parseLlama(c llamaChart, coin string) map[string]float64 {
	out := map[string]float64{}
	for _, p := range c.Coins[coin].Prices {
		out[time.Unix(p.Timestamp, 0).UTC().Format(Day)] = p.Price
	}
	return out
}

// Prices refetches from the last stored day (or the backfill start) to today.
func (g *Ingester) Prices(ctx context.Context) error {
	for sym, coin := range Coins {
		start := g.today().AddDate(0, 0, -g.Backfill)
		var last *time.Time
		if err := g.DB.QueryRow(ctx, `select max(day) from series_price where token = $1`, sym).Scan(&last); err != nil {
			return err
		}
		if last != nil && last.After(start) {
			start = *last
		}
		// DefiLlama caps a chart at 500 points
		span := min(int(g.today().Sub(start).Hours()/24)+1, 500)
		url := fmt.Sprintf("%s%s?start=%d&span=%d&period=1d", LlamaURL, coin, start.Unix(), span)
		var c llamaChart
		if err := g.getJSON(ctx, url, &c); err != nil {
			return err
		}
		b := &pgx.Batch{}
		for day, usd := range parseLlama(c, coin) {
			b.Queue(`insert into series_price (token, day, usd) values ($1, $2, $3)
				on conflict (token, day) do update set usd = excluded.usd`, sym, day, usd)
		}
		if err := g.DB.SendBatch(ctx, b).Close(); err != nil {
			return err
		}
	}
	return nil
}

func (g *Ingester) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("GET %s: %d %s", url, resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
