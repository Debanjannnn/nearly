// Package routes serves cached rates, prices and pool quotes read-only from Redis.
package routes

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/services/market-data/internal/hub"
	"github.com/Debanjannnn/nearly/services/market-data/internal/quote"
	"github.com/Debanjannnn/nearly/services/market-data/internal/store"
	"github.com/redis/go-redis/v9"
)

var u128RE = regexp.MustCompile(`^[0-9]{1,39}$`)

const (
	defaultSlip   = 50
	defaultImpact = 100
)

type Server struct {
	Store   store.Store
	Hub     *hub.Hub
	PoolIDs []uint64
	// MaxAge: a value older than this is stale; quotes on a stale pool are refused.
	MaxAge time.Duration
	// StakeEvery widens MaxAge for staking rates, which refresh only that often.
	StakeEvery time.Duration
	Now        func() time.Time
}

// Handler serves the routes; origins are CORS origins (scheme://host[:port] or "*"), also allowed on the WebSocket.
func (s *Server) Handler(origins []string, checks map[string]httpx.Check) http.Handler {
	mux := httpx.NewMux(checks)
	for _, o := range origins {
		if _, host, ok := strings.Cut(o, "://"); ok {
			o = host
		}
		s.Hub.OriginPatterns = append(s.Hub.OriginPatterns, o)
	}
	mux.HandleFunc("GET /api/v1/ws", s.Hub.Serve)
	mux.HandleFunc("GET /api/v1/rates", s.rates)
	mux.HandleFunc("GET /api/v1/prices", s.prices)
	mux.HandleFunc("GET /api/v1/quote", s.quote)
	return httpx.CORS(origins, mux)
}

func (s *Server) stale(ts int64) bool { return s.Now().Sub(time.UnixMilli(ts)) > s.MaxAge }

// load reads a key; false means a response was already written.
func (s *Server) load(w http.ResponseWriter, ctx context.Context, key string, out any) bool {
	err := s.Store.Get(ctx, key, out)
	if errors.Is(err, redis.Nil) {
		httpx.Error(w, http.StatusServiceUnavailable, "unavailable")
		return false
	}
	if err != nil {
		slog.Error("redis get", "key", key, "err", err)
		httpx.Error(w, http.StatusServiceUnavailable, "unavailable")
		return false
	}
	return true
}

func (s *Server) rates(w http.ResponseWriter, r *http.Request) {
	var v []store.Rate
	if s.load(w, r.Context(), store.KeyRates, &v) {
		stale := slices.ContainsFunc(v, func(x store.Rate) bool {
			if x.Venue == "staking" {
				return s.Now().Sub(time.UnixMilli(x.Ts)) > s.MaxAge+s.StakeEvery
			}
			return s.stale(x.Ts)
		})
		httpx.JSON(w, http.StatusOK, map[string]any{"rates": v, "stale": stale || len(v) == 0})
	}
}

func (s *Server) prices(w http.ResponseWriter, r *http.Request) {
	var v []store.Price
	if s.load(w, r.Context(), store.KeyPrices, &v) {
		stale := slices.ContainsFunc(v, func(x store.Price) bool { return s.stale(x.Ts) })
		httpx.JSON(w, http.StatusOK, map[string]any{"prices": v, "stale": stale || len(v) == 0})
	}
}

func bpsParam(q string, def int64) (int64, bool) {
	if q == "" {
		return def, true
	}
	n, err := strconv.ParseInt(q, 10, 64)
	return n, err == nil && n >= 0 && n <= 10_000
}

// quote: ?token_in&token_out&amount_in[&pool_id][&slippage_bps=50][&max_impact_bps=100].
// Without pool_id the first configured pool holding both tokens is used.
func (s *Server) quote(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	in, out, amt := q.Get("token_in"), q.Get("token_out"), q.Get("amount_in")
	slip, ok1 := bpsParam(q.Get("slippage_bps"), defaultSlip)
	impact, ok2 := bpsParam(q.Get("max_impact_bps"), defaultImpact)
	ids := s.PoolIDs
	if v := q.Get("pool_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil || !slices.Contains(s.PoolIDs, id) {
			httpx.Error(w, http.StatusBadRequest, "unknown_pool")
			return
		}
		ids = []uint64{id}
	}
	if !ok1 || !ok2 || in == "" || out == "" || !u128RE.MatchString(amt) {
		httpx.Error(w, http.StatusBadRequest, "bad_request")
		return
	}
	down := false
	for _, id := range ids {
		var p quote.Pool
		if err := s.Store.Get(r.Context(), store.KeyPool(id), &p); err != nil {
			if !errors.Is(err, redis.Nil) {
				slog.Error("redis get", "pool", id, "err", err)
				down = true
			}
			continue
		}
		if !p.Has(in, out) {
			continue
		}
		if s.stale(p.Ts) {
			httpx.Error(w, http.StatusServiceUnavailable, "stale")
			return
		}
		a, _ := new(big.Int).SetString(amt, 10)
		res, err := quote.Simple(p, in, out, a, slip, impact)
		switch {
		case err == nil:
			httpx.JSON(w, http.StatusOK, res)
		case errors.Is(err, quote.ErrNoLiquidity):
			httpx.Error(w, http.StatusServiceUnavailable, err.Error())
		default:
			httpx.Error(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if down {
		httpx.Error(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	httpx.Error(w, http.StatusNotFound, "no_pool")
}
