package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/handlers"
	"github.com/venu-live/venu-api/internal/mailer"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/ratelimit"
	"github.com/venu-live/venu-api/internal/repositories"
	"github.com/venu-live/venu-api/internal/services"
	"github.com/venu-live/venu-api/internal/store"
)

type application struct {
	cfg     *config.Config
	handler *handlers.TicketingHandler
	rbac    *middleware.RBAC
	limiter ratelimit.Limiter
	logger  *slog.Logger
}

func (app *application) mount() http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(app.cfg.RequestTimeout))
	r.Use(middleware.CORS(app.cfg.AllowedOrigins))

	r.Route("/api", func(api chi.Router) {
		// Public health & event discovery
		api.Get("/health", app.handler.Health)
		api.Get("/events", app.handler.ListEvents)
		api.Get("/events/{id}", app.handler.GetEventDetail)
		api.Get("/events/{id}/seats", app.handler.GetSeatMap)
		api.Post("/events/{id}/quote", app.handler.QuoteSeats)

		// Public auth, two-step Admin/Staff login & token refresh/logout (rate-limited)
		api.Group(func(authPub chi.Router) {
			authPub.Use(app.limiter.Middleware(app.cfg.AuthRateLimitMax, app.cfg.AuthRateLimitWindow))
			authPub.Post("/auth/google", app.handler.GoogleAuth)
			authPub.Post("/auth/admin/login", app.handler.AdminLogin)
			authPub.Post("/auth/admin/mfa", app.handler.AdminMFAVerify)
			authPub.Post("/auth/refresh", app.handler.RefreshSession)
			authPub.Post("/auth/logout", app.handler.Logout)
		})

		// Razorpay payment webhook (HMAC-SHA256 verified + rate-limited)
		api.With(app.limiter.Middleware(app.cfg.WebhookRateLimitMax, app.cfg.WebhookRateLimitWin)).
			Post("/webhooks/razorpay", app.handler.RazorpayWebhook)

		// Protected routes (JWT + live sqlc user status + strict RBAC)
		api.Group(func(authed chi.Router) {
			authed.Use(app.rbac.Authenticate)

			authed.Get("/auth/me", app.handler.Me)

			// User / Patron routes
			authed.With(app.limiter.Middleware(app.cfg.HoldRateLimitMax, app.cfg.HoldRateLimitWindow)).
				Group(func(checkoutGrp chi.Router) {
					checkoutGrp.Post("/checkout/hold", app.handler.HoldSeats)
					checkoutGrp.Delete("/checkout/hold/{id}", app.handler.ClearSeatHolds)
					checkoutGrp.Post("/checkout/pay", app.handler.CheckoutPay)
				})
			authed.Get("/tickets", app.handler.ListUserTickets)
			authed.Get("/tickets/{id}/qr", app.handler.GetTicketQR)

			// Strict RBAC: Gate Scanner (SCANNER or ADMIN)
			authed.Group(func(scanGrp chi.Router) {
				scanGrp.Use(app.rbac.RequireRole("SCANNER", "ADMIN"))
				scanGrp.Use(app.limiter.Middleware(app.cfg.ScanRateLimitMax, app.cfg.ScanRateLimitWindow))
				scanGrp.Post("/scan/validate", app.handler.ScanValidate)
				scanGrp.Post("/scan/check-in", app.handler.ScanCheckIn)
				scanGrp.Get("/scan/recent", app.handler.ListRecentScans)
			})

			// Strict RBAC: Bartender Counter (BARTENDER or ADMIN)
			authed.Group(func(counterGrp chi.Router) {
				counterGrp.Use(app.rbac.RequireRole("BARTENDER", "ADMIN"))
				counterGrp.Get("/counter/menu", app.handler.ListFnbMenu)
				counterGrp.Post("/counter/items", app.handler.CreateFnbMenuItem)
				counterGrp.Get("/counter/transactions", app.handler.ListFnbTransactions)
				counterGrp.Post("/counter/validate", app.handler.CounterValidate)
				counterGrp.Post("/counter/quote", app.handler.CounterQuote)
				counterGrp.Post("/counter/redeem", app.handler.CounterRedeem)
			})

			// Strict RBAC: Operations Admin Console (ADMIN only)
			authed.Group(func(adminGrp chi.Router) {
				adminGrp.Use(app.rbac.RequireRole("ADMIN"))
				adminGrp.Post("/admin/mfa/enroll", app.handler.AdminMFAEnroll)
				adminGrp.Get("/admin/stats", app.handler.AdminStats)
				adminGrp.Get("/admin/staff", app.handler.ListStaffUsers)
				adminGrp.Post("/admin/staff", app.handler.CreateStaffUser)
				adminGrp.Post("/admin/events", app.handler.CreateEvent)
				adminGrp.Patch("/admin/events/{id}/layout", app.handler.UpdateEventLayout)
				adminGrp.Patch("/admin/events/{id}/status", app.handler.UpdateEventStatus)
				adminGrp.Get("/admin/orders", app.handler.ListRecentOrders)
				adminGrp.Patch("/admin/users/{id}/role", app.handler.UpdateUserRole)
			})
		})
	})

	return r
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPostgresPool(rootCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("postgres initialization failed", "error", err)
		os.Exit(1)
	}

	var redisClient *redis.Client
	var limiter ratelimit.Limiter
	if cfg.RedisURL != "" {
		opt, parseErr := redis.ParseURL(cfg.RedisURL)
		if parseErr != nil {
			logger.Error("invalid REDIS_URL", "error", parseErr)
			pool.Close()
			os.Exit(1)
		}
		redisClient = redis.NewClient(opt)
		if pingErr := redisClient.Ping(rootCtx).Err(); pingErr != nil {
			logger.Error("redis ping failed", "error", pingErr)
			pool.Close()
			os.Exit(1)
		}
		limiter = ratelimit.NewRedisRateLimiter(redisClient)
		logger.Info("using distributed Redis rate limiter")
	} else {
		limiter = ratelimit.NewMemoryRateLimiter()
		logger.Info("using local memory rate limiter")
	}

	passMailer := mailer.NewFirebasePassMailer(
		cfg.FirebaseProjectID,
		cfg.FirebaseServerKey,
		cfg.AppBaseURL,
		2,
		logger,
	)

	queries := db.New(pool)
	tokenMgr := auth.NewTokenManager(cfg.JWTAccessSecret, cfg.QRSigningSecret, cfg.AccessTokenTTL, cfg.MFATokenTTL)
	identityVerifier := auth.NewIdentityVerifier(cfg.GoogleOAuthClientID, cfg.FirebaseProjectID)
	repo := repositories.NewTicketingRepository(pool, cfg.DBQueryTimeout)
	svc := services.NewTicketingService(cfg, repo, tokenMgr, identityVerifier, passMailer, logger)
	handler := handlers.NewTicketingHandler(cfg, svc, tokenMgr, logger)
	rbac := middleware.NewRBAC(tokenMgr, queries, cfg.DBQueryTimeout)

	app := &application{
		cfg:     cfg,
		handler: handler,
		rbac:    rbac,
		limiter: limiter,
		logger:  logger,
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.mount(),
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server forced shutdown", "error", err)
	}
	if err := passMailer.CloseAndWait(shutdownCtx); err != nil {
		logger.Warn("firebase pass mailer drain timed out", "error", err)
	}
	if redisClient != nil {
		_ = redisClient.Close()
	}
	pool.Close()
	logger.Info("Venu Go API shutdown complete")
}
