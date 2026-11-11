// Package config reads service env vars and the contracts/deployments/<network>.json file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Env collects every missing/invalid var so a service reports them all at once.
//
//	var e config.Env
//	rpc := e.Str("NEAR_RPC_URL")
//	if err := e.Err(); err != nil { ... }
type Env struct{ errs []error }

func (e *Env) Str(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		e.errs = append(e.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (e *Env) Opt(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func (e *Env) Int(key string, def int) int { return parse(e, key, def, strconv.Atoi) }

func (e *Env) Bool(key string, def bool) bool { return parse(e, key, def, strconv.ParseBool) }

func (e *Env) Duration(key string, def time.Duration) time.Duration {
	return parse(e, key, def, time.ParseDuration)
}

func (e *Env) Err() error { return errors.Join(e.errs...) }

func parse[T any](e *Env, key string, def T, fn func(string) (T, error)) T {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := fn(raw)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q: %w", key, raw, err))
		return def
	}
	return v
}

// Network is env NEAR_NETWORK, default testnet.
func Network() string {
	if n := strings.TrimSpace(os.Getenv("NEAR_NETWORK")); n != "" {
		return n
	}
	return "testnet"
}

type Deployments struct {
	Root     string            `json:"root"`
	Factory  string            `json:"factory"`
	Treasury string            `json:"treasury"`
	Tokens   map[string]string `json:"tokens"`
	Venues   struct {
		Lending string `json:"lending"`
		Dex     string `json:"dex"`
		Staking string `json:"staking"`
	} `json:"venues"`
}

// LoadDeployments reads <dir>/<network>.json, e.g. LoadDeployments("contracts/deployments", Network()).
func LoadDeployments(dir, network string) (*Deployments, error) {
	if network != "testnet" && network != "mainnet" {
		return nil, fmt.Errorf("network %q, want testnet or mainnet", network)
	}
	path := filepath.Join(dir, network+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Deployments
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if d.Treasury == "" || d.Tokens["usdc"] == "" || d.Tokens["wnear"] == "" {
		return nil, fmt.Errorf("%s: treasury, tokens.usdc or tokens.wnear missing", path)
	}
	return &d, nil
}
