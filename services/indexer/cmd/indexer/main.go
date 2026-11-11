package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/pg"
	"github.com/Debanjannnn/nearly/services/indexer/internal/loop"
	"github.com/Debanjannnn/nearly/services/indexer/internal/source"
	"github.com/Debanjannnn/nearly/services/indexer/internal/store"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run() error {
	net := config.Network()
	var e config.Env
	dbURL := e.Str("DATABASE_URL")
	rpcURL := e.Opt("RPC_URL", "https://rpc."+net+".near.org")
	dataURL := e.Opt("NEARDATA_URL", "https://"+net+".neardata.xyz")
	depDir := e.Opt("DEPLOYMENTS_DIR", "contracts/deployments")
	start := e.Int("START_HEIGHT", 0)
	batch := e.Int("BATCH", 50)
	// free neardata allows ~30 req/min; 429s back off on top of this
	every := e.Duration("POLL_EVERY", 5*time.Second)
	maxLag := e.Int("MAX_LAG", 600)
	full := e.Duration("RECONCILE_EVERY", 5*time.Minute)
	addr := ":" + e.Opt("PORT", "8088")
	if err := e.Err(); err != nil {
		return err
	}
	dep, err := config.LoadDeployments(depDir, net)
	if err != nil {
		return err
	}
	if dep.Factory == "" {
		return errors.New("deployments factory is empty: nothing to index")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := pg.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	x := &loop.Indexer{Src: source.New(dataURL), RPC: near.New(rpcURL), Store: store.Store{DB: db},
		Factory: dep.Factory, Tokens: dep.Tokens, Start: uint64(start), Batch: batch, MaxLag: uint64(maxLag)}
	h := http.NewServeMux()
	h.HandleFunc("GET /health", x.Health)
	h.Handle("/", httpx.NewMux(map[string]httpx.Check{"postgres": func(ctx context.Context) error { return db.Ping(ctx) }}))
	errc := make(chan error, 2)
	go func() { errc <- x.Run(ctx, every, full) }()
	go func() { errc <- httpx.Run(ctx, addr, h) }()
	slog.Info("indexer up", "addr", addr, "network", net, "factory", dep.Factory)
	err = <-errc
	stop()
	return err
}
