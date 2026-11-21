package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/cache"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/repositories"
	"github.com/venu-live/venu-api/internal/whatsapp"
)

type AdminService struct {
	repo       *repositories.TicketingRepository
	roleCache  *cache.StaffRoleCache
	notifier   whatsapp.Notifier
	appBaseURL string
	logger     *slog.Logger
}

func NewAdminService(
	repo *repositories.TicketingRepository,
	roleCache *cache.StaffRoleCache,
	notifier whatsapp.Notifier,
	appBaseURL string,
	logger *slog.Logger,
) *AdminService {
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminService{
		repo:       repo,
		roleCache:  roleCache,
		notifier:   notifier,
		appBaseURL: strings.TrimRight(appBaseURL, "/"),
		logger:     logger,
	}
}

type CreateStaffRequest struct {
	Username string       `json:"username"`
	Password string       `json:"password"`
	Role     db.StaffRole `json:"role"`
}

type StaffAccountView struct {
	ID        uuid.UUID    `json:"id"`
	Username  string       `json:"username"`
	Role      db.StaffRole `json:"role"`
	IsActive  bool         `json:"is_active"`
	CreatedAt time.Time    `json:"created_at,omitempty"`
}

func (s *AdminService) CreateStaff(ctx context.Context, req CreateStaffRequest) (StaffAccountView, error) {
	username := strings.TrimSpace(req.Username)
	if len(username) < 3 || len(username) > 100 {
		return StaffAccountView{}, httpx.NewBadRequest("username must be between 3 and 100 characters", nil)
	}
	if !isValidStaffRole(req.Role) {
		return StaffAccountView{}, httpx.NewBadRequest("role must be ADMIN, SCANNER, or BARTENDER", nil)
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return StaffAccountView{}, httpx.NewBadRequest(err.Error(), err)
	}

	staff, err := s.repo.CreateStaff(ctx, db.CreateStaffParams{
		Username:     username,
		PasswordHash: hash,
		Role:         req.Role,
	})
	if err != nil {
		return StaffAccountView{}, fmt.Errorf("svc_admin_create_staff: %w", err)
	}

	return StaffAccountView{
		ID:        staff.ID,
		Username:  staff.Username,
		Role:      staff.Role,
		IsActive:  staff.IsActive,
		CreatedAt: staff.CreatedAt,
	}, nil
}

func (s *AdminService) UpdateStaffPassword(ctx context.Context, staffID uuid.UUID, newPassword string) (StaffAccountView, error) {
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return StaffAccountView{}, httpx.NewBadRequest(err.Error(), err)
	}

	row, err := s.repo.UpdateStaffPassword(ctx, db.UpdateStaffPasswordParams{
		ID:           staffID,
		PasswordHash: hash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StaffAccountView{}, httpx.NewNotFound("active staff account not found")
		}
		return StaffAccountView{}, fmt.Errorf("svc_admin_update_staff_password: %w", err)
	}

	// Evict server-side role cache immediately so password rotation takes effect right away.
	if s.roleCache != nil {
		s.roleCache.Invalidate(staffID)
	}

	return StaffAccountView{
		ID:        row.ID,
		Username:  row.Username,
		Role:      row.Role,
		IsActive:  row.IsActive,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *AdminService) DeactivateStaff(ctx context.Context, staffID, actorID uuid.UUID) (StaffAccountView, error) {
	if staffID == actorID {
		return StaffAccountView{}, httpx.NewBadRequest("administrators cannot deactivate their own account", nil)
	}

	row, err := s.repo.DeactivateStaff(ctx, staffID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StaffAccountView{}, httpx.NewNotFound("active staff account not found")
		}
		return StaffAccountView{}, fmt.Errorf("svc_admin_deactivate_staff: %w", err)
	}

	// Instant revocation in server-side cache — zero stale-access window.
	if s.roleCache != nil {
		s.roleCache.Invalidate(staffID)
	}

	return StaffAccountView{
		ID:       row.ID,
		Username: row.Username,
		Role:     row.Role,
		IsActive: row.IsActive,
	}, nil
}

// --- Manual UPI Payment Verification Queue & Approval (Triggers WhatsApp Ticket Image + Message) ---

type BookingVerificationItem struct {
	BookingID        uuid.UUID        `json:"booking_id"`
	TicketID         uuid.UUID        `json:"ticket_id"`
	EventID          uuid.UUID        `json:"event_id"`
	EventTitle       string           `json:"event_title"`
	CategoryName     string           `json:"category_name"`
	CategoryTier     string           `json:"category_tier,omitempty"`
	EntryCount       int32            `json:"entry_count"`
	ExpectedAmount   int32            `json:"expected_amount"`
	TierBasePrice    int32            `json:"tier_base_price"`
	FnbCredit        int32            `json:"fnb_credit"`
	UPITransactionID string           `json:"upi_transaction_id,omitempty"`
	Status           db.BookingStatus `json:"status"`
	ContactPhone     string           `json:"contact_phone"`
	ContactEmail     string           `json:"contact_email"`
	BarCode          string           `json:"bar_code"`
	RejectionReason  string           `json:"rejection_reason,omitempty"`
	VerifiedAt       *time.Time       `json:"verified_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
}

func (s *AdminService) ListBookingsForVerification(
	ctx context.Context,
	status db.BookingStatus,
	eventID uuid.UUID,
	limit, offset int32,
) ([]BookingVerificationItem, error) {
	if status == "" {
		status = db.BookingStatusPENDING
	}
	switch status {
	case db.BookingStatusPENDING, db.BookingStatusCONFIRMED, db.BookingStatusFAILED:
	default:
		return nil, httpx.NewBadRequest("status must be PENDING, CONFIRMED, or FAILED", nil)
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.repo.ListBookingsForVerification(ctx, db.ListBookingsForVerificationParams{
		Status:  status,
		EventID: eventID,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("svc_admin_list_bookings: %w", err)
	}

	items := make([]BookingVerificationItem, 0, len(rows))
	for _, r := range rows {
		item := BookingVerificationItem{
			BookingID:      r.ID,
			TicketID:       r.TicketID,
			EventID:        r.EventID,
			EventTitle:     r.EventTitle,
			CategoryName:   r.CategoryName,
			EntryCount:     r.EntryCount,
			ExpectedAmount: r.TotalAmount,
			TierBasePrice:  r.TierBasePrice,
			FnbCredit:      r.TierFnbCredit,
			Status:         r.Status,
			ContactPhone:   r.ContactPhone,
			ContactEmail:   r.ContactEmail,
			BarCode:        r.BarCode,
			CreatedAt:      r.CreatedAt,
		}
		if r.CategoryTier.Valid {
			item.CategoryTier = r.CategoryTier.String
		}
		if r.UpiTransactionID.Valid {
			item.UPITransactionID = r.UpiTransactionID.String
		}
		if r.RejectionReason.Valid {
			item.RejectionReason = r.RejectionReason.String
		}
		if r.VerifiedAt.Valid {
			t := r.VerifiedAt.Time
			item.VerifiedAt = &t
		}
		items = append(items, item)
	}
	return items, nil
}

type VerifyBookingRequest struct {
	Action          string `json:"action"` // APPROVE or REJECT
	RejectionReason string `json:"rejection_reason,omitempty"`
}

type VerifyBookingResponse struct {
	BookingID         uuid.UUID        `json:"booking_id"`
	TicketID          uuid.UUID        `json:"ticket_id"`
	Status            db.BookingStatus `json:"status"`
	ExpectedAmount    int32            `json:"expected_amount"`
	UPITransactionID  string           `json:"upi_transaction_id,omitempty"`
	WhatsAppQueued    bool             `json:"whatsapp_queued"`
	VerifiedAt        time.Time        `json:"verified_at"`
}

// VerifyBookingPayment atomically approves or rejects a pending UPI booking under row-level lock.
// On approval, dispatches the rendered PNG ticket image + confirmation message via WhatsApp Cloud API.
func (s *AdminService) VerifyBookingPayment(
	ctx context.Context,
	bookingID, adminID uuid.UUID,
	req VerifyBookingRequest,
) (VerifyBookingResponse, error) {
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	if action != "APPROVE" && action != "REJECT" {
		return VerifyBookingResponse{}, httpx.NewBadRequest("action must be APPROVE or REJECT", nil)
	}

	var resp VerifyBookingResponse
	var waMsg *whatsapp.TicketPassMessage

	err := s.repo.WithTx(ctx, func(txCtx context.Context, q db.Querier) error {
		booking, err := q.GetBookingByIDForUpdate(txCtx, bookingID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NewNotFound("booking not found")
			}
			return fmt.Errorf("tx_admin_lock_booking: %w", err)
		}

		if booking.Status != db.BookingStatusPENDING {
			return httpx.NewConflict(fmt.Sprintf("booking is already %s", booking.Status), nil)
		}

		adminUUID := pgtype.UUID{Bytes: adminID, Valid: true}

		if action == "REJECT" {
			reason := strings.TrimSpace(req.RejectionReason)
			if reason == "" {
				reason = "UPI payment could not be verified"
			}
			if len(reason) > 255 {
				return httpx.NewBadRequest("rejection_reason must be at most 255 characters", nil)
			}
			rejected, err := q.RejectBookingAtomic(txCtx, db.RejectBookingAtomicParams{
				ID:              booking.ID,
				VerifiedBy:      adminUUID,
				RejectionReason: pgtype.Text{String: reason, Valid: true},
			})
			if err != nil {
				return fmt.Errorf("tx_admin_reject_booking: %w", err)
			}
			ticket, err := q.GetTicketByBookingID(txCtx, booking.ID)
			if err != nil {
				return fmt.Errorf("tx_admin_reject_get_ticket: %w", err)
			}
			resp = VerifyBookingResponse{
				BookingID:        rejected.ID,
				TicketID:         ticket.ID,
				Status:           rejected.Status,
				ExpectedAmount:   rejected.TotalAmount,
				UPITransactionID: rejected.UpiTransactionID.String,
				WhatsAppQueued:   false,
				VerifiedAt:       rejected.VerifiedAt.Time,
			}
			return nil
		}

		// APPROVE flow: require submitted UPI transaction ID for paid bookings.
		if booking.TotalAmount > 0 && (!booking.UpiTransactionID.Valid || strings.TrimSpace(booking.UpiTransactionID.String) == "") {
			return httpx.NewBadRequest("cannot approve paid booking before patron submits a UPI transaction ID", nil)
		}

		approved, err := q.ApproveBookingAtomic(txCtx, db.ApproveBookingAtomicParams{
			ID:         booking.ID,
			VerifiedBy: adminUUID,
		})
		if err != nil {
			return fmt.Errorf("tx_admin_approve_booking: %w", err)
		}

		ticket, err := q.GetTicketByBookingID(txCtx, approved.ID)
		if err != nil {
			return fmt.Errorf("tx_admin_approve_get_ticket: %w", err)
		}
		details, err := q.GetTicketDetailsByID(txCtx, ticket.ID)
		if err != nil {
			return fmt.Errorf("tx_admin_approve_ticket_details: %w", err)
		}
		guests, err := q.ListGuestsByTicketID(txCtx, ticket.ID)
		if err != nil {
			return fmt.Errorf("tx_admin_approve_guests: %w", err)
		}

		primaryName := PrimaryGuestName(guests)
		ticketURL := fmt.Sprintf("%s/tickets/%s", s.appBaseURL, ticket.ID.String())

		waMsg = &whatsapp.TicketPassMessage{
			TicketID:       ticket.ID.String(),
			RecipientPhone: approved.ContactPhone,
			PrimaryGuest:   primaryName,
			EventTitle:     details.EventTitle,
			EventLocation:  details.EventLocation,
			EventStartsAt:  details.EventStartsAt,
			CategoryName:   details.CategoryName,
			TicketURL:      ticketURL,
			BarCode:        ticket.BarCode,
			EntryCount:     details.EntryCount,
			FoodBalance:    ticket.FoodBalance,
		}

		resp = VerifyBookingResponse{
			BookingID:        approved.ID,
			TicketID:         ticket.ID,
			Status:           approved.Status,
			ExpectedAmount:   approved.TotalAmount,
			UPITransactionID: approved.UpiTransactionID.String,
			WhatsAppQueued:   s.notifier != nil,
			VerifiedAt:       approved.VerifiedAt.Time,
		}
		return nil
	})
	if err != nil {
		return VerifyBookingResponse{}, err
	}

	if waMsg != nil && s.notifier != nil {
		s.notifier.EnqueueTicketConfirmation(*waMsg)
	}
	return resp, nil
}

// --- Event & Tier Management ---

type CreateEventRequest struct {
	Title    string         `json:"title"`
	StartsAt time.Time      `json:"starts_at"`
	EndsAt   time.Time      `json:"ends_at"`
	Location string         `json:"location"`
	Status   db.EventStatus `json:"status"`
}

func (s *AdminService) CreateEvent(ctx context.Context, req CreateEventRequest) (db.Event, error) {
	title := strings.TrimSpace(req.Title)
	location := strings.TrimSpace(req.Location)
	if title == "" || len(title) > 255 || location == "" || len(location) > 255 {
		return db.Event{}, httpx.NewBadRequest("title and location are required (max 255 chars)", nil)
	}
	if req.StartsAt.IsZero() || req.EndsAt.IsZero() || !req.EndsAt.After(req.StartsAt) {
		return db.Event{}, httpx.NewBadRequest("ends_at must be strictly after starts_at", nil)
	}

	status := req.Status
	if status == "" {
		status = db.EventStatusDRAFT
	}
	if !isValidEventStatus(status) {
		return db.Event{}, httpx.NewBadRequest("invalid event status", nil)
	}

	ev, err := s.repo.CreateEvent(ctx, db.CreateEventParams{
		Title:    title,
		StartsAt: req.StartsAt.UTC(),
		EndsAt:   req.EndsAt.UTC(),
		Location: location,
		Status:   status,
	})
	if err != nil {
		return db.Event{}, fmt.Errorf("svc_admin_create_event: %w", err)
	}
	return ev, nil
}

type CreateTicketTierRequest struct {
	Name           string `json:"name"`
	Category       string `json:"category,omitempty"`
	BasePrice      int32  `json:"base_price"`
	FnbCredit      int32  `json:"fnb_credit"`
	EntryCount     int32  `json:"entry_count"`
	IsGenderStrict bool   `json:"is_gender_strict"`
}

func (s *AdminService) CreateTicketTier(ctx context.Context, eventID uuid.UUID, req CreateTicketTierRequest) (db.TicketCategory, error) {
	if _, err := s.repo.GetEventByID(ctx, eventID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.TicketCategory{}, httpx.NewNotFound("event not found")
		}
		return db.TicketCategory{}, fmt.Errorf("svc_admin_tier_get_event: %w", err)
	}

	name := strings.ToUpper(strings.TrimSpace(req.Name))
	if name == "" || len(name) > 100 {
		return db.TicketCategory{}, httpx.NewBadRequest("tier name is required (max 100 chars)", nil)
	}
	if req.BasePrice < 0 || req.FnbCredit < 0 {
		return db.TicketCategory{}, httpx.NewBadRequest("base_price and fnb_credit must be non-negative", nil)
	}
	switch req.EntryCount {
	case 1, 2, 10, 15:
	default:
		return db.TicketCategory{}, httpx.NewBadRequest("entry_count must be 1, 2, 10, or 15", nil)
	}

	genderStrict := req.IsGenderStrict
	if name == "COUPLE" {
		if req.EntryCount != 2 {
			return db.TicketCategory{}, httpx.NewBadRequest("COUPLE tier must have entry_count = 2", nil)
		}
		genderStrict = true
	}

	catText := pgtype.Text{Valid: false}
	if trimmed := strings.ToUpper(strings.TrimSpace(req.Category)); trimmed != "" {
		if len(trimmed) > 50 {
			return db.TicketCategory{}, httpx.NewBadRequest("category must be at most 50 characters", nil)
		}
		catText = pgtype.Text{String: trimmed, Valid: true}
	}

	tier, err := s.repo.CreateTicketCategory(ctx, db.CreateTicketCategoryParams{
		EventID:        eventID,
		Name:           name,
		Category:       catText,
		BasePrice:      req.BasePrice,
		FnbCredit:      req.FnbCredit,
		EntryCount:     req.EntryCount,
		IsGenderStrict: genderStrict,
	})
	if err != nil {
		return db.TicketCategory{}, fmt.Errorf("svc_admin_create_tier: %w", err)
	}
	return tier, nil
}

type EventStatsView struct {
	EventID              uuid.UUID `json:"event_id"`
	EventTitle           string    `json:"event_title"`
	ConfirmedBookings    int64     `json:"confirmed_bookings"`
	TicketRevenue        int64     `json:"ticket_revenue"`
	TotalTickets         int64     `json:"total_tickets"`
	CheckedInTickets     int64     `json:"checked_in_tickets"`
	TotalFnbAllocated    int64     `json:"total_fnb_allocated"`
	TotalFnbRedeemed     int64     `json:"total_fnb_redeemed"`
	TotalFnbRemaining    int64     `json:"total_fnb_remaining"`
	BarTransactions      int64     `json:"bar_transactions"`
	TotalBarBill         int64     `json:"total_bar_bill"`
	TotalWalletDeducted  int64     `json:"total_wallet_deducted"`
	TotalCounterDelta    int64     `json:"total_counter_delta"`
	TotalGuests          int64     `json:"total_guests"`
	MaleGuests           int64     `json:"male_guests"`
	FemaleGuests         int64     `json:"female_guests"`
	OtherGuests          int64     `json:"other_guests"`
	FreeShotsRedeemed    int64     `json:"free_shots_redeemed"`
	ConfirmedFemaleStags int64     `json:"confirmed_female_stags"`
}

func (s *AdminService) GetEventStats(ctx context.Context, eventID uuid.UUID) (EventStatsView, error) {
	ev, err := s.repo.GetEventByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EventStatsView{}, httpx.NewNotFound("event not found")
		}
		return EventStatsView{}, fmt.Errorf("svc_admin_stats_event: %w", err)
	}

	var (
		bookingStats db.GetEventBookingStatsRow
		barStats     db.GetEventBarRevenueStatsRow
		genderStats  db.GetEventGenderStatsRow
		femaleStags  int64
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		bookingStats, err = s.repo.GetEventBookingStats(gctx, eventID)
		return err
	})
	g.Go(func() error {
		var err error
		barStats, err = s.repo.GetEventBarRevenueStats(gctx, eventID)
		return err
	})
	g.Go(func() error {
		var err error
		genderStats, err = s.repo.GetEventGenderStats(gctx, eventID)
		return err
	})
	g.Go(func() error {
		var err error
		femaleStags, err = s.repo.CountConfirmedFemaleStagByEvent(gctx, eventID)
		return err
	})

	if err := g.Wait(); err != nil {
		return EventStatsView{}, fmt.Errorf("svc_admin_stats_aggregate: %w", err)
	}

	return EventStatsView{
		EventID:              ev.ID,
		EventTitle:           ev.Title,
		ConfirmedBookings:    bookingStats.ConfirmedBookings,
		TicketRevenue:        bookingStats.TicketRevenue,
		TotalTickets:         bookingStats.TotalTickets,
		CheckedInTickets:     bookingStats.CheckedInTickets,
		TotalFnbAllocated:    bookingStats.TotalFnbAllocated,
		TotalFnbRedeemed:     bookingStats.TotalFnbRedeemed,
		TotalFnbRemaining:    bookingStats.TotalFnbRemaining,
		BarTransactions:      barStats.TotalTransactions,
		TotalBarBill:         barStats.TotalBarBill,
		TotalWalletDeducted:  barStats.TotalWalletDeducted,
		TotalCounterDelta:    barStats.TotalUpiDelta,
		TotalGuests:          genderStats.TotalGuests,
		MaleGuests:           genderStats.MaleGuests,
		FemaleGuests:         genderStats.FemaleGuests,
		OtherGuests:          genderStats.OtherGuests,
		FreeShotsRedeemed:    genderStats.FreeShotsRedeemed,
		ConfirmedFemaleStags: femaleStags,
	}, nil
}

func isValidStaffRole(role db.StaffRole) bool {
	switch role {
	case db.StaffRoleADMIN, db.StaffRoleSCANNER, db.StaffRoleBARTENDER:
		return true
	default:
		return false
	}
}

func isValidEventStatus(status db.EventStatus) bool {
	switch status {
	case db.EventStatusDRAFT, db.EventStatusONSALE, db.EventStatusSOLDOUT, db.EventStatusENDED:
		return true
	default:
		return false
	}
}
