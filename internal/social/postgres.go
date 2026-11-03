package social

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openpak/ps5-common/pgdb"
)

// Schema is the idempotent DDL of the durable friend graph. Friendships are stored in both
// directions (like Memory) so a list is one index scan.
const Schema = `
CREATE TABLE IF NOT EXISTS ps5_friends (
	account_id text NOT NULL,
	friend_id  text NOT NULL,
	since      timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (account_id, friend_id)
);
CREATE TABLE IF NOT EXISTS ps5_friend_requests (
	from_id    text NOT NULL,
	to_id      text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (from_id, to_id)
);
CREATE INDEX IF NOT EXISTS ps5_friend_requests_to ON ps5_friend_requests (to_id);
CREATE TABLE IF NOT EXISTS ps5_blocks (
	blocker_id text NOT NULL,
	blocked_id text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (blocker_id, blocked_id)
);
CREATE INDEX IF NOT EXISTS ps5_blocks_blocked ON ps5_blocks (blocked_id);
`

const opTimeout = 5 * time.Second

// Postgres keeps friends, requests and blocks in Postgres. Presence is transient and stays in
// the embedded Memory (it is re-announced by the console on every sign-in).
type Postgres struct {
	*Memory
	pool *pgxpool.Pool
	log  *slog.Logger
}

var _ Store = (*Postgres)(nil)

// NewPostgres migrates the schema and returns the store. The caller owns the pool.
func NewPostgres(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (*Postgres, error) {
	if err := pgdb.Migrate(ctx, pool, Schema); err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &Postgres{Memory: NewMemory(), pool: pool, log: log}, nil
}

func (p *Postgres) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}

// col returns the ids in column pick of table where filter = id, sorted. Reads that fail log and
// answer empty (a degraded list is better than a console error loop); writes return their errors.
func (p *Postgres) col(sql, id string) []string {
	ctx, cancel := p.ctx()
	defer cancel()
	rows, err := p.pool.Query(ctx, sql, id)
	if err != nil {
		p.log.Error("social read", "err", err)
		return []string{}
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		p.log.Error("social read", "err", err)
		return []string{}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func (p *Postgres) Friends(id string) []string {
	return p.col(`SELECT friend_id FROM ps5_friends WHERE account_id=$1 ORDER BY friend_id`, id)
}
func (p *Postgres) Received(id string) []string {
	return p.col(`SELECT from_id FROM ps5_friend_requests WHERE to_id=$1 ORDER BY from_id`, id)
}
func (p *Postgres) Sent(id string) []string {
	return p.col(`SELECT to_id FROM ps5_friend_requests WHERE from_id=$1 ORDER BY to_id`, id)
}
func (p *Postgres) Blocks(id string) []string {
	return p.col(`SELECT blocked_id FROM ps5_blocks WHERE blocker_id=$1 ORDER BY blocked_id`, id)
}

func (p *Postgres) Relation(id, other string) Relation {
	ctx, cancel := p.ctx()
	defer cancel()
	var friend, sent, recv bool
	err := p.pool.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM ps5_friends WHERE account_id=$1 AND friend_id=$2),
		EXISTS(SELECT 1 FROM ps5_friend_requests WHERE from_id=$1 AND to_id=$2),
		EXISTS(SELECT 1 FROM ps5_friend_requests WHERE from_id=$2 AND to_id=$1)`, id, other).Scan(&friend, &sent, &recv)
	if err != nil {
		p.log.Error("social read", "err", err)
		return RelNone
	}
	switch {
	case friend:
		return RelFriend
	case sent:
		return RelRequested
	case recv:
		return RelRequesting
	}
	return RelNone
}

func (p *Postgres) IsBlocked(a, b string) bool {
	ctx, cancel := p.ctx()
	defer cancel()
	var blocked bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ps5_blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1))`, a, b).Scan(&blocked)
	if err != nil {
		p.log.Error("social read", "err", err)
		return false
	}
	return blocked
}

// pairTx runs fn in a transaction holding an advisory lock on the unordered pair, so concurrent
// request/accept/block calls on the same two accounts serialise.
func (p *Postgres) pairTx(a, b string, fn func(ctx context.Context, tx pgx.Tx) error) error {
	ctx, cancel := p.ctx()
	defer cancel()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lo+"|"+hi); err != nil {
		return err
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func exists(ctx context.Context, tx pgx.Tx, sql string, args ...any) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(`+sql+`)`, args...).Scan(&ok)
	return ok, err
}

func (p *Postgres) Request(from, to string) (Outcome, error) {
	if from == to {
		return 0, ErrSelf
	}
	var out Outcome
	err := p.pairTx(from, to, func(ctx context.Context, tx pgx.Tx) error {
		if blocked, err := exists(ctx, tx, `SELECT 1 FROM ps5_blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)`, from, to); err != nil {
			return err
		} else if blocked {
			return ErrBlocked
		}
		if ok, err := exists(ctx, tx, `SELECT 1 FROM ps5_friends WHERE account_id=$1 AND friend_id=$2`, from, to); err != nil {
			return err
		} else if ok {
			out = AlreadyFriends
			return nil
		}
		if ok, err := exists(ctx, tx, `SELECT 1 FROM ps5_friend_requests WHERE from_id=$2 AND to_id=$1`, from, to); err != nil {
			return err
		} else if ok {
			if _, err := tx.Exec(ctx, `DELETE FROM ps5_friend_requests WHERE from_id=$1 AND to_id=$2`, to, from); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ps5_friends (account_id, friend_id) VALUES ($1,$2), ($2,$1) ON CONFLICT DO NOTHING`, from, to); err != nil {
				return err
			}
			out = Accepted
			return nil
		}
		tag, err := tx.Exec(ctx, `INSERT INTO ps5_friend_requests (from_id, to_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, from, to)
		if err != nil {
			return err
		}
		out = Sent
		if tag.RowsAffected() == 0 {
			out = AlreadySent
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return out, nil
}

// sever drops the friendship and both request directions and reports whether any existed.
func sever(ctx context.Context, tx pgx.Tx, a, b string) (bool, error) {
	t1, err := tx.Exec(ctx, `DELETE FROM ps5_friends WHERE (account_id=$1 AND friend_id=$2) OR (account_id=$2 AND friend_id=$1)`, a, b)
	if err != nil {
		return false, err
	}
	t2, err := tx.Exec(ctx, `DELETE FROM ps5_friend_requests WHERE (from_id=$1 AND to_id=$2) OR (from_id=$2 AND to_id=$1)`, a, b)
	if err != nil {
		return false, err
	}
	return t1.RowsAffected()+t2.RowsAffected() > 0, nil
}

func (p *Postgres) Remove(id, other string) error {
	return p.pairTx(id, other, func(ctx context.Context, tx pgx.Tx) error {
		any, err := sever(ctx, tx, id, other)
		if err != nil {
			return err
		}
		if !any {
			return ErrNothing
		}
		return nil
	})
}

func (p *Postgres) Block(id, other string) error {
	if id == other {
		return ErrSelf
	}
	return p.pairTx(id, other, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := sever(ctx, tx, id, other); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ps5_blocks (blocker_id, blocked_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, other)
		return err
	})
}

func (p *Postgres) Unblock(id, other string) error {
	ctx, cancel := p.ctx()
	defer cancel()
	tag, err := p.pool.Exec(ctx, `DELETE FROM ps5_blocks WHERE blocker_id=$1 AND blocked_id=$2`, id, other)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNothing
	}
	return nil
}
