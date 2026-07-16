// Command api is the Discurd REST service (docs/ARCHITECTURE.md §7).
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"discurd/internal/auth"
	"discurd/internal/config"
	"discurd/internal/events"
	"discurd/internal/httpapi"
	"discurd/internal/objstore"
	"discurd/internal/obs"
	"discurd/internal/presence"
	"discurd/internal/ratelimit"
	"discurd/internal/store"
)

const connectBudget = 60 * time.Second

func main() {
	cfg, err := config.Load("api")
	if err != nil {
		obs.NewLogger("info", "api").Error("invalid configuration", "error", err.Error())
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
			nats.Name("discurd-api"),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
		)
		return err
	}); err != nil {
		logger.Error("nats unavailable", "error", err.Error())
		os.Exit(1)
	}
	defer nc.Drain() //nolint:errcheck

	objects, err := objstore.New(cfg.MinioEndpoint, cfg.MinioUser, cfg.MinioPassword, cfg.MinioUseSSL)
	if err != nil {
		logger.Error("minio client", "error", err.Error())
		os.Exit(1)
	}
	if err := obs.Retry(ctx, logger, "minio", connectBudget, func() error {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return objects.Ping(pingCtx)
	}); err != nil {
		logger.Error("minio unavailable", "error", err.Error())
		os.Exit(1)
	}

	// ---- wiring ----
	users := store.NewUsers(session)
	server := httpapi.NewServer(httpapi.Deps{
		Cfg:       cfg,
		Logger:    logger,
		Metrics:   metrics,
		JWT:       auth.NewJWT(cfg.JWTSecret, cfg.AccessTokenTTL),
		Refresh:   auth.NewRefreshStore(rdb, cfg.RefreshTokenTTL),
		Limiter:   ratelimit.NewLimiter(rdb),
		Users:     users,
		Guilds:    store.NewGuilds(session),
		Channels:  store.NewChannels(session),
		Messages:  store.NewMessages(session),
		Invites:   store.NewInvites(session),
		UserCache: store.NewUserCache(users, 30*time.Second),
		Presence:  presence.NewTracker(rdb),
		Publisher: events.NewPublisher(nc),
		Objects:   objects,
		Readyz: obs.Readyz(
			obs.ReadyCheck{Name: "scylla", Check: func(c context.Context) error { return store.Ping(c, session) }},
			obs.ReadyCheck{Name: "redis", Check: func(c context.Context) error { return rdb.Ping(c).Err() }},
			obs.ReadyCheck{Name: "nats", Check: func(context.Context) error {
				if !nc.IsConnected() {
					return errors.New("nats disconnected")
				}
				return nil
			}},
		),
	})

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", httpServer.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		logger.Error("http server failed", "error", err.Error())
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err.Error())
	}
	logger.Info("api stopped")
}
