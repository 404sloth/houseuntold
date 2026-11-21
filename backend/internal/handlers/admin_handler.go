package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/services"
)

type AdminHandler struct {
	svc    *services.AdminService
	logger *slog.Logger
}

func NewAdminHandler(svc *services.AdminService, logger *slog.Logger) *AdminHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminHandler{svc: svc, logger: logger}
}

func (h *AdminHandler) CreateStaff(w http.ResponseWriter, r *http.Request) {
	var req services.CreateStaffRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	staff, err := h.svc.CreateStaff(r.Context(), req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, staff)
}

type updatePasswordRequest struct {
	Password string `json:"password"`
}

func (h *AdminHandler) UpdateStaffPassword(w http.ResponseWriter, r *http.Request) {
	staffID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid staff id", err))
		return
	}

	var req updatePasswordRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	staff, err := h.svc.UpdateStaffPassword(r.Context(), staffID, req.Password)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, staff)
}

func (h *AdminHandler) DeactivateStaff(w http.ResponseWriter, r *http.Request) {
	staffID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid staff id", err))
		return
	}

	actorID, ok := middleware.StaffIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}

	staff, err := h.svc.DeactivateStaff(r.Context(), staffID, actorID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, staff)
}

func (h *AdminHandler) ListBookingsForVerification(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := db.BookingStatus(strings.ToUpper(strings.TrimSpace(q.Get("status"))))

	var eventID uuid.UUID
	if rawEv := strings.TrimSpace(q.Get("event_id")); rawEv != "" {
		parsed, err := uuid.Parse(rawEv)
		if err != nil {
			httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event_id query parameter", err))
			return
		}
		eventID = parsed
	}

	limit := int32(50)
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		if n, err := strconv.Atoi(rawLimit); err == nil {
			limit = int32(n)
		}
	}
	offset := int32(0)
	if rawOffset := strings.TrimSpace(q.Get("offset")); rawOffset != "" {
		if n, err := strconv.Atoi(rawOffset); err == nil {
			offset = int32(n)
		}
	}

	items, err := h.svc.ListBookingsForVerification(r.Context(), status, eventID, limit, offset)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"bookings": items,
		"count":    len(items),
	})
}

func (h *AdminHandler) VerifyBookingPayment(w http.ResponseWriter, r *http.Request) {
	bookingID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid booking id", err))
		return
	}

	adminID, ok := middleware.StaffIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}

	var req services.VerifyBookingRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	resp, err := h.svc.VerifyBookingPayment(r.Context(), bookingID, adminID, req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *AdminHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req services.CreateEventRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	ev, err := h.svc.CreateEvent(r.Context(), req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, ev)
}

func (h *AdminHandler) CreateTicketTier(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event id", err))
		return
	}

	var req services.CreateTicketTierRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	tier, err := h.svc.CreateTicketTier(r.Context(), eventID, req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, tier)
}

func (h *AdminHandler) GetEventStats(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "event_id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid event_id", err))
		return
	}

	stats, err := h.svc.GetEventStats(r.Context(), eventID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, stats)
}
