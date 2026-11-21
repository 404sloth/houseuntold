package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/cache"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/repositories"
	"github.com/venu-live/venu-api/internal/ticketimg"
	"github.com/venu-live/venu-api/internal/whatsapp"
)

const maxBarCodeCollisionRetries = 10

type TicketingService struct {
	cfg       *config.Config
	repo      *repositories.TicketingRepository
	tokens    *auth.TokenManager
	qrSigner  *auth.QRSigner
	roleCache *cache.StaffRoleCache
	notifier  whatsapp.Notifier
	logger    *slog.Logger
}

func NewTicketingService(
	cfg *config.Config,
	repo *repositories.TicketingRepository,
	tokens *auth.TokenManager,
	qrSigner *auth.QRSigner,
	roleCache *cache.StaffRoleCache,
	notifier whatsapp.Notifier,
	logger *slog.Logger,
) *TicketingService {
	if logger == nil {
		logger = slog.Default()
	}
	return &TicketingService{
		cfg:       cfg,
		repo:      repo,
		tokens:    tokens,
		qrSigner:  qrSigner,
		roleCache: roleCache,
		notifier:  notifier,
		logger:    logger,
	}
}

func (s *TicketingService) Health(ctx context.Context) error {
	return s.repo.Ping(ctx)
}

// --- Staff Authentication (Username / Password + Server-Side Role Cache) ---

type StaffSessionResult struct {
	Staff  StaffSummary   `json:"staff"`
	Tokens auth.TokenPair `json:"-"`
}

type StaffSummary struct {
	ID       uuid.UUID    `json:"id"`
	Username string       `json:"username"`
	Role     db.StaffRole `json:"role"`
}

func (s *TicketingService) StaffLogin(ctx context.Context, username, password string) (StaffSessionResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return StaffSessionResult{}, httpx.NewBadRequest("username and password are required", nil)
	}

	staff, err := s.repo.GetStaffByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StaffSessionResult{}, httpx.NewUnauthorized("invalid username or password", nil)
		}
		return StaffSessionResult{}, fmt.Errorf("svc_staff_login: %w", err)
	}

	if !staff.IsActive {
		return StaffSessionResult{}, httpx.NewUnauthorized("staff account is deactivated", nil)
	}

	valid, err := auth.VerifyPassword(password, staff.PasswordHash)
	if err != nil || !valid {
		return StaffSessionResult{}, httpx.NewUnauthorized("invalid username or password", nil)
	}

	pair, err := s.tokens.IssueStaffTokens(staff)
	if err != nil {
		return StaffSessionResult{}, fmt.Errorf("svc_staff_login_tokens: %w", err)
	}

	if s.roleCache != nil {
		s.roleCache.Set(staff)
	}

	return StaffSessionResult{
		Staff: StaffSummary{
			ID:       staff.ID,
			Username: staff.Username,
			Role:     staff.Role,
		},
		Tokens: pair,
	}, nil
}

func (s *TicketingService) RefreshSession(ctx context.Context, refreshToken string) (StaffSessionResult, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return StaffSessionResult{}, httpx.NewUnauthorized("refresh token is required", nil)
	}

	claims, err := s.tokens.VerifyRefreshToken(refreshToken)
	if err != nil {
		return StaffSessionResult{}, httpx.NewUnauthorized("refresh token invalid or expired", err)
	}

	staff, err := s.repo.GetStaffByID(ctx, claims.StaffID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StaffSessionResult{}, httpx.NewUnauthorized("staff account not found", nil)
		}
		return StaffSessionResult{}, fmt.Errorf("svc_refresh_session: %w", err)
	}
	if !staff.IsActive {
		if s.roleCache != nil {
			s.roleCache.Invalidate(staff.ID)
		}
		return StaffSessionResult{}, httpx.NewUnauthorized("staff account is deactivated", nil)
	}

	pair, err := s.tokens.IssueStaffTokens(staff)
	if err != nil {
		return StaffSessionResult{}, fmt.Errorf("svc_refresh_tokens: %w", err)
	}

	if s.roleCache != nil {
		s.roleCache.Set(staff)
	}

	return StaffSessionResult{
		Staff: StaffSummary{
			ID:       staff.ID,
			Username: staff.Username,
			Role:     staff.Role,
		},
		Tokens: pair,
	}, nil
}

func (s *TicketingService) LogoutStaff(staffID uuid.UUID) {
	if s.roleCache != nil && staffID != uuid.Nil {
		s.roleCache.Invalidate(staffID)
	}
}

// --- Public Event Details & Dynamic First 99 Pricing ---

type EventTierView struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	Category           string    `json:"category,omitempty"`
	BasePrice          int32     `json:"base_price"`
	EffectivePrice     int32     `json:"effective_price"`
	FnbCredit          int32     `json:"fnb_credit"`
	EntryCount         int32     `json:"entry_count"`
	IsGenderStrict     bool      `json:"is_gender_strict"`
	First99Applied     bool      `json:"first_99_applied"`
	FreeSlotsRemaining int64     `json:"free_slots_remaining,omitempty"`
	IncludesFreeShot   bool      `json:"includes_free_shot"`
}

type EventDetailsView struct {
	ID              uuid.UUID       `json:"id"`
	Title           string          `json:"title"`
	StartsAt        time.Time       `json:"starts_at"`
	EndsAt          time.Time       `json:"ends_at"`
	Location        string          `json:"location"`
	Status          db.EventStatus  `json:"status"`
	UPIVPA          string          `json:"upi_vpa"`
	UPIMerchantName string          `json:"upi_merchant_name"`
	UPIQRImageURL   string          `json:"upi_qr_image_url,omitempty"`
	Tiers           []EventTierView `json:"tiers"`
}

func (s *TicketingService) GetEventDetails(ctx context.Context, eventID uuid.UUID) (EventDetailsView, error) {
	event, err := s.repo.GetEventByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EventDetailsView{}, httpx.NewNotFound("event not found")
		}
		return EventDetailsView{}, fmt.Errorf("svc_get_event: %w", err)
	}

	categories, err := s.repo.ListEventCategories(ctx, eventID)
	if err != nil {
		return EventDetailsView{}, fmt.Errorf("svc_list_categories: %w", err)
	}

	confirmedFemales, err := s.repo.CountConfirmedFemaleStagByEvent(ctx, eventID)
	if err != nil {
		return EventDetailsView{}, fmt.Errorf("svc_count_female_stag: %w", err)
	}

	remainingFree := s.cfg.First99FemaleLimit - confirmedFemales
	if remainingFree < 0 {
		remainingFree = 0
	}

	tiers := make([]EventTierView, 0, len(categories))
	for _, cat := range categories {
		pricing := CalculateCheckoutPricing(cat, confirmedFemales, s.cfg.First99FemaleLimit)
		tv := EventTierView{
			ID:               cat.ID,
			Name:             cat.Name,
			BasePrice:        cat.BasePrice,
			EffectivePrice:   pricing.TotalAmount,
			FnbCredit:        pricing.FnbCredit,
			EntryCount:       cat.EntryCount,
			IsGenderStrict:   cat.IsGenderStrict,
			First99Applied:   pricing.First99Applied,
			IncludesFreeShot: pricing.FreeShotPerk,
		}
		if cat.Category.Valid {
			tv.Category = cat.Category.String
		}
		if pricing.First99Applied {
			tv.FreeSlotsRemaining = remainingFree
		}
		tiers = append(tiers, tv)
	}

	return EventDetailsView{
		ID:              event.ID,
		Title:           event.Title,
		StartsAt:        event.StartsAt,
		EndsAt:          event.EndsAt,
		Location:        event.Location,
		Status:          event.Status,
		UPIVPA:          s.cfg.UPIVPA,
		UPIMerchantName: s.cfg.UPIMerchantName,
		UPIQRImageURL:   s.cfg.UPIQRImageURL,
		Tiers:           tiers,
	}, nil
}

// --- Guest Checkout Initialization (Direct UPI QR + UTR Submission, Atomic First-99 Lock) ---

type InitCheckoutRequest struct {
	EventID          uuid.UUID    `json:"event_id"`
	CategoryID       uuid.UUID    `json:"category_id"`
	ContactPhone     string       `json:"contact_phone"`
	ContactEmail     string       `json:"contact_email"`
	UPITransactionID string       `json:"upi_transaction_id,omitempty"`
	Guests           []GuestInput `json:"guests"`
}

type UPIPaymentInstructions struct {
	VPA          string `json:"vpa"`
	MerchantName string `json:"merchant_name"`
	AmountINR    int32  `json:"amount_inr"`
	IntentURI    string `json:"intent_uri"`
	QRImageURL   string `json:"qr_image_url,omitempty"`
}

type InitCheckoutResponse struct {
	BookingID        uuid.UUID               `json:"booking_id"`
	TicketID         uuid.UUID               `json:"ticket_id"`
	Status           db.BookingStatus        `json:"status"`
	TotalAmount      int32                   `json:"total_amount"`
	Currency         string                  `json:"currency"`
	FnbCredit        int32                   `json:"fnb_credit"`
	EntryCount       int32                   `json:"entry_count"`
	First99Applied   bool                    `json:"first_99_applied"`
	UPITransactionID string                  `json:"upi_transaction_id,omitempty"`
	UPIPayment       *UPIPaymentInstructions `json:"upi_payment,omitempty"`
	TicketURL        string                  `json:"ticket_url,omitempty"`
	TicketImageURL   string                  `json:"ticket_image_url,omitempty"`
	BarCode          string                  `json:"bar_code,omitempty"`
	GateQRToken      string                  `json:"gate_qr_token,omitempty"`
}

func (s *TicketingService) InitGuestCheckout(ctx context.Context, req InitCheckoutRequest) (InitCheckoutResponse, error) {
	if req.EventID == uuid.Nil || req.CategoryID == uuid.Nil {
		return InitCheckoutResponse{}, httpx.NewBadRequest("event_id and category_id are required", nil)
	}
	phone := strings.TrimSpace(req.ContactPhone)
	if len(phone) < 10 || len(phone) > 20 {
		return InitCheckoutResponse{}, httpx.NewBadRequest("valid contact_phone is required", nil)
	}
	email := strings.TrimSpace(req.ContactEmail)
	if len(email) > 255 {
		return InitCheckoutResponse{}, httpx.NewBadRequest("contact_email must be at most 255 characters", nil)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return InitCheckoutResponse{}, httpx.NewBadRequest("valid contact_email is required", nil)
	}

	upiTxnID := strings.TrimSpace(req.UPITransactionID)
	if upiTxnID != "" && !auth.IsValidUPITransactionID(upiTxnID) {
		return InitCheckoutResponse{}, httpx.NewBadRequest("upi_transaction_id must be 8-40 alphanumeric characters", nil)
	}

	cat, err := s.repo.GetCategoryByID(ctx, req.CategoryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InitCheckoutResponse{}, httpx.NewNotFound("ticket category not found")
		}
		return InitCheckoutResponse{}, fmt.Errorf("svc_checkout_get_cat: %w", err)
	}
	if cat.EventID != req.EventID {
		return InitCheckoutResponse{}, httpx.NewBadRequest("category does not belong to specified event", nil)
	}
	if err := ValidateCategoryGuests(cat, req.Guests, s.cfg.MinGuestAge); err != nil {
		return InitCheckoutResponse{}, err
	}

	hasPrimary := false
	for _, g := range req.Guests {
		if g.IsPrimaryContact {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		req.Guests[0].IsPrimaryContact = true
	}

	var resp InitCheckoutResponse
	var waMsg *whatsapp.TicketPassMessage

	// Retry on the rare chance a randomly generated 5-char 1D bartender barcode collides under 100+ concurrent checkouts.
	for attempt := 0; attempt < maxBarCodeCollisionRetries; attempt++ {
		waMsg = nil
		err = s.repo.WithTx(ctx, func(txCtx context.Context, q db.Querier) error {
			var event db.Event
			var confirmedFemaleCount int64

			// Only take row-level event lock when STAG_FEMALE (First 99 rule) to maximize throughput for 100+ concurrent buyers.
			if strings.ToUpper(strings.TrimSpace(cat.Name)) == "STAG_FEMALE" {
				event, err = q.GetEventByIDForUpdate(txCtx, req.EventID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return httpx.NewNotFound("event not found")
					}
					return fmt.Errorf("tx_lock_event: %w", err)
				}
				confirmedFemaleCount, err = q.CountConfirmedFemaleStagByEvent(txCtx, req.EventID)
				if err != nil {
					return fmt.Errorf("tx_count_female_stag: %w", err)
				}
			} else {
				event, err = q.GetEventByID(txCtx, req.EventID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						return httpx.NewNotFound("event not found")
					}
					return fmt.Errorf("tx_get_event: %w", err)
				}
			}

			if event.Status != db.EventStatusONSALE {
				return httpx.NewConflict("event is not currently on sale", nil)
			}
			if time.Now().UTC().After(event.EndsAt) {
				return httpx.NewConflict("event has already ended", nil)
			}

			pricing := CalculateCheckoutPricing(cat, confirmedFemaleCount, s.cfg.First99FemaleLimit)
			bookingStatus := db.BookingStatusPENDING
			upiText := pgtype.Text{Valid: false}
			if pricing.TotalAmount == 0 {
				bookingStatus = db.BookingStatusCONFIRMED
			} else if upiTxnID != "" {
				upiText = pgtype.Text{String: upiTxnID, Valid: true}
			}

			ticketID := uuid.New()
			barCode, err := auth.GenerateBarCode()
			if err != nil {
				return fmt.Errorf("tx_generate_barcode: %w", err)
			}
			digest := s.qrSigner.ComputeDigest(ticketID)

			booking, err := q.CreateBooking(txCtx, db.CreateBookingParams{
				EventID:          event.ID,
				CategoryID:       cat.ID,
				ContactPhone:     phone,
				ContactEmail:     email,
				TotalAmount:      pricing.TotalAmount,
				UpiTransactionID: upiText,
				Status:           bookingStatus,
			})
			if err != nil {
				return fmt.Errorf("tx_create_booking: %w", err)
			}

			ticket, err := q.CreateTicket(txCtx, db.CreateTicketParams{
				ID:                 ticketID,
				BookingID:          booking.ID,
				EventID:            event.ID,
				QrTokenDigest:      digest,
				BarCode:            barCode,
				FoodBalance:        pricing.FnbCredit,
				InitialFoodBalance: pricing.FnbCredit,
			})
			if err != nil {
				return fmt.Errorf("tx_create_ticket: %w", err)
			}

			primaryName := strings.TrimSpace(req.Guests[0].Name)
			for _, g := range req.Guests {
				name := strings.TrimSpace(g.Name)
				if g.IsPrimaryContact {
					primaryName = name
				}
				if _, err := q.CreateTicketGuest(txCtx, db.CreateTicketGuestParams{
					TicketID:         ticket.ID,
					Name:             name,
					Age:              g.Age,
					Gender:           g.Gender,
					IsPrimaryContact: g.IsPrimaryContact,
				}); err != nil {
					return fmt.Errorf("tx_create_guest: %w", err)
				}
			}

			ticketURL := fmt.Sprintf("%s/tickets/%s", s.cfg.AppBaseURL, ticket.ID.String())
			ticketImgURL := fmt.Sprintf("%s/api/tickets/%s/image", s.cfg.AppBaseURL, ticket.ID.String())

			resp = InitCheckoutResponse{
				BookingID:        booking.ID,
				TicketID:         ticket.ID,
				Status:           bookingStatus,
				TotalAmount:      pricing.TotalAmount,
				Currency:         "INR",
				FnbCredit:        pricing.FnbCredit,
				EntryCount:       cat.EntryCount,
				First99Applied:   pricing.First99Applied,
				UPITransactionID: upiTxnID,
			}

			if bookingStatus == db.BookingStatusCONFIRMED {
				qrExp := event.EndsAt.Add(s.cfg.TicketQRTTL)
				resp.TicketURL = ticketURL
				resp.TicketImageURL = ticketImgURL
				resp.BarCode = ticket.BarCode
				resp.GateQRToken = s.qrSigner.SignToken(ticket.ID, event.ID, time.Now().UTC(), qrExp)
				waMsg = &whatsapp.TicketPassMessage{
					TicketID:       ticket.ID.String(),
					RecipientPhone: phone,
					PrimaryGuest:   primaryName,
					EventTitle:     event.Title,
					EventLocation:  event.Location,
					EventStartsAt:  event.StartsAt,
					CategoryName:   cat.Name,
					TicketURL:      ticketURL,
					BarCode:        ticket.BarCode,
					EntryCount:     cat.EntryCount,
					FoodBalance:    ticket.FoodBalance,
				}
			} else {
				note := fmt.Sprintf("Venu %s", booking.ID.String()[:8])
				resp.UPIPayment = &UPIPaymentInstructions{
					VPA:          s.cfg.UPIVPA,
					MerchantName: s.cfg.UPIMerchantName,
					AmountINR:    pricing.TotalAmount,
					IntentURI:    auth.BuildUPIIntentURI(s.cfg.UPIVPA, s.cfg.UPIMerchantName, note, pricing.TotalAmount),
					QRImageURL:   s.cfg.UPIQRImageURL,
				}
			}
			return nil
		})

		if err == nil {
			break
		}
		if isBarCodeUniqueViolation(err) {
			continue
		}
		if isUPITxnUniqueViolation(err) {
			return InitCheckoutResponse{}, httpx.NewConflict("this UPI transaction ID has already been submitted", err)
		}
		return InitCheckoutResponse{}, err
	}
	if err != nil {
		return InitCheckoutResponse{}, fmt.Errorf("svc_checkout_exhausted_barcode_retries: %w", err)
	}

	if waMsg != nil && s.notifier != nil {
		s.notifier.EnqueueTicketConfirmation(*waMsg)
	}
	return resp, nil
}

// SubmitUPITransactionID records the patron's UPI UTR/transaction ID on a pending booking for admin verification.
func (s *TicketingService) SubmitUPITransactionID(ctx context.Context, bookingID uuid.UUID, upiTransactionID string) (db.Booking, error) {
	txnID := strings.TrimSpace(upiTransactionID)
	if !auth.IsValidUPITransactionID(txnID) {
		return db.Booking{}, httpx.NewBadRequest("upi_transaction_id must be 8-40 alphanumeric characters", nil)
	}

	booking, err := s.repo.SubmitBookingUPITransactionID(ctx, db.SubmitBookingUPITransactionIDParams{
		ID: bookingID,
		UpiTransactionID: pgtype.Text{
			String: txnID,
			Valid:  true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Booking{}, httpx.NewConflict("booking not found or is no longer pending verification", nil)
		}
		if isUPITxnUniqueViolation(err) {
			return db.Booking{}, httpx.NewConflict("this UPI transaction ID has already been submitted for another booking", err)
		}
		return db.Booking{}, fmt.Errorf("svc_submit_upi_txn: %w", err)
	}
	return booking, nil
}

// --- Public Ticket Pass Lookup & PNG Ticket Image Rendering ---

type GuestView struct {
	ID               uuid.UUID     `json:"id"`
	Name             string        `json:"name"`
	Age              int32         `json:"age"`
	Gender           db.GenderType `json:"gender"`
	IsPrimaryContact bool          `json:"is_primary_contact"`
	FreeShotRedeemed bool          `json:"free_shot_redeemed"`
}

type PublicTicketView struct {
	ID                 uuid.UUID        `json:"id"`
	BookingID          uuid.UUID        `json:"booking_id"`
	EventID            uuid.UUID        `json:"event_id"`
	BookingStatus      db.BookingStatus `json:"booking_status"`
	EventTitle         string           `json:"event_title"`
	EventStartsAt      time.Time        `json:"event_starts_at"`
	EventEndsAt        time.Time        `json:"event_ends_at"`
	EventLocation      string           `json:"event_location"`
	CategoryName       string           `json:"category_name"`
	CategoryTier       string           `json:"category_tier,omitempty"`
	EntryCount         int32            `json:"entry_count"`
	TotalAmount        int32            `json:"total_amount"`
	BarCode            string           `json:"bar_code,omitempty"`       // 5-char 1D linear bartender barcode (multi-use until event end)
	BarCodeFormat      string           `json:"bar_code_format,omitempty"`
	GateQRToken        string           `json:"gate_qr_token,omitempty"`  // Single-use gate QR (immediately expires & hidden once scanned)
	TicketImageURL     string           `json:"ticket_image_url,omitempty"`
	FoodBalance        int32            `json:"food_balance"`
	InitialFoodBalance int32            `json:"initial_food_balance"`
	FnBExpired         bool             `json:"fnb_expired"`
	IsCheckedIn        bool             `json:"is_checked_in"`
	CheckedInAt        *time.Time       `json:"checked_in_at,omitempty"`
	Guests             []GuestView      `json:"guests"`
}

func (s *TicketingService) GetTicketByUUID(ctx context.Context, ticketID uuid.UUID) (PublicTicketView, error) {
	details, err := s.repo.GetTicketDetailsByID(ctx, ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PublicTicketView{}, httpx.NewNotFound("ticket not found")
		}
		return PublicTicketView{}, fmt.Errorf("svc_get_ticket_by_uuid: %w", err)
	}

	guests, err := s.repo.ListGuestsByTicketID(ctx, ticketID)
	if err != nil {
		return PublicTicketView{}, fmt.Errorf("svc_get_ticket_guests: %w", err)
	}

	guestViews := make([]GuestView, 0, len(guests))
	for _, g := range guests {
		guestViews = append(guestViews, GuestView{
			ID:               g.ID,
			Name:             g.Name,
			Age:              g.Age,
			Gender:           g.Gender,
			IsPrimaryContact: g.IsPrimaryContact,
			FreeShotRedeemed: g.FreeShotRedeemed,
		})
	}

	now := time.Now().UTC()
	view := PublicTicketView{
		ID:                 details.ID,
		BookingID:          details.BookingID,
		EventID:            details.EventID,
		BookingStatus:      details.BookingStatus,
		EventTitle:         details.EventTitle,
		EventStartsAt:      details.EventStartsAt,
		EventEndsAt:        details.EventEndsAt,
		EventLocation:      details.EventLocation,
		CategoryName:       details.CategoryName,
		EntryCount:         details.EntryCount,
		TotalAmount:        details.TotalAmount,
		FoodBalance:        details.FoodBalance,
		InitialFoodBalance: details.InitialFoodBalance,
		FnBExpired:         now.After(details.EventEndsAt),
		IsCheckedIn:        details.IsCheckedIn,
		Guests:             guestViews,
	}
	if details.CategoryTier.Valid {
		view.CategoryTier = details.CategoryTier.String
	}
	if details.CheckedInAt.Valid {
		t := details.CheckedInAt.Time
		view.CheckedInAt = &t
	}

	// Only expose barcodes & ticket image once Admin has verified payment (CONFIRMED).
	// Gate QR is omitted immediately after gate check-in; 1D bartender barcode remains until event end.
	if details.BookingStatus == db.BookingStatusCONFIRMED {
		view.BarCode = details.BarCode
		view.BarCodeFormat = "CODE39_1D"
		view.TicketImageURL = fmt.Sprintf("%s/api/tickets/%s/image", s.cfg.AppBaseURL, details.ID.String())
		if !details.IsCheckedIn {
			qrExp := details.EventEndsAt.Add(s.cfg.TicketQRTTL)
			view.GateQRToken = s.qrSigner.SignToken(details.ID, details.EventID, now, qrExp)
		}
	}

	return view, nil
}

func (s *TicketingService) RenderTicketImagePNG(ctx context.Context, ticketID uuid.UUID) ([]byte, error) {
	details, err := s.repo.GetTicketDetailsByID(ctx, ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.NewNotFound("ticket not found")
		}
		return nil, fmt.Errorf("svc_render_ticket_img_lookup: %w", err)
	}
	if details.BookingStatus != db.BookingStatusCONFIRMED {
		return nil, httpx.NewConflict("ticket pass image is available only after payment verification", nil)
	}

	guests, err := s.repo.ListGuestsByTicketID(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("svc_render_ticket_img_guests: %w", err)
	}
	primaryName := PrimaryGuestName(guests)

	return ticketimg.RenderTicketPNG(ticketimg.TicketImageData{
		TicketID:      details.ID.String(),
		EventTitle:    details.EventTitle,
		EventLocation: details.EventLocation,
		EventStartsAt: details.EventStartsAt,
		CategoryName:  details.CategoryName,
		PrimaryGuest:  primaryName,
		EntryCount:    details.EntryCount,
		FoodBalance:   details.FoodBalance,
		BarCode:       details.BarCode,
		GateCheckedIn: details.IsCheckedIn,
	})
}

func isBarCodeUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return strings.Contains(pgErr.ConstraintName, "bar_code")
	}
	return false
}

func isUPITxnUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return strings.Contains(pgErr.ConstraintName, "upi_txn") || strings.Contains(pgErr.ConstraintName, "upi_transaction_id")
	}
	return false
}
