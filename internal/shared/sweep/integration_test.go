//go:build integration

package sweep

import (
	"context"
	"testing"

	"github.com/Debanjannnn/nearly/internal/shared/pg/pgtest"
)

func TestOnce(t *testing.T) {
	db, ctx := pgtest.DB(t), context.Background()
	q := func(sql string, args ...any) (id string) {
		t.Helper()
		if err := db.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
		return
	}
	q(`insert into users (account_id) values ('a.t') returning account_id`)
	b := q(`insert into baskets (creator, name, definition) values ('a.t', 'b', '{}') returning id::text`)
	pos := func(state string) string {
		return q(`insert into positions (account_id, basket_id, deposit, state) values ('a.t', $1, 10, $2) returning id::text`, b, state)
	}
	exec := func(p, kind, age string) string {
		return q(`insert into executions (account_id, position_id, kind, status, params, updated_at)
values ('a.t', $1, $2, 'running', '{"held":["7","3"]}', now() - $3::interval) returning id::text`, p, kind, age)
	}
	dep, wd, live := pos("opening"), pos("withdrawing"), pos("opening")
	xDep, xWd, xLive := exec(dep, "deploy", "1 hour"), exec(wd, "withdraw", "1 hour"), exec(live, "deploy", "1 minute")

	if n, err := Once(ctx, db); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	for id, want := range map[string]string{xDep: "failed", xWd: "failed", xLive: "running"} {
		if got := q(`select status from executions where id=$1`, id); got != want {
			t.Fatalf("exec %s: %s want %s", id, got, want)
		}
	}
	for id, want := range map[string]string{dep: `withdrawing ["7", "3"]`, wd: "withdrawing []", live: "opening []"} {
		if got := q(`select state || ' ' || held::text from positions where id=$1`, id); got != want {
			t.Fatalf("position %s: %q want %q", id, got, want)
		}
	}
	if n, err := Once(ctx, db); err != nil || n != 0 {
		t.Fatal("second sweep must be a no-op", n, err)
	}
}
