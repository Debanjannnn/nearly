package main

import (
	"context"
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
	"github.com/Debanjannnn/nearly/internal/shared/near"
	"github.com/Debanjannnn/nearly/internal/shared/pg"
	"github.com/Debanjannnn/nearly/internal/shared/planner"
	"github.com/Debanjannnn/nearly/services/scheduler/internal/keys"
	"github.com/Debanjannnn/nearly/services/scheduler/internal/market"
	"github.com/Debanjannnn/nearly/services/scheduler/internal/sched"
)

func main() {
	if err := run(); err != nil {
		slog.Error("scheduler exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	network := config.Network()
	var e config.Env
	dbURL, execURL, internal := e.Str("DATABASE_URL"), e.Str("EXECUTOR_URL"), e.Str("INTERNAL_SECRET")
	execKey, mdURL := e.Str("EXECUTOR_PUBLIC_KEY"), e.Str("MARKET_DATA_URL")
	rpcURL := e.Opt("RPC_URL", "https://rpc."+network+".near.org")
	depDir := e.Opt("DEPLOYMENTS_DIR", "contracts/deployments")
	tick, slip := e.Duration("TICK", 10*time.Second), e.Int("SLIPPAGE_BPS", 50)
	addr := ":" + e.Opt("PORT", "8084")
	if err := e.Err(); err != nil {
		return err
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
	md := &market.Client{URL: strings.TrimRight(mdURL, "/"), HTTP: &http.Client{Timeout: 10 * time.Second}}
	s := &sched.Scheduler{
		Store:  sched.Store{DB: db},
		Plan:   &planner.Planner{Tokens: dep.Tokens, LST: dep.Venues.Staking, LSTPool: dep.LSTPool, Q: md, SlippageBps: slip},
		Market: md,
		Keys:   &keys.Checker{RPC: near.New(rpcURL), ExecutorKey: execKey},
		Exec:   &executor.Client{URL: strings.TrimRight(execURL, "/"), Secret: internal, Caller: "scheduler", HTTP: &http.Client{Timeout: 15 * time.Minute}},
		Tick:   tick,
	}
	ran := make(chan struct{})
	go func() { s.Run(ctx); close(ran) }()
	slog.Info("listening", "addr", addr, "network", network)
	err = httpx.Run(ctx, addr, httpx.NewMux(map[string]httpx.Check{"postgres": db.Ping, "market-data": md.Fresh}))
	stop()
	<-ran
	s.Wait() // let in-flight runs record results before db closes
	return err
}
