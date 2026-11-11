package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/services/market-data/internal/hub"
	"github.com/Debanjannnn/nearly/services/market-data/internal/poll"
	"github.com/Debanjannnn/nearly/services/market-data/internal/routes"
	"github.com/Debanjannnn/nearly/services/market-data/internal/store"
	"github.com/Debanjannnn/nearly/services/market-data/internal/venue"
	"github.com/redis/go-redis/v9"
)

// Rhea SIMPLE pools quoted per network (docs/STACK.md): USDC/wNEAR.
var defaultPools = map[string]string{"testnet": "1900", "mainnet": "4512"}

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run() error {
	net := config.Network()
	var e config.Env
	redisURL := e.Str("REDIS_URL")
	rpcURL := e.Opt("RPC_URL", "https://rpc."+net+".near.org")
	archURL := e.Opt("ARCHIVAL_RPC_URL", "https://archival-rpc."+net+".near.org")
	depDir := e.Opt("DEPLOYMENTS_DIR", "contracts/deployments")
	poolCSV := e.Opt("POOL_IDS", defaultPools[net])
	every := e.Duration("POLL_EVERY", 30*time.Second)
	stakeEvery := e.Duration("STAKE_EVERY", time.Hour)
	maxAge := e.Duration("MAX_AGE", 0)
	addr := ":" + e.Opt("PORT", "8082")
	origins := strings.Split(e.Opt("CORS_ORIGINS", "http://localhost:3000"), ",")
	if err := e.Err(); err != nil {
		return err
	}
	if maxAge == 0 {
		maxAge = 3 * every
	}
	pools, err := parseIDs(poolCSV)
	if err != nil {
		return err
	}
	dep, err := config.LoadDeployments(depDir, net)
	if err != nil {
		return err
	}
	ropt, err := redis.ParseURL(redisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(ropt)
	defer rdb.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rpc := near.New(rpcURL)
	st := store.Store{RDB: rdb, Net: net}
	h := hub.New()
	p := &poll.Poller{Venue: venue.Reader{RPC: rpc, Archival: near.New(archURL)}, Dep: dep, PoolIDs: pools,
		Store: st, Publish: h.Publish, StakeEvery: stakeEvery, Now: time.Now}
	srv := &routes.Server{Store: st, Hub: h, PoolIDs: pools, MaxAge: maxAge, Now: time.Now}
	handler := srv.Handler(origins, map[string]httpx.Check{
		"redis": func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
		"rpc":   func(ctx context.Context) error { _, err := rpc.Block(ctx, 0); return err },
	})

	errc := make(chan error, 2)
	go func() { errc <- p.Run(ctx, every) }()
	go func() { errc <- httpx.Run(ctx, addr, handler) }()
	slog.Info("market-data up", "addr", addr, "network", net, "pools", pools)
	err = <-errc
	stop()
	return err
}

func parseIDs(csv string) ([]uint64, error) {
	var out []uint64
	for _, s := range strings.Split(csv, ",") {
		id, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("POOL_IDS=%q: %w", csv, err)
		}
		out = append(out, id)
	}
	return out, nil
}
