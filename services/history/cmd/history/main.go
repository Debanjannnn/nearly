package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Debanjannnn/nearly/internal/shared/config"
	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/pg"
	"github.com/Debanjannnn/nearly/services/history/internal/ingest"
	"github.com/Debanjannnn/nearly/services/history/internal/routes"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var e config.Env
	// read-only market history; testnet venue rates are not meaningful for backtests
	net := e.Opt("HISTORY_NETWORK", "mainnet")
	dbURL := e.Str("DATABASE_URL")
	archURL := e.Opt("ARCHIVAL_RPC_URL", "https://archival-rpc."+net+".fastnear.com")
	depDir := e.Opt("DEPLOYMENTS_DIR", "contracts/deployments")
	addr := ":" + e.Opt("PORT", "8087")
	every := e.Duration("INGEST_EVERY", 6*time.Hour)
	backfill := e.Int("BACKFILL_DAYS", 180)
	pause := e.Duration("SAMPLE_PAUSE", 2*time.Second)
	ingestOn := e.Bool("INGEST", true)
	origins := strings.Split(e.Opt("CORS_ORIGINS", "http://localhost:3000"), ",")
	if err := e.Err(); err != nil {
		return err
	}
	dep, err := config.LoadDeployments(depDir, net)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := pg.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if ingestOn {
		g := &ingest.Ingester{DB: db, Archival: near.New(archURL), HTTP: &http.Client{Timeout: time.Minute},
			Dep: dep, Backfill: backfill, Pause: pause, Now: time.Now}
		go func() {
			for {
				g.RunAll(ctx)
				select {
				case <-ctx.Done():
					return
				case <-time.After(every):
				}
			}
		}()
	}

	mux := httpx.NewMux(map[string]httpx.Check{"postgres": func(ctx context.Context) error { return db.Ping(ctx) }})
	(&routes.Server{DB: db, Tokens: dep.Tokens, Now: time.Now}).Routes(mux)
	slog.Info("history up", "addr", addr, "network", net)
	return httpx.Run(ctx, addr, httpx.CORS(origins, mux))
}
