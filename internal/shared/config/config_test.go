package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnv(t *testing.T) {
	t.Setenv("A", "x")
	t.Setenv("N", "7")
	t.Setenv("BAD", "nope")
	t.Setenv("D", "3s")
	var e Env
	if e.Str("A") != "x" || e.Opt("MISSING_OPT", "d") != "d" || e.Int("N", 0) != 7 || e.Duration("D", 0) != 3*time.Second {
		t.Fatal("wrong values")
	}
	if e.Err() != nil {
		t.Fatal(e.Err())
	}
	e.Str("MISSING_REQ")
	e.Int("BAD", 1)
	err := e.Err()
	if err == nil || !strings.Contains(err.Error(), "MISSING_REQ") || !strings.Contains(err.Error(), "BAD") {
		t.Fatalf("want both errors, got %v", err)
	}
}

func TestLoadDeployments(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "testnet.json"), []byte(`{"root":"nearly.testnet","treasury":"t.testnet",
		"tokens":{"usdc":"usdc.testnet","wnear":"wrap.testnet"},"venues":{"dex":"dex.testnet"}}`), 0o600)
	d, err := LoadDeployments(dir, "testnet")
	if err != nil {
		t.Fatal(err)
	}
	if d.Tokens["wnear"] != "wrap.testnet" || d.Venues.Dex != "dex.testnet" {
		t.Fatalf("bad parse %+v", d)
	}
	os.WriteFile(filepath.Join(dir, "mainnet.json"), []byte(`{}`), 0o600)
	if _, err := LoadDeployments(dir, "mainnet"); err == nil {
		t.Fatal("want missing tokens error")
	}
	if _, err := LoadDeployments(dir, "../x"); err == nil {
		t.Fatal("want bad network error")
	}
}

func TestNetwork(t *testing.T) {
	t.Setenv("NEAR_NETWORK", "")
	if Network() != "testnet" {
		t.Fatal("default")
	}
}
