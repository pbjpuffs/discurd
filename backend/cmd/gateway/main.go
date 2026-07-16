// Command gateway is the Discurd WebSocket service (docs/ARCHITECTURE.md §8).
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gocql/gocql"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"discurd/internal/auth"
	"discurd/internal/config"
	"discurd/internal/events"
	"discurd/internal/obs"
	"discurd/internal/presence"
	"discurd/internal/store"
	"discurd/internal/ws"
)

const connectBudget = 60 * time.Second

func main() {
	cfg, err := config.Load("gateway")
	if err != nil {
		obs.NewLogger("info", "gateway").Error("invalid configuration", "error", err.Error())
		os.Exit(1)
	}
	logger := obs.NewLogger(cfg.LogLevel, cfg.ServiceName)
	metrics := obs.NewMetrics(cfg.ServiceName)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// ---- infra connections (retry with backoff for ~60s) ----
	var session *gocql.Session
	if err := obs.Retry(ctx, logger, "scylla", connectBudget, func() error {
		var err error
		session, err = store.Connect(cfg.ScyllaHosts, cfg.ScyllaKeyspace)
		return err
	}); err != nil {
		logger.Error("scylla unavailable", "error", err.Error())
		os.Exit(1)
	}
	defer session.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()
	if err := obs.Retry(ctx, logger, "redis", connectBudget, func() error {
		return rdb.Ping(ctx).Err()
	}); err != nil {
		logger.Error("redis unavailable", "error", err.Error())
		os.Exit(1)
	}

	var nc *nats.Conn
	if err := obs.Retry(ctx, logger, "nats", connectBudget, func() error {
		var err error
		nc, err = nats.Connect(cfg.NATSURL,
			nats.Name("discurd-gateway"),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)
		return err
	}); err != nil {
		logger.Error("nats unavailable", "error", err.Error())
		os.Exit(1)
	}
	defer nc.Close()

	// ---- hub ----
	hub := ws.NewHub(
		logger,
		metrics,
		auth.NewJWT(cfg.JWTSecret, 0), // TTL only matters for issuing; gateway verifies
		store.NewUsers(session),
		store.NewGuilds(session),
		presence.NewTracker(rdb),
		events.NewPublisher(nc),
	)
	sub, err := hub.Subscribe(nc)
	if err != nil {
		logger.Error("nats subscribe failed", "error", err.Error())
		os.Exit(1)
	}

	// React to NATS connectivity changes. On reconnect, force every client to
	// resync (events published while we were disconnected are gone — core NATS
	// is at-most-once). nats.go auto-resubscribes the existing subscription.
	nc.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		if err != nil {
			logger.Warn("nats disconnected", "error", err.Error())
		}
	})
	nc.SetReconnectHandler(func(c *nats.Conn) {
		logger.Info("nats reconnected, dropping sessions to force resync", "url", c.ConnectedUrl())
		hub.DropAllSessions("nats reconnected")
	})
	nc.SetErrorHandler(func(_ *nats.Conn, s *nats.Subscription, err error) {
		subj := ""
		if s != nil {
			subj = s.Subject
		}
		logger.Error("nats async error (possible slow consumer)", "subject", subj, "error", err.Error())
	})

	// ---- http ----
	r := chi.NewRouter()
	r.Get("/healthz", obs.Healthz)
	r.Get("/readyz", obs.Readyz(
		obs.ReadyCheck{Name: "scylla", Check: func(c context.Context) error { return store.Ping(c, session) }},
		obs.ReadyCheck{Name: "redis", Check: func(c context.Context) error { return rdb.Ping(c).Err() }},
		obs.ReadyCheck{Name: "nats", Check: func(context.Context) error {
			if !nc.IsConnected() {
				return errors.New("nats disconnected")
			}
			return nil
		}},
	))
	r.Method(http.MethodGet, "/metrics", metrics.Handler())
	r.Get("/ws", func(w http.ResponseWriter, req *http.Request) {
		ws.ServeWS(hub, w, req)
	})

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gateway listening", "addr", httpServer.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		logger.Error("http server failed", "error", err.Error())
		os.Exit(1)
	}

	// Graceful drain: stop new events, close sessions, stop the listener.
	_ = sub.Unsubscribe()
	hub.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err.Error())
	}
	logger.Info("gateway stopped")
}
