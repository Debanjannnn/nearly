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
	"github.com/Debanjannnn/nearly/internal/shared/executor"
	"github.com/Debanjannnn/nearly/internal/shared/httpx"
	"github.com/Debanjannnn/nearly/internal/shared/intents"
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/pg"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/internal/shared/sweep"
	"github.com/Debanjannnn/nearly/services/api/internal/api"
	"github.com/Debanjannnn/nearly/services/api/internal/auth"
	"github.com/Debanjannnn/nearly/services/api/internal/chain"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	network := config.Network()
	var e config.Env
	dbURL, secret, execURL, internal := e.Str("DATABASE_URL"), e.Str("SESSION_SECRET"), e.Str("EXECUTOR_URL"), e.Str("INTERNAL_SECRET")
	execKey := e.Str("EXECUTOR_PUBLIC_KEY")
	rpcURL := e.Opt("RPC_URL", "https://rpc."+network+".near.org")
	depDir := e.Opt("DEPLOYMENTS_DIR", "contracts/deployments")
	ttl, slip := e.Duration("SESSION_TTL", 24*time.Hour), e.Int("SLIPPAGE_BPS", 50)
	addr := ":" + e.Opt("PORT", "8080")
	ic := intents.New(e.Opt("INTENTS_URL", intents.DefaultURL), e.Opt("INTENTS_JWT", ""))
	origins := strings.Split(e.Opt("CORS_ORIGINS", "http://localhost:3000"), ",")
	if err := e.Err(); err != nil {
		return err
	}
	if len(secret) < 32 {
		return errors.New("SESSION_SECRET must be at least 32 bytes")
	}
	dep, err := config.LoadDeployments(depDir, network)
	if err != nil {
		return err
	}
	db, err := pg.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	go sweep.Run(ctx, db, 5*time.Minute)
	ch := &chain.Chain{RPC: near.New(rpcURL), Dep: dep, ExecutorKey: execKey}
	mux := httpx.NewMux(map[string]httpx.Check{"postgres": db.Ping})
	srv := &api.Server{
		Store:       api.Store{DB: db},
		Chain:       ch,
		Exec:        &executor.Client{URL: strings.TrimRight(execURL, "/"), Secret: internal, Caller: "api", HTTP: &http.Client{Timeout: executor.MaxRun}},
		Sessions:    &auth.Sessions{Secret: []byte(secret), TTL: ttl},
		Plan:        &planner.Planner{Tokens: dep.Tokens, Q: ch, SlippageBps: slip},
		Dep:         dep,
		ExecutorKey: execKey,
		Intents:     ic,
	}
	srv.Routes(mux)
	slog.Info("listening", "addr", addr, "network", network)
	err = httpx.Run(ctx, addr, httpx.CORS(origins, mux))
	srv.Wait() // let in-flight executor runs record results before db closes
	return err
}
