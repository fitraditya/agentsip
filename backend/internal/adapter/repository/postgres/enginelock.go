package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// engineAdvisoryLockKey identifies this application's singleton lock. The
// value is arbitrary but must never change: two builds using different keys
// would not see each other's lock, which defeats the entire point. Postgres
// advisory locks share one namespace per database, so this only needs to be
// distinct from other applications' keys (including obrelin's own) on the
// same database.
const engineAdvisoryLockKey int64 = 6462_7107_19

// ErrEngineAlreadyRunning means another agentsip engine process holds the lock
// on this database.
var ErrEngineAlreadyRunning = errors.New("another agentsip engine is already running against this database")

// EngineLock enforces one-engine-process-per-database sharding, matching
// obrelin's Decision 9/33: scaling is done by running more engine+database
// pairs, not by pointing several interchangeable instances at the same
// database. agentsip has no reconciliation sweep to corrupt the way obrelin's
// does, but the same race exists for its own in-process state — Admission's
// per-trunk counters and PendingCalls' webhook registry are both per-process
// and would double-count or silently drop decisions if two instances served
// the same configuration.
//
// The lock is a Postgres session-level advisory lock held on a dedicated
// connection — dedicated rather than borrowed from the pool because a pooled
// connection returns to the pool and can be handed out or recycled, which
// would drop the lock.
//
// Residual window, same as obrelin's: if this connection drops (network
// blip, database restart, operator terminating the backend), Postgres
// releases the lock while this process keeps running, and a second instance
// could then start. Not guarded, for the same reason obrelin leaves it
// unguarded — killing a healthy engine mid-call on a transient database blip
// is worse than the narrow race it would prevent.
type EngineLock struct {
	conn *pgx.Conn
}

// AcquireEngineLock takes the lock, or returns ErrEngineAlreadyRunning if
// another process holds it. It never waits: a caller that blocked here would
// hang at boot with no indication why.
func AcquireEngineLock(ctx context.Context, databaseURL string) (*EngineLock, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening the engine lock connection: %w", err)
	}

	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, engineAdvisoryLockKey).Scan(&acquired); err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("acquiring the engine lock: %w", err)
	}
	if !acquired {
		_ = conn.Close(ctx)
		return nil, ErrEngineAlreadyRunning
	}

	return &EngineLock{conn: conn}, nil
}

// Release drops the lock by closing its session. Postgres releases it on
// disconnection anyway, including if this process dies without calling
// Release — that is what makes the lock safe against a crash, rather than
// leaving a stale marker a human has to clear.
func (l *EngineLock) Release(ctx context.Context) error {
	return l.conn.Close(ctx)
}
