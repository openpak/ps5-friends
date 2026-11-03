package social

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/openpak/ps5-common/pgdb"
)

func openPG(t *testing.T) *Postgres {
	t.Helper()
	url := os.Getenv("PS5_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PS5_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgdb.Open(ctx, url, 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	st, err := NewPostgres(ctx, pool, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE ps5_friends, ps5_friend_requests, ps5_blocks`); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestPostgresGraphSurvivesRestart(t *testing.T) {
	st := openPG(t)
	if o, err := st.Request("1", "2"); err != nil || o != Sent {
		t.Fatal(o, err)
	}
	if o, _ := st.Request("1", "2"); o != AlreadySent {
		t.Fatal(o)
	}
	if o, err := st.Request("2", "1"); err != nil || o != Accepted {
		t.Fatal(o, err)
	}
	if o, _ := st.Request("1", "2"); o != AlreadyFriends {
		t.Fatal(o)
	}
	st.Request("3", "1") // pending request to 1
	if err := st.Block("1", "4"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Request("4", "1"); err != ErrBlocked {
		t.Fatal(err)
	}

	// New instance on the same database: the whole graph is still there.
	ctx := context.Background()
	pool, err := pgdb.Open(ctx, os.Getenv("PS5_TEST_DATABASE_URL"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	st2, err := NewPostgres(ctx, pool, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if f := st2.Friends("1"); len(f) != 1 || f[0] != "2" {
		t.Fatal(f)
	}
	if f := st2.Friends("2"); len(f) != 1 || f[0] != "1" {
		t.Fatal(f)
	}
	if r := st2.Received("1"); len(r) != 1 || r[0] != "3" {
		t.Fatal(r)
	}
	if r := st2.Sent("3"); len(r) != 1 || r[0] != "1" {
		t.Fatal(r)
	}
	if b := st2.Blocks("1"); len(b) != 1 || b[0] != "4" || !st2.IsBlocked("4", "1") {
		t.Fatal(b)
	}
	if st2.Relation("1", "2") != RelFriend || st2.Relation("1", "3") != RelRequesting || st2.Relation("3", "1") != RelRequested || st2.Relation("1", "9") != RelNone {
		t.Fatal("relations")
	}
	// Blocking severs a friendship and survives too.
	if err := st2.Block("1", "2"); err != nil {
		t.Fatal(err)
	}
	if len(st2.Friends("2")) != 0 || !st2.IsBlocked("2", "1") {
		t.Fatal("block did not sever")
	}
	if err := st2.Unblock("1", "2"); err != nil {
		t.Fatal(err)
	}
	if st2.Unblock("1", "2") != ErrNothing || st2.Remove("1", "9") != ErrNothing || st2.Block("1", "1") != ErrSelf {
		t.Fatal("sentinels")
	}
}
