package handlers

import (
	"log/slog"
	"net/http"

	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/services"
)

type GateHandler struct {
	svc    *services.GateService
	logger *slog.Logger
}

func NewGateHandler(svc *services.GateService, logger *slog.Logger) *GateHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &GateHandler{svc: svc, logger: logger}
}

type qrScanRequest struct {
	QRToken string `json:"qr_token"`
}

func (h *GateHandler) ValidateQR(w http.ResponseWriter, r *http.Request) {
	var req qrScanRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	view, err := h.svc.ValidateQR(r.Context(), req.QRToken)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *GateHandler) CheckInTicket(w http.ResponseWriter, r *http.Request) {
	var req qrScanRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}

	view, err := h.svc.CheckInTicket(r.Context(), req.QRToken)
	if err != nil {
		httpx.HandleError(w, r, h.logger, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, view)
}
