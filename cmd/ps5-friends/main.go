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

	"github.com/openpak/ps5-common/config"
	"github.com/openpak/ps5-common/httpx"
	"github.com/openpak/ps5-common/oauth"

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
	s := &server.Server{Store: social.NewMemory()}
	srv := &http.Server{Addr: cfg.Addr, Handler: httpx.Logger(log, s.Handler(signer)), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
