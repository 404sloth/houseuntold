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

	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/cache"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/handlers"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/ratelimit"
	"github.com/venu-live/venu-api/internal/repositories"
	"github.com/venu-live/venu-api/internal/server"
	"github.com/venu-live/venu-api/internal/services"
	"github.com/venu-live/venu-api/internal/store"
	"github.com/venu-live/venu-api/internal/whatsapp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// 1. Load Config (Strictly from environment variables)
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// 2. Context & Graceful Shutdown Signals
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. Connect High-Concurrency PostgreSQL Pool
	pool, err := store.NewPostgresPool(rootCtx, store.PoolConfig{
		DSN:             cfg.DatabaseURL,
		MaxConns:        cfg.DBMaxConns,
		MinConns:        cfg.DBMinConns,
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		MaxConnIdleTime: cfg.DBMaxConnIdleTime,
	})
	if err != nil {
		logger.Error("postgres initialization failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	queries := db.New(pool)

	// 4. Server-Side Staff Role Cache (Never exposed to browser/third-party origins)
	roleCache := cache.NewStaffRoleCache(cfg.StaffCacheTTL)
	roleCache.StartCleanup(rootCtx, time.Minute)

	// 5. PostgreSQL Rate Limiter (Zero external cache dependency)
	limiter := ratelimit.NewPostgresRateLimiter(queries, cfg.DBQueryTimeout, logger)
	limiter.StartCleanup(rootCtx, 10*time.Minute, time.Hour)

	// 6. Meta WhatsApp Cloud API Notifier (Rendered PNG Ticket + Confirmation Caption)
	waNotifier := whatsapp.NewCloudNotifier(
		cfg.WhatsAppAccessToken,
		cfg.WhatsAppPhoneNumberID,
		cfg.WhatsAppAPIVersion,
		cfg.WhatsAppWorkers,
		cfg.WhatsAppHTTPTimeout,
		logger,
	)

	// 7. Auth, Repository, Services & Handlers
	tokenMgr := auth.NewTokenManager(
		cfg.JWTAccessSecret,
		cfg.JWTRefreshSecret,
		cfg.JWTAudience,
		cfg.JWTIssuer,
		cfg.AccessTokenTTL,
		cfg.RefreshTokenTTL,
	)
	qrSigner := auth.NewQRSigner(cfg.QRSigningSecret)
	repo := repositories.NewTicketingRepository(pool, cfg.DBQueryTimeout)

	ticketingSvc := services.NewTicketingService(cfg, repo, tokenMgr, qrSigner, roleCache, waNotifier, logger)
	adminSvc := services.NewAdminService(repo, roleCache, waNotifier, cfg.AppBaseURL, logger)
	gateSvc := services.NewGateService(repo, qrSigner, logger)
	barSvc := services.NewBarService(repo, logger)

	router := server.NewRouter(server.Deps{
		Cfg:       cfg,
		Handler:   handlers.NewTicketingHandler(cfg, ticketingSvc, tokenMgr, logger),
		AdminHdlr: handlers.NewAdminHandler(adminSvc, logger),
		GateHdlr:  handlers.NewGateHandler(gateSvc, logger),
		BarHdlr:   handlers.NewBarHandler(barSvc, logger),
		RBAC:      middleware.NewRBAC(tokenMgr, queries, roleCache, cfg.DBQueryTimeout, cfg.AllowedOrigins),
		Limiter:   limiter,
	})

	// 8. Start HTTP Server
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("Venu Go API listening", "addr", srv.Addr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown signal received, draining server")
	case err := <-serverErr:
		if err != nil {
			logger.Error("http server terminated unexpectedly", "error", err)
		}
	}

	// 9. Graceful Shutdown (HTTP -> WhatsApp workers -> Postgres pool)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server forced shutdown", "error", err)
	}
	if err := waNotifier.CloseAndWait(shutdownCtx); err != nil {
		logger.Warn("whatsapp cloud notifier drain timed out", "error", err)
	}

	logger.Info("Venu Go API shutdown complete")
}