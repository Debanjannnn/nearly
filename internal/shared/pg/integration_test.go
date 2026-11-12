//go:build integration

package pg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// Applies every migration into a throwaway schema, then walks a position through its states.
func TestMigrations(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	schema := fmt.Sprintf("t_%d", time.Now().UnixNano())
	must := func(sql string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	must("create schema " + schema)
	t.Cleanup(func() { pool.Exec(context.Background(), "drop schema "+schema+" cascade") })
	must("set search_path to " + schema)
	files, _ := filepath.Glob("../../../migrations/*.sql")
	if len(files) == 0 {
		t.Fatal("no migrations")
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		must(string(b))
	}
	must(`insert into users (account_id) values ('alice.testnet')`)
	var basket, pos string
	if err := conn.QueryRow(ctx, `insert into baskets (creator, name, definition) values ('alice.testnet', 'b', '{}') returning id`).Scan(&basket); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `insert into positions (account_id, basket_id, deposit) values ('alice.testnet', $1, 340282366920938463463374607431768211455) returning id`, basket).Scan(&pos); err != nil {
		t.Fatal("u128 max must fit:", err)
	}
	must(`update positions set state = 'withdrawing' where id = $1`, pos)
	if _, err := conn.Exec(ctx, `update positions set state = 'bogus' where id = $1`, pos); err == nil {
		t.Fatal("state check must reject unknown state")
	}
}
