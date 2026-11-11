// Package sweep fails executions a crashed or killed api/scheduler left in flight.
package sweep

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Debanjannnn/nearly/internal/shared/executor"
)

// Stale is older than any live run can be: executor budget plus time to record results.
const Stale = executor.MaxRun + time.Minute

// Once fails runs in flight past Stale: a dead withdraw stays resumable, a dead deploy's position goes withdrawing for the owner to unwind.
// shortcut: a dead deploy's paid fee is not booked in fee_ledger; book it from chain if that matters.
func Once(ctx context.Context, db *pgxpool.Pool) (n int64, err error) {
	err = db.QueryRow(ctx, `with dead as (
  update executions set status='failed', error='stale', updated_at=now()
  where status in ('pending','running') and updated_at < now() - make_interval(secs => $1)
  returning kind, position_id, params
), reopen as (
  update positions p set state='withdrawing', held=coalesce(d.params->'held', '[]'), updated_at=now()
  from dead d where p.id=d.position_id and d.kind='deploy' and p.state='opening'
)
select count(*) from dead`, Stale.Seconds()).Scan(&n)
	return
}

// Run sweeps now, then every `every`, until ctx ends.
func Run(ctx context.Context, db *pgxpool.Pool, every time.Duration) {
	for {
		if n, err := Once(ctx, db); err != nil {
			slog.Error("sweep stale executions", "err", err)
		} else if n > 0 {
			slog.Warn("failed stale executions", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
