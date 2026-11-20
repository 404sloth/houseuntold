package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
)

type ctxKey string

const (
	AccessCookieName  = "venu_access"
	RefreshCookieName = "venu_refresh"
	userContextKey    = ctxKey("venu_auth_user")
	userIDContextKey  = ctxKey("venu_auth_user_id")
	roleContextKey    = ctxKey("venu_auth_role")
)

func ContextWithAuth(ctx context.Context, user db.User) context.Context {
	ctx = context.WithValue(ctx, userContextKey, user)
	ctx = context.WithValue(ctx, userIDContextKey, user.ID)
	ctx = context.WithValue(ctx, roleContextKey, user.Role)
	return ctx
}

func UserFromContext(ctx context.Context) (db.User, bool) {
	u, ok := ctx.Value(userContextKey).(db.User)
	return u, ok
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return id, ok
}

func RoleFromContext(ctx context.Context) (db.UserRole, bool) {
	r, ok := ctx.Value(roleContextKey).(db.UserRole)
	return r, ok
}

type RBAC struct {
	tokens    *auth.TokenManager
	queries   db.Querier
	dbTimeout time.Duration
}

func NewRBAC(tokens *auth.TokenManager, queries db.Querier, dbTimeout time.Duration) *RBAC {
	if dbTimeout <= 0 {
		dbTimeout = 3 * time.Second
	}
	return &RBAC{tokens: tokens, queries: queries, dbTimeout: dbTimeout}
}

// Authenticate validates the access JWT from HTTP-only cookie or Bearer header,
// performs an authoritative sqlc lookup within DB timeout, and injects UserID & Role into context.
func (rb *RBAC) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := extractAccessToken(r)
		if rawToken == "" {
			_ = httpx.WriteJSON(w, http.StatusUnauthorized, httpx.ErrorPayload{
				Error: "authentication required",
			})
			return
		}

		claims, err := rb.tokens.VerifyAccessToken(rawToken)
		if err != nil {
			_ = httpx.WriteJSON(w, http.StatusUnauthorized, httpx.ErrorPayload{
				Error: "access token invalid or expired",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), rb.dbTimeout)
		defer cancel()

		user, err := rb.queries.GetUserByID(ctx, claims.UserID)
		if err != nil || !user.IsActive {
			_ = httpx.WriteJSON(w, http.StatusUnauthorized, httpx.ErrorPayload{
				Error: "user account unavailable",
			})
			return
		}

		next.ServeHTTP(w, r.WithContext(ContextWithAuth(r.Context(), user)))
	})
}

// RequireRole enforces strict RBAC against the role stored in context on every request.
func (rb *RBAC) RequireRole(allowed ...string) func(http.Handler) http.Handler {
	allowedSet := make(map[db.UserRole]struct{}, len(allowed))
	for _, role := range allowed {
		allowedSet[db.UserRole(role)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := RoleFromContext(r.Context())
			if !ok {
				_ = httpx.WriteJSON(w, http.StatusUnauthorized, httpx.ErrorPayload{
					Error: "authentication required",
				})
				return
			}

			if _, permitted := allowedSet[role]; !permitted {
				_ = httpx.WriteJSON(w, http.StatusForbidden, httpx.ErrorPayload{
					Error: "forbidden: insufficient role permissions",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := originSet[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Razorpay-Signature")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractAccessToken(r *http.Request) string {
	if authHeader := strings.TrimSpace(r.Header.Get("Authorization")); authHeader != "" {
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			return strings.TrimSpace(authHeader[7:])
		}
	}
	if cookie, err := r.Cookie(AccessCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}
