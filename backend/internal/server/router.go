package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/handlers"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/ratelimit"
)

// Deps groups the wired HTTP dependencies mounted on the Venu Chi router.
type Deps struct {
	Cfg       *config.Config
	Handler   *handlers.TicketingHandler
	AdminHdlr *handlers.AdminHandler
	GateHdlr  *handlers.GateHandler
	BarHdlr   *handlers.BarHandler
	RBAC      *middleware.RBAC
	Limiter   ratelimit.Limiter
}

// NewRouter mounts all 25 Venu API routes with security headers, CORS, Postgres rate limiting, and per-request RBAC.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(d.Cfg.RequestTimeout))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(d.Cfg.AllowedOrigins))

	r.Route("/api", func(api chi.Router) {
		// 1. PUBLIC / GUEST ROUTES (Direct UPI QR + UTR Submission)
		api.Get("/health", d.Handler.Health)
		api.Get("/events/{id}", d.Handler.GetEventDetails)
		api.Get("/tickets/{uuid}", d.Handler.GetTicketByUUID)
		api.Get("/tickets/{uuid}/image", d.Handler.GetTicketImagePNG)

		// Guest Checkout & UPI UTR Submission (PostgreSQL rate limited)
		api.Group(func(checkoutPub chi.Router) {
			checkoutPub.Use(d.Limiter.Middleware(d.Cfg.CheckoutRateLimitMax, d.Cfg.CheckoutRateLimitWindow))
			checkoutPub.Post("/checkout/init", d.Handler.InitGuestCheckout)
			checkoutPub.Post("/checkout/{booking_id}/upi-reference", d.Handler.SubmitUPITransactionID)
		})

		// Staff Auth (ADMIN, SCANNER, BARTENDER — username/password, HttpOnly SameSite=Strict cookies)
		api.Group(func(authPub chi.Router) {
			authPub.Use(d.Limiter.Middleware(d.Cfg.AuthRateLimitMax, d.Cfg.AuthRateLimitWindow))
			authPub.Post("/auth/login", d.Handler.StaffLogin)
			authPub.Post("/auth/refresh", d.Handler.RefreshSession)
			authPub.Post("/auth/logout", d.Handler.Logout)
		})

		// 2. PROTECTED STAFF ROUTES (JWT + Server-Side StaffRoleCache on every request)
		api.Group(func(authed chi.Router) {
			authed.Use(d.RBAC.Authenticate)

			// --- GATE SCANNER (SCANNER or ADMIN) ---
			authed.Group(func(scanGrp chi.Router) {
				scanGrp.Use(d.RBAC.RequireRole("SCANNER", "ADMIN"))
				scanGrp.Use(d.Limiter.Middleware(d.Cfg.ScanRateLimitMax, d.Cfg.ScanRateLimitWindow))

				scanGrp.Post("/gate/validate", d.GateHdlr.ValidateQR)
				scanGrp.Post("/gate/check-in", d.GateHdlr.CheckInTicket)
			})

			// --- BARTENDER POS (BARTENDER or ADMIN) ---
			authed.Group(func(barGrp chi.Router) {
				barGrp.Use(d.RBAC.RequireRole("BARTENDER", "ADMIN"))
				barGrp.Use(d.Limiter.Middleware(d.Cfg.BarRateLimitMax, d.Cfg.BarRateLimitWindow))

				barGrp.Get("/counter/menu", d.BarHdlr.GetMenu)
				barGrp.Post("/counter/menu", d.BarHdlr.CreateMenuItem)
				barGrp.Delete("/counter/menu/{id}", d.BarHdlr.DeleteMenuItem)
				barGrp.Get("/counter/search", d.BarHdlr.SearchTicketByBarCode)
				barGrp.Get("/counter/tickets/{bar_code}/history", d.BarHdlr.GetTicketBarHistory)
				barGrp.Post("/counter/redeem", d.BarHdlr.RedeemItems)
			})

			// --- ADMIN OPERATIONS (ADMIN only) ---
			authed.Group(func(adminGrp chi.Router) {
				adminGrp.Use(d.RBAC.RequireRole("ADMIN"))

				adminGrp.Post("/admin/staff", d.AdminHdlr.CreateStaff)
				adminGrp.Patch("/admin/staff/{id}/password", d.AdminHdlr.UpdateStaffPassword)
				adminGrp.Delete("/admin/staff/{id}", d.AdminHdlr.DeactivateStaff)

				adminGrp.Get("/admin/bookings", d.AdminHdlr.ListBookingsForVerification)
				adminGrp.Post("/admin/bookings/{id}/verify", d.AdminHdlr.VerifyBookingPayment)

				adminGrp.Post("/admin/events", d.AdminHdlr.CreateEvent)
				adminGrp.Post("/admin/events/{id}/tiers", d.AdminHdlr.CreateTicketTier)

				adminGrp.Get("/admin/stats/{event_id}", d.AdminHdlr.GetEventStats)
			})
		})
	})

	return r
}
