package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/repositories"
	"github.com/venu-live/venu-api/internal/services"
)

type handlerMockStore struct {
	repositories.TicketingStore
	listEventsErr error
	statsRow      db.GetAdminOverviewStatsRow
}

func (m *handlerMockStore) ListActiveEvents(ctx context.Context) ([]db.Event, error) {
	if m.listEventsErr != nil {
		return nil, m.listEventsErr
	}
	return []db.Event{}, nil
}

func (m *handlerMockStore) GetAdminOverviewStats(ctx context.Context) (db.GetAdminOverviewStatsRow, error) {
	return m.statsRow, nil
}

func injectRole(role db.UserRole) func(http.Handler) http.Handler {
	u := db.User{
		ID:       uuid.New(),
		Email:    "actor@venu.live",
		Role:     role,
		IsActive: true,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(middleware.ContextWithAuth(r.Context(), u)))
		})
	}
}

func TestStrictRBACMiddlewareAndErrorJSON(t *testing.T) {
	cfg := &config.Config{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		DBQueryTimeout:  3 * time.Second,
	}
	tokens := auth.NewTokenManager("test-access", "test-qr", cfg.AccessTokenTTL, 5*time.Minute)
	rbac := middleware.NewRBAC(tokens, nil, cfg.DBQueryTimeout)
	store := &handlerMockStore{statsRow: db.GetAdminOverviewStatsRow{ActiveEventsCount: 3}}
	svc := services.NewTicketingService(cfg, store, tokens, nil, nil, nil)
	h := NewTicketingHandler(cfg, svc, tokens, nil)

	tests := []struct {
		name       string
		method     string
		path       string
		callerRole *db.UserRole
		wantStatus int
	}{
		{
			name:       "unauthenticated user blocked from admin stats with 401",
			method:     http.MethodGet,
			path:       "/api/admin/stats",
			callerRole: nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "patron USER rejected from /api/admin/stats with 403",
			method:     http.MethodGet,
			path:       "/api/admin/stats",
			callerRole: ptrRole(db.UserRoleUSER),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "BARTENDER rejected from /api/scan/validate with 403",
			method:     http.MethodPost,
			path:       "/api/scan/validate",
			callerRole: ptrRole(db.UserRoleBARTENDER),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "SCANNER rejected from /api/counter/redeem with 403",
			method:     http.MethodPost,
			path:       "/api/counter/redeem",
			callerRole: ptrRole(db.UserRoleSCANNER),
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "ADMIN permitted on /api/admin/stats with 200",
			method:     http.MethodGet,
			path:       "/api/admin/stats",
			callerRole: ptrRole(db.UserRoleADMIN),
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			if tc.callerRole != nil {
				r.Use(injectRole(*tc.callerRole))
			}
			r.With(rbac.RequireRole("ADMIN")).Get("/api/admin/stats", h.AdminStats)
			r.With(rbac.RequireRole("SCANNER", "ADMIN")).Post("/api/scan/validate", h.ScanValidate)
			r.With(rbac.RequireRole("BARTENDER", "ADMIN")).Post("/api/counter/redeem", h.CounterRedeem)

			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(`{}`))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("got status %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if tc.wantStatus >= 400 {
				var payload map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload["error"] == "" {
					t.Fatalf("expected {\"error\": \"message\"} JSON body, got %s", rec.Body.String())
				}
			}
		})
	}
}

func TestTimeoutMapsToGatewayTimeout504(t *testing.T) {
	cfg := &config.Config{}
	store := &handlerMockStore{
		listEventsErr: errors.Join(errors.New("repo_list_active_events"), context.DeadlineExceeded),
	}
	svc := services.NewTicketingService(cfg, store, nil, nil, nil, nil)
	h := NewTicketingHandler(cfg, svc, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	rec := httptest.NewRecorder()
	h.ListEvents(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 Gateway Timeout on context.DeadlineExceeded, got %d (%s)", rec.Code, rec.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || payload["error"] == "" {
		t.Fatalf("expected {\"error\": \"...\"}, got %s", rec.Body.String())
	}
}

func ptrRole(r db.UserRole) *db.UserRole {
	return &r
}
