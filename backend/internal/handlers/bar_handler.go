package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/services"
)

type BarHandler struct {
	svc    *services.BarService
	logger *slog.Logger
}

func NewBarHandler(svc *services.BarService, logger *slog.Logger) *BarHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &BarHandler{svc: svc, logger: logger}
}

func (h *BarHandler) GetMenu(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.GetMenu(r.Context())
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{"items": items})
}

func (h *BarHandler) CreateMenuItem(w http.ResponseWriter, r *http.Request) {
	var req services.CreateMenuItemRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	item, err := h.svc.CreateMenuItem(r.Context(), req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *BarHandler) DeleteMenuItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.HandleError(w, r, h.logger, httpx.NewBadRequest("invalid menu item id", err))
		return
	}

	item, err := h.svc.DeleteMenuItem(r.Context(), itemID)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"deleted": true,
		"item":    item,
	})
}

func (h *BarHandler) SearchTicketByBarCode(w http.ResponseWriter, r *http.Request) {
	barCode := strings.TrimSpace(r.URL.Query().Get("bar_code"))
	if barCode == "" {
		barCode = strings.TrimSpace(r.URL.Query().Get("code"))
	}

	view, err := h.svc.SearchTicketByBarCode(r.Context(), barCode)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *BarHandler) GetTicketBarHistory(w http.ResponseWriter, r *http.Request) {
	barCode := strings.TrimSpace(chi.URLParam(r, "bar_code"))
	limit := int32(50)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = int32(n)
		}
	}

	history, err := h.svc.GetTicketBarHistory(r.Context(), barCode, limit)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, httpx.Envelope{
		"bar_code":     strings.ToUpper(barCode),
		"transactions": history,
	})
}

func (h *BarHandler) RedeemItems(w http.ResponseWriter, r *http.Request) {
	bartenderID, ok := middleware.StaffIDFromContext(r.Context())
	if !ok {
		httpx.HandleError(w, r, h.logger, httpx.NewUnauthorized("authentication required", nil))
		return
	}

	var req services.RedeemItemsRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	resp, err := h.svc.RedeemItems(r.Context(), bartenderID, idemKey, req)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}
