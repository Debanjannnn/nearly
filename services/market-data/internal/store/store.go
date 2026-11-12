// Package store keeps the latest market snapshots in Redis as JSON under md:<network>:<key>.
package store

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const (
	KeyRates  = "rates"
	KeyPrices = "prices"
)

func KeyPool(id uint64) string { return "pool:" + strconv.FormatUint(id, 10) }

// Ts on every value is unix ms of its last successful read; readers judge freshness from it.
type Rate struct {
	Venue    string  `json:"venue"` // lending | staking
	Contract string  `json:"contract"`
	Token    string  `json:"token"`
	APY      float64 `json:"apy"`
	Ts       int64   `json:"ts"`
}

type Price struct {
	Token  string  `json:"token"`
	Symbol string  `json:"symbol"`
	USD    float64 `json:"usd"`
	Ts     int64   `json:"ts"`
}

type Store struct {
	RDB redis.Cmdable
	Net string
}

func (s Store) key(k string) string { return "md:" + s.Net + ":" + k }

func (s Store) Put(ctx context.Context, k string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.RDB.Set(ctx, s.key(k), b, 0).Err()
}

// Get returns redis.Nil when the key was never written.
func (s Store) Get(ctx context.Context, k string, out any) error {
	b, err := s.RDB.Get(ctx, s.key(k)).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
