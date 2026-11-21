package ratelimit

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/middleware"
)

// PostgresRateLimiter enforces fixed-window request quotas using PostgreSQL UPSERT via sqlc.
// Keys authenticated staff by staff ID (preventing club Wi-Fi NAT false positives) and normalizes route patterns.
type PostgresRateLimiter struct {
	queries   db.Querier
	dbTimeout time.Duration
	logger    *slog.Logger
}

func NewPostgresRateLimiter(queries db.Querier, dbTimeout time.Duration, logger *slog.Logger) *PostgresRateLimiter {
	if dbTimeout <= 0 {
		dbTimeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PostgresRateLimiter{
		queries:   queries,
		dbTimeout: dbTimeout,
		logger:    logger,
	}
}

func (l *PostgresRateLimiter) Middleware(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	if maxRequests <= 0 {
		maxRequests = 60
	}
	if window <= 0 {
		window = time.Minute
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identifier := extractIdentifier(r)
			endpoint := r.Method + " " + extractRoutePattern(r)
			windowStart := time.Now().UTC().Truncate(window)

			ctx, cancel := context.WithTimeout(r.Context(), l.dbTimeout)
			defer cancel()

			hits, err := l.queries.UpsertRateLimitHit(ctx, db.UpsertRateLimitHitParams{
				Identifier:  identifier,
				Endpoint:    endpoint,
				WindowStart: windowStart,
			})
			if err != nil {
				l.logger.ErrorContext(r.Context(), "postgres rate limiter upsert failed", "endpoint", endpoint, "error", err)
				next.ServeHTTP(w, r)
				return
			}

			currentHits := int(hits)
			remaining := maxRequests - currentHits
			if remaining < 0 {
				remaining = 0
			}
			resetAt := windowStart.Add(window)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(maxRequests))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if currentHits > maxRequests {
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				_ = httpx.WriteJSON(w, http.StatusTooManyRequests, httpx.ErrorPayload{
					Error: "too many requests; please retry shortly",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// StartCleanup periodically removes expired rate_limits rows in the background until ctx is canceled.
func (l *PostgresRateLimiter) StartCleanup(ctx context.Context, interval, retention time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	if retention <= 0 {
		retention = time.Hour
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pruneCtx, cancel := context.WithTimeout(context.Background(), l.dbTimeout)
				cutoff := time.Now().UTC().Add(-retention)
				if err := l.queries.DeleteExpiredRateLimits(pruneCtx, cutoff); err != nil {
					l.logger.Warn("failed to prune expired rate_limits rows", "error", err)
				}
				cancel()
			}
		}
	}()
}

func extractIdentifier(r *http.Request) string {
	if staffID, ok := middleware.StaffIDFromContext(r.Context()); ok {
		return "staff:" + staffID.String()
	}
	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil && host != "" {
		return "ip:" + host
	}
	if addr := strings.TrimSpace(r.RemoteAddr); addr != "" {
		return "ip:" + addr
	}
	return "ip:unknown"
}

func extractRoutePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return r.URL.Path
}
