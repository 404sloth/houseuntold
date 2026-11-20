package ratelimit

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/venu-live/venu-api/internal/httpx"
)

type Limiter interface {
	Middleware(maxRequests int, window time.Duration) func(http.Handler) http.Handler
}

// RedisRateLimiter uses an atomic Lua INCR + PEXPIRE window in Redis.
type RedisRateLimiter struct {
	client *redis.Client
}

var incrExpireLua = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if tonumber(current) == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return current
`)

func NewRedisRateLimiter(client *redis.Client) *RedisRateLimiter {
	return &RedisRateLimiter{client: client}
}

func (l *RedisRateLimiter) Allow(ctx context.Context, key string, maxRequests int, window time.Duration) (bool, error) {
	count, err := incrExpireLua.Run(ctx, l.client, []string{"rl:" + key}, window.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return count <= maxRequests, nil
}

func (l *RedisRateLimiter) Middleware(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientIP(r) + ":" + r.URL.Path
			allowed, err := l.Allow(r.Context(), key, maxRequests, window)
			if err != nil || !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				_ = httpx.WriteJSON(w, http.StatusTooManyRequests, httpx.ErrorPayload{
					Error: "too many requests, slow down",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MemoryRateLimiter provides local sliding-window rate limiting when Redis is not configured.
type MemoryRateLimiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
}

func NewMemoryRateLimiter() *MemoryRateLimiter {
	return &MemoryRateLimiter{
		buckets: make(map[string][]time.Time),
	}
}

func (l *MemoryRateLimiter) Allow(key string, maxRequests int, window time.Duration) bool {
	now := time.Now()
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	stamps := l.buckets[key]
	filtered := stamps[:0]
	for _, t := range stamps {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) >= maxRequests {
		l.buckets[key] = filtered
		return false
	}
	filtered = append(filtered, now)
	l.buckets[key] = filtered
	return true
}

func (l *MemoryRateLimiter) Middleware(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientIP(r) + ":" + r.URL.Path
			if !l.Allow(key, maxRequests, window) {
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				_ = httpx.WriteJSON(w, http.StatusTooManyRequests, httpx.ErrorPayload{
					Error: "too many requests, slow down",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
