package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
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

func NewTicketingHandler(
	cfg *config.Config,
	svc *services.TicketingService,
	tokens *auth.TokenManager,
	logger *slog.Logger,
) *TicketingHandler {
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

func (h *TicketingHandler) setAuthCookies(w http.ResponseWriter, res services.AuthResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessCookieName,
		Value:    res.AccessToken,
		Path:     "/",
		Domain:   h.cfg.CookieDomain,
		Expires:  res.AccessExpiresAt,
		MaxAge:   int(h.cfg.AccessTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.RefreshCookieName,
		Value:    res.RefreshToken,
		Path:     "/api/auth",
		Domain:   h.cfg.CookieDomain,
		Expires:  time.Now().UTC().Add(h.cfg.RefreshTokenTTL),
		MaxAge:   int(h.cfg.RefreshTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *TicketingHandler) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.AccessCookieName,
		Value:    "",
		Path:     "/",
		Domain:   h.cfg.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.RefreshCookieName,
		Value:    "",
		Path:     "/api/auth",
		Domain:   h.cfg.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *TicketingHandler) Health(w http.ResponseWriter, r *http.Request) {
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"status": "ok",
		"env":    h.cfg.Env,
	})
}

// --- Auth & RBAC Session Endpoints ---

func (h *TicketingHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, user)
}

func (h *TicketingHandler) GoogleAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDToken string `json:"idToken"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	res, err := h.svc.LoginWithGoogle(r.Context(), req.IDToken)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	h.setAuthCookies(w, res)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"user":            res.User,
		"accessExpiresAt": res.AccessExpiresAt,
	})
}

func (h *TicketingHandler) AdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	outcome, err := h.svc.LoginAdminOrStaff(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	if outcome.RequiresMFA {
		_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
			"requires_mfa":        true,
			"mfa_challenge_token": outcome.MFAChallengeToken,
		})
		return
	}

	h.setAuthCookies(w, *outcome.Session)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"requires_mfa":    false,
		"user":            outcome.Session.User,
		"accessExpiresAt": outcome.Session.AccessExpiresAt,
	})
}

func (h *TicketingHandler) AdminMFAVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAChallengeToken string `json:"mfa_challenge_token"`
		TotpCode          string `json:"totp_code"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	res, err := h.svc.VerifyAdminMFA(r.Context(), req.MFAChallengeToken, req.TotpCode)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	h.setAuthCookies(w, res)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"user":            res.User,
		"accessExpiresAt": res.AccessExpiresAt,
	})
}

func (h *TicketingHandler) AdminMFAEnroll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}
	enrollment, err := h.svc.EnrollAdminMFA(r.Context(), userID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, enrollment)
}

func (h *TicketingHandler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.RefreshCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("missing refresh cookie", err))
		return
	}
	res, err := h.svc.RefreshSession(r.Context(), cookie.Value)
	if err != nil {
		h.clearAuthCookies(w)
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	h.setAuthCookies(w, res)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"user":            res.User,
		"accessExpiresAt": res.AccessExpiresAt,
	})
}

func (h *TicketingHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.RefreshCookieName); err == nil && cookie.Value != "" {
		if err := h.svc.Logout(r.Context(), cookie.Value); err != nil {
			httpx.HandleError(w, r, h.logger, err)
			return
		}
	}
	h.clearAuthCookies(w)
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{"loggedOut": true})
}

// --- Public Event Discovery & Seat Maps ---

func (h *TicketingHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	evts, err := h.svc.ListActiveEvents(r.Context())
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, evts)
}

func (h *TicketingHandler) GetEventDetail(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event UUID", err))
		return
	}
	evt, secs, err := h.svc.GetEventDetail(r.Context(), eventID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"event":    evt,
		"sections": secs,
	})
}

func (h *TicketingHandler) GetSeatMap(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event UUID", err))
		return
	}

	var optionalUID *uuid.UUID
	if c, err := r.Cookie(middleware.AccessCookieName); err == nil && c.Value != "" && h.tokens != nil {
		if claims, vErr := h.tokens.VerifyAccessToken(c.Value); vErr == nil {
			optionalUID = &claims.UserID
		}
	}

	payload, err := h.svc.GetSeatMap(r.Context(), eventID, optionalUID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, payload)
}

func (h *TicketingHandler) QuoteSeats(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event UUID", err))
		return
	}
	var req struct {
		SeatIDs []uuid.UUID `json:"seatIds"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	pricing, err := h.svc.QuoteSeats(r.Context(), eventID, req.SeatIDs)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, pricing)
}

// --- Patron Checkout & Tickets (Protected) ---

func (h *TicketingHandler) HoldSeats(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}

	var req struct {
		EventID uuid.UUID   `json:"eventId"`
		SeatID  *uuid.UUID  `json:"seatId"`
		SeatIDs []uuid.UUID `json:"seatIds"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	ids := req.SeatIDs
	if len(ids) == 0 && req.SeatID != nil && *req.SeatID != uuid.Nil {
		ids = []uuid.UUID{*req.SeatID}
	}
	if req.EventID == uuid.Nil || len(ids) == 0 {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("eventId and seatIds are required", nil))
		return
	}

	seatMap, err := h.svc.HoldSeats(r.Context(), req.EventID, ids, userID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, seatMap)
}

func (h *TicketingHandler) ClearSeatHolds(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event UUID", err))
		return
	}
	seatMap, err := h.svc.ClearHolds(r.Context(), eventID, userID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, seatMap)
}

func (h *TicketingHandler) CheckoutPay(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}

	var req struct {
		EventID      uuid.UUID                `json:"eventId"`
		SeatIDs      []uuid.UUID              `json:"seatIds"`
		ContactEmail string                   `json:"contactEmail"`
		ContactPhone string                   `json:"contactPhone"`
		Attendees    []services.AttendeeInput `json:"attendees"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	res, err := h.svc.InitCheckoutPay(
		r.Context(),
		req.EventID,
		userID,
		req.SeatIDs,
		req.ContactEmail,
		req.ContactPhone,
		req.Attendees,
	)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *TicketingHandler) ListUserTickets(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}
	tickets, err := h.svc.ListUserTickets(r.Context(), userID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, tickets)
}

func (h *TicketingHandler) GetTicketQR(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}
	ticketID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid ticket UUID", err))
		return
	}

	rotating := r.URL.Query().Get("mode") == "rotating"
	res, err := h.svc.GenerateTicketQR(r.Context(), ticketID, userID, rotating)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *TicketingHandler) RazorpayWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, httpx.MaxJSONBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("unreadable webhook body", err))
		return
	}

	var payload struct {
		RazorpayOrderID string `json:"razorpay_order_id"`
		Phone           string `json:"phone"`
		EventTitle      string `json:"event_title"`
		GateLabel       string `json:"gate_label"`
		FnBCredit       int32  `json:"fnb_credit"`
	}
	if err := httpx.DecodeStrictBytes(raw, &payload); err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("malformed webhook json", err))
		return
	}

	sig := r.Header.Get("X-Razorpay-Signature")
	ticket, err := h.svc.HandleRazorpayWebhook(
		r.Context(),
		raw,
		sig,
		payload.RazorpayOrderID,
		payload.Phone,
		payload.EventTitle,
		payload.GateLabel,
		payload.FnBCredit,
	)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"status": "confirmed",
		"ticket": ticket,
	})
}

// --- Gate Scanner (RBAC: SCANNER, ADMIN) ---

func (h *TicketingHandler) ScanValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	res, err := h.svc.ValidateScanTicket(r.Context(), req.Query)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *TicketingHandler) ScanCheckIn(w http.ResponseWriter, r *http.Request) {
	scannerID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}
	var req struct {
		TicketID uuid.UUID  `json:"ticketId"`
		SeatID   *uuid.UUID `json:"seatId"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	tkt, attendees, err := h.svc.CheckInTicket(r.Context(), req.TicketID, req.SeatID, scannerID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"ticket":    tkt,
		"attendees": attendees,
	})
}

func (h *TicketingHandler) ListRecentScans(w http.ResponseWriter, r *http.Request) {
	scans, err := h.svc.ListRecentScans(r.Context(), 25)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, scans)
}

// --- Bartender Counter (RBAC: BARTENDER, ADMIN) ---

func (h *TicketingHandler) CounterValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	res, err := h.svc.ValidateCounterTicket(r.Context(), req.Query)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *TicketingHandler) CounterRedeem(w http.ResponseWriter, r *http.Request) {
	bartenderID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}
	var req struct {
		TicketID          uuid.UUID               `json:"ticketId"`
		Items             []services.FnbCartEntry `json:"items"`
		UpiDeltaConfirmed bool                    `json:"upiDeltaConfirmed"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	res, err := h.svc.RedeemCounterBalance(r.Context(), req.TicketID, bartenderID, req.Items, req.UpiDeltaConfirmed)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *TicketingHandler) ListFnbMenu(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListFnbMenu(r.Context())
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *TicketingHandler) CounterQuote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TicketID *uuid.UUID              `json:"ticketId"`
		Items    []services.FnbCartEntry `json:"items"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	res, err := h.svc.QuoteCounterOrder(r.Context(), req.TicketID, req.Items)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *TicketingHandler) CreateFnbMenuItem(w http.ResponseWriter, r *http.Request) {
	var req services.CreateFnbMenuItemInput
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	item, err := h.svc.CreateFnbMenuItem(r.Context(), req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *TicketingHandler) ListFnbTransactions(w http.ResponseWriter, r *http.Request) {
	txs, err := h.svc.ListRecentFnbTransactions(r.Context(), 25)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, txs)
}

// --- Admin Operations (RBAC: ADMIN) ---

func (h *TicketingHandler) AdminStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetAdminStats(r.Context())
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, stats)
}

func (h *TicketingHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req services.CreateEventInput
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	evt, secs, err := h.svc.CreateEvent(r.Context(), req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, httpx.Envelope{
		"event":    evt,
		"sections": secs,
	})
}

func (h *TicketingHandler) UpdateEventLayout(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event UUID", err))
		return
	}
	var req struct {
		LayoutType string          `json:"layoutType"`
		LayoutJSON json.RawMessage `json:"layoutJson"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	evt, err := h.svc.UpdateEventLayout(r.Context(), eventID, req.LayoutType, req.LayoutJSON)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, evt)
}

func (h *TicketingHandler) UpdateEventStatus(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event UUID", err))
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	evt, err := h.svc.UpdateEventStatus(r.Context(), eventID, req.Status)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, evt)
}

func (h *TicketingHandler) ListRecentOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.svc.ListRecentOrders(r.Context(), 50)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, orders)
}

func (h *TicketingHandler) UpdateUserRole(w http.ResponseWriter, r *http.Request) {
	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid user UUID", err))
		return
	}
	var req struct {
		Role db.UserRole `json:"role"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	user, err := h.svc.UpdateUserRole(r.Context(), targetID, req.Role)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, user)
}

func (h *TicketingHandler) ListStaffUsers(w http.ResponseWriter, r *http.Request) {
	staff, err := h.svc.ListStaffUsers(r.Context())
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, staff)
}

func (h *TicketingHandler) CreateStaffUser(w http.ResponseWriter, r *http.Request) {
	var req services.CreateStaffUserInput
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	staff, err := h.svc.CreateStaffUser(r.Context(), req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, staff)
}
