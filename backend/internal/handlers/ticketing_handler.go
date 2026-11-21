package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/services"
)

type TicketingHandler struct {
	cfg    *config.Config
	svc    *services.TicketingService
	tokens *auth.TokenManager
	logger *slog.Logger
}

func NewTicketingHandler(cfg *config.Config, svc *services.TicketingService, tokens *auth.TokenManager, logger *slog.Logger) *TicketingHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &TicketingHandler{
		cfg:    cfg,
		svc:    svc,
		tokens: tokens,
		logger: logger,
	}
}

func (h *TicketingHandler) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Health(r.Context()); err != nil {
		h.logger.ErrorContext(r.Context(), "health check failed", "error", err)
		_ = httpx.WriteJSON(w, http.StatusServiceUnavailable, httpx.ErrorPayload{
			Error: "database unavailable",
		})
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{"status": "ok"})
}

func (h *TicketingHandler) GetEventDetails(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event id", err))
		return
	}

	view, err := h.svc.GetEventDetails(r.Context(), eventID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *TicketingHandler) GetTicketByUUID(w http.ResponseWriter, r *http.Request) {
	ticketID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid ticket uuid", err))
		return
	}

	view, err := h.svc.GetTicketByUUID(r.Context(), ticketID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *TicketingHandler) GetTicketImagePNG(w http.ResponseWriter, r *http.Request) {
	ticketID, err := uuid.Parse(chi.URLParam(r, "uuid"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid ticket uuid", err))
		return
	}

	pngBytes, err := h.svc.RenderTicketImagePNG(r.Context(), ticketID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="venu-pass-%s.png"`, ticketID.String()[:8]))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pngBytes)
}

func (h *TicketingHandler) InitGuestCheckout(w http.ResponseWriter, r *http.Request) {
	var req services.InitCheckoutRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	resp, err := h.svc.InitGuestCheckout(r.Context(), req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, resp)
}

type submitUPIReferenceRequest struct {
	UPITransactionID string `json:"upi_transaction_id"`
}

func (h *TicketingHandler) SubmitUPITransactionID(w http.ResponseWriter, r *http.Request) {
	bookingID, err := uuid.Parse(chi.URLParam(r, "booking_id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid booking_id", err))
		return
	}

	var req submitUPIReferenceRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	booking, err := h.svc.SubmitUPITransactionID(r.Context(), bookingID, req.UPITransactionID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"booking_id":         booking.ID,
		"status":             booking.Status,
		"total_amount":       booking.TotalAmount,
		"upi_transaction_id": booking.UpiTransactionID.String,
		"message":            "UPI transaction reference submitted; awaiting admin verification",
	})
}

type staffLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *TicketingHandler) StaffLogin(w http.ResponseWriter, r *http.Request) {
	var req staffLoginRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	session, err := h.svc.StaffLogin(r.Context(), req.Username, req.Password)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	h.setSessionCookies(w, session.Tokens)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"staff":        session.Staff,
		"access_token": session.Tokens.AccessToken,
		"expires_at":   session.Tokens.ExpiresAt,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *TicketingHandler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	var rawRefresh string
	fromCookie := false
	if cookie, err := r.Cookie(middleware.RefreshCookieName); err == nil && cookie.Value != "" {
		rawRefresh = cookie.Value
		fromCookie = true
	} else {
		var req refreshRequest
		if err := httpx.ReadJSON(w, r, &req); err == nil {
			rawRefresh = strings.TrimSpace(req.RefreshToken)
		}
	}

	if fromCookie && !h.isAllowedRefreshOrigin(r) {
		httpx.HandleError(w, r, h.logger, httpx.NewForbidden("cross-site refresh origin rejected"))
		return
	}

	session, err := h.svc.RefreshSession(r.Context(), rawRefresh)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	h.setSessionCookies(w, session.Tokens)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"staff":        session.Staff,
		"access_token": session.Tokens.AccessToken,
		"expires_at":   session.Tokens.ExpiresAt,
	})
}

func (h *TicketingHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, cookieErr := r.Cookie(middleware.AccessCookieName)
	hasCookie := cookieErr == nil && cookie.Value != ""
	if hasCookie && !h.isAllowedRefreshOrigin(r) {
		httpx.HandleError(w, r, h.logger, httpx.NewForbidden("cross-site logout origin rejected"))
		return
	}

	// Best-effort eviction of server-side role cache for this staff session.
	if h.tokens != nil {
		if hasCookie {
			if claims, err := h.tokens.VerifyAccessToken(cookie.Value); err == nil {
				h.svc.LogoutStaff(claims.StaffID)
			}
		} else if authHdr := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(authHdr), "bearer ") {
			if claims, err := h.tokens.VerifyAccessToken(strings.TrimSpace(authHdr[7:])); err == nil {
				h.svc.LogoutStaff(claims.StaffID)
			}
		}
	}

	h.clearSessionCookies(w)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{"status": "signed_out"})
}

func (h *TicketingHandler) isAllowedRefreshOrigin(r *http.Request) bool {
	secFetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if secFetchSite == "cross-site" {
		return false
	}
	if strings.TrimSpace(r.Header.Get("X-Requested-With")) != "" {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return secFetchSite == "same-origin"
	}
	for _, allowed := range h.cfg.AllowedOrigins {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}

func (h *TicketingHandler) setSessionCookies(w http.ResponseWriter, pair auth.TokenPair) {
	h.writeCookie(w, middleware.AccessCookieName, pair.AccessToken, int(h.cfg.AccessTokenTTL.Seconds()))
	h.writeCookie(w, middleware.RefreshCookieName, pair.RefreshToken, int(h.cfg.RefreshTokenTTL.Seconds()))
}

func (h *TicketingHandler) clearSessionCookies(w http.ResponseWriter) {
	h.writeCookie(w, middleware.AccessCookieName, "", -1)
	h.writeCookie(w, middleware.RefreshCookieName, "", -1)
}

// writeCookie issues strictly isolated HttpOnly + SameSite=Strict cookies so no third-party site or JS can read tokens/roles.
func (h *TicketingHandler) writeCookie(w http.ResponseWriter, name, value string, maxAge int) {
	securePart := ""
	if h.cfg.CookieSecure {
		securePart = "; Secure"
	}
	domainPart := ""
	if h.cfg.CookieDomain != "" {
		domainPart = "; Domain=" + h.cfg.CookieDomain
	}
	cookieHeader := fmt.Sprintf("%s=%s; Path=/; Max-Age=%d; HttpOnly; SameSite=Strict%s%s",
		name, value, maxAge, domainPart, securePart)
	w.Header().Add("Set-Cookie", cookieHeader)
}
