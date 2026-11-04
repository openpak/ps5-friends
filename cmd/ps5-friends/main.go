// Command ps5-friends serves the friends, blocks and basic presence parts of
// the PSN userProfile API (m.np.playstation.com) for the PlayStation 5.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openpak/ps5-common/accountcore"
	"github.com/openpak/ps5-common/config"
	"github.com/openpak/ps5-common/erase"
	"github.com/openpak/ps5-common/httpx"
	"github.com/openpak/ps5-common/oauth"
	"github.com/openpak/ps5-common/pgdb"

	"openpak/ps5-friends/internal/server"
	"openpak/ps5-friends/internal/social"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(2)
	}
	if cfg.TokenKey == "" {
		log.Error("config", "err", "PS5_TOKEN_KEY is required")
		os.Exit(2)
	}
	signer, err := oauth.NewSigner([]byte(cfg.TokenKey))
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var store social.Store
	if cfg.DatabaseURL == "" && config.DevMemory(os.Getenv) {
		log.Warn("PS5_DEV_MEMORY=1: friends are kept in memory and lost on restart (development only)")
		store = social.NewMemory()
	} else {
		pool, err := pgdb.Open(ctx, cfg.DatabaseURL, pgdb.DefaultMaxConns)
		if err != nil {
			log.Error("database", "err", err)
			os.Exit(2)
		}
		defer pool.Close()
		if store, err = social.NewPostgres(ctx, pool, log); err != nil {
			log.Error("database schema", "err", err)
			os.Exit(2)
		}
		log.Info("friends stored in Postgres")
	}
	s := &server.Server{Store: store}
	var core accountcore.Core
	if cfg.CoreURL != "" {
		c, err := accountcore.New(cfg.CoreURL, cfg.CoreToken, nil)
		if err != nil {
			log.Error("account core", "err", err)
			os.Exit(2)
		}
		core = c
	}
	eraseKey := os.Getenv("ACCOUNT_ERASE_KEY")
	go erase.Serve(ctx, os.Getenv("ERASE_ADDR"), eraseKey, erase.Handler("ps5-friends", eraseKey, core,
		func(_ context.Context, ids []string) (int, error) { return store.Erase(ids) }, log), log)
	srv := &http.Server{Addr: cfg.Addr, Handler: httpx.Logger(log, s.Handler(signer)), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Info("ps5-friends listening", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
