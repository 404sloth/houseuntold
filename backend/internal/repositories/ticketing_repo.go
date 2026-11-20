package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/db"
)

var (
	ErrSeatUnavailable      = errors.New("seat no longer available")
	ErrMaxSeatsExceeded     = errors.New("maximum seats per order exceeded")
	ErrInsufficientFnb      = errors.New("insufficient f&b wallet balance")
	ErrTicketNotFound       = errors.New("ticket not found")
	ErrTicketExpired        = errors.New("ticket has expired")
	ErrAlreadyCheckedIn     = errors.New("ticket already checked in")
	ErrTokenReusedOrRevoked = errors.New("refresh token revoked or reused")
	ErrOrderNotPending      = errors.New("order is not pending or already confirmed")
)

// TicketingStore defines the persistence contract so internal/services can be unit-tested cleanly without a live DB.
type TicketingStore interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (db.User, error)
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
	ListStaffUsers(ctx context.Context) ([]db.StaffUserRow, error)
	CreateStaffUser(ctx context.Context, arg db.CreateStaffUserParams) (db.StaffUserRow, error)
	UpsertGoogleUser(ctx context.Context, arg db.UpsertGoogleUserParams) (db.User, error)
	UpsertFirebasePhoneUser(ctx context.Context, arg db.UpsertFirebasePhoneUserParams) (db.User, error)
	MarkUserPhoneVerified(ctx context.Context, arg db.MarkUserPhoneVerifiedParams) (db.User, error)
	UpdateUserRole(ctx context.Context, userID uuid.UUID, role db.UserRole) (db.User, error)
	EnrollUserTOTPSecret(ctx context.Context, userID uuid.UUID, encryptedSecret string) (db.User, error)
	VerifyAndAdvanceTOTPStepTx(ctx context.Context, userID uuid.UUID, verifyFn func(user db.User) (int64, error)) (db.User, error)
	CreateRefreshToken(ctx context.Context, arg db.CreateRefreshTokenParams) (db.RefreshToken, error)
	RotateRefreshTokenTx(ctx context.Context, incomingHash, newHash string, newExpiresAt time.Time) (db.User, db.RefreshToken, error)
	RevokeTokenByHash(ctx context.Context, tokenHash string) error

	ListActiveEvents(ctx context.Context) ([]db.Event, error)
	GetEventByID(ctx context.Context, eventID uuid.UUID) (db.Event, error)
	ListEventSections(ctx context.Context, eventID uuid.UUID) ([]db.EventSection, error)
	ListSeatsByEvent(ctx context.Context, eventID uuid.UUID) ([]db.ListSeatsByEventRow, error)
	HoldSeatsAtomicallyTx(ctx context.Context, eventID uuid.UUID, seatIDs []uuid.UUID, userID uuid.UUID, holdTTL time.Duration, maxSeatsPerOrder int) error
	ClearUserHolds(ctx context.Context, eventID, userID uuid.UUID) error
	QuoteSeats(ctx context.Context, eventID uuid.UUID, seatIDs []uuid.UUID) ([]db.QuoteSeatsByIDsRow, error)
	CreatePendingOrderTx(ctx context.Context, orderArg db.CreateOrderParams, attendees []db.CreateOrderSeatParams) (db.Order, error)
	ConfirmOrderWebhookTx(ctx context.Context, razorpayOrderID, ticketCode, barCode, gateLabel string, fnbCredit int32, expiresAt time.Time, qrOtpSecret *string, computeDigest func(ticketID uuid.UUID) string) (db.Order, db.Ticket, error)
	ListTicketsByUser(ctx context.Context, userID uuid.UUID) ([]db.ListTicketsByUserRow, error)
	GetTicketByID(ctx context.Context, ticketID uuid.UUID) (db.Ticket, error)
	ListOrderAttendees(ctx context.Context, orderID uuid.UUID) ([]db.ListOrderAttendeesRow, error)

	LookupTicketByCodeOrDigest(ctx context.Context, query string) (db.GetTicketByCodeOrDigestRow, error)
	AdmitTicketTx(ctx context.Context, ticketID uuid.UUID, seatID *uuid.UUID, scannerUserID uuid.UUID) (db.Ticket, []db.ListOrderAttendeesRow, error)
	ListRecentScans(ctx context.Context, limit int32) ([]db.ListRecentScansRow, error)

	ListFnbMenuItems(ctx context.Context) ([]db.FnbMenuItem, error)
	CreateFnbMenuItem(ctx context.Context, arg db.CreateFnbMenuItemParams) (db.FnbMenuItem, error)
	GetFnbMenuItemsByIDs(ctx context.Context, ids []uuid.UUID) ([]db.FnbMenuItem, error)
	ListRecentFnbTransactions(ctx context.Context, limit int32) ([]db.ListRecentFnbTransactionsRow, error)
	DeductFnBWalletTx(ctx context.Context, ticketID, bartenderID uuid.UUID, summary string, totalAmount, walletDeduct, upiDelta int32) (db.Ticket, db.FnbTransaction, error)

	GetAdminOverviewStats(ctx context.Context) (db.GetAdminOverviewStatsRow, error)
	ListRecentOrders(ctx context.Context, limit int32) ([]db.Order, error)
	UpdateEventStatus(ctx context.Context, eventID uuid.UUID, status string) (db.Event, error)
	UpdateEventLayout(ctx context.Context, eventID uuid.UUID, layoutType string, layoutJSON []byte) (db.Event, error)
	CreateEventWithInventoryTx(ctx context.Context, eventArg db.CreateEventParams, vipPrice, generalPrice, vipFnbCredit, vipMinAge int32) (db.Event, []db.EventSection, error)
}

var _ TicketingStore = (*TicketingRepository)(nil)

type TicketingRepository struct {
	pool      *pgxpool.Pool
	queries   *db.Queries
	dbTimeout time.Duration
}

func NewTicketingRepository(pool *pgxpool.Pool, dbTimeout time.Duration) *TicketingRepository {
	if dbTimeout <= 0 {
		dbTimeout = 3 * time.Second
	}
	return &TicketingRepository{
		pool:      pool,
		queries:   db.New(pool),
		dbTimeout: dbTimeout,
	}
}

func (r *TicketingRepository) Queries() db.Querier {
	return r.queries
}

func (r *TicketingRepository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.dbTimeout)
}

// --- Auth & Refresh Token Rotation ---

func (r *TicketingRepository) GetUserByID(ctx context.Context, id uuid.UUID) (db.User, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	u, err := r.queries.GetUserByID(qCtx, id)
	if err != nil {
		return db.User{}, fmt.Errorf("repo_get_user_by_id: %w", err)
	}
	return u, nil
}

func (r *TicketingRepository) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	u, err := r.queries.GetUserByEmail(qCtx, email)
	if err != nil {
		return db.User{}, fmt.Errorf("repo_get_user_by_email: %w", err)
	}
	return u, nil
}

func (r *TicketingRepository) ListStaffUsers(ctx context.Context) ([]db.StaffUserRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	rows, err := r.queries.ListStaffUsers(qCtx)
	if err != nil {
		return nil, fmt.Errorf("repo_list_staff_users: %w", err)
	}
	return rows, nil
}

func (r *TicketingRepository) CreateStaffUser(ctx context.Context, arg db.CreateStaffUserParams) (db.StaffUserRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.CreateStaffUser(qCtx, arg)
	if err != nil {
		return db.StaffUserRow{}, fmt.Errorf("repo_create_staff_user: %w", err)
	}
	return row, nil
}

func (r *TicketingRepository) UpsertGoogleUser(ctx context.Context, arg db.UpsertGoogleUserParams) (db.User, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	u, err := r.queries.UpsertGoogleUser(qCtx, arg)
	if err != nil {
		return db.User{}, fmt.Errorf("repo_upsert_google_user: %w", err)
	}
	return u, nil
}

func (r *TicketingRepository) UpsertFirebasePhoneUser(ctx context.Context, arg db.UpsertFirebasePhoneUserParams) (db.User, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	u, err := r.queries.UpsertFirebasePhoneUser(qCtx, arg)
	if err != nil {
		return db.User{}, fmt.Errorf("repo_upsert_firebase_user: %w", err)
	}
	return u, nil
}

func (r *TicketingRepository) MarkUserPhoneVerified(ctx context.Context, arg db.MarkUserPhoneVerifiedParams) (db.User, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	u, err := r.queries.MarkUserPhoneVerified(qCtx, arg)
	if err != nil {
		return db.User{}, fmt.Errorf("repo_mark_phone_verified: %w", err)
	}
	return u, nil
}

func (r *TicketingRepository) UpdateUserRole(ctx context.Context, userID uuid.UUID, role db.UserRole) (db.User, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	u, err := r.queries.UpdateUserRole(qCtx, db.UpdateUserRoleParams{ID: userID, Role: role})
	if err != nil {
		return db.User{}, fmt.Errorf("repo_update_user_role: %w", err)
	}
	return u, nil
}

func (r *TicketingRepository) EnrollUserTOTPSecret(ctx context.Context, userID uuid.UUID, encryptedSecret string) (db.User, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	u, err := r.queries.EnrollUserTOTP(qCtx, db.EnrollUserTOTPParams{
		ID:         userID,
		TotpSecret: &encryptedSecret,
	})
	if err != nil {
		return db.User{}, fmt.Errorf("repo_enroll_totp: %w", err)
	}
	return u, nil
}

func (r *TicketingRepository) VerifyAndAdvanceTOTPStepTx(
	ctx context.Context,
	userID uuid.UUID,
	verifyFn func(user db.User) (int64, error),
) (db.User, error) {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return db.User{}, fmt.Errorf("repo_mfa_begin: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)

	user, err := qtx.GetUserForMFAUpdate(txCtx, userID)
	if err != nil {
		return db.User{}, fmt.Errorf("repo_mfa_lock_user: %w", err)
	}

	matchedStep, err := verifyFn(user)
	if err != nil {
		return db.User{}, err
	}

	updated, err := qtx.AdvanceTOTPLastAcceptedStep(txCtx, db.AdvanceTOTPLastAcceptedStepParams{
		ID:                   userID,
		TotpLastAcceptedStep: &matchedStep,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, auth.ErrTOTPCodeReused
		}
		return db.User{}, fmt.Errorf("repo_mfa_advance_step: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return db.User{}, fmt.Errorf("repo_mfa_commit: %w", err)
	}
	return updated, nil
}

func (r *TicketingRepository) CreateRefreshToken(ctx context.Context, arg db.CreateRefreshTokenParams) (db.RefreshToken, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	rt, err := r.queries.CreateRefreshToken(qCtx, arg)
	if err != nil {
		return db.RefreshToken{}, fmt.Errorf("repo_create_refresh_token: %w", err)
	}
	return rt, nil
}

func (r *TicketingRepository) RotateRefreshTokenTx(
	ctx context.Context,
	incomingHash string,
	newHash string,
	newExpiresAt time.Time,
) (db.User, db.RefreshToken, error) {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_rotate_token_begin: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)

	existing, err := qtx.GetRefreshTokenByHash(txCtx, incomingHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, db.RefreshToken{}, ErrTokenReusedOrRevoked
		}
		return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_rotate_lookup: %w", err)
	}

	if existing.Revoked || existing.ExpiresAt.Before(time.Now().UTC()) {
		if revErr := qtx.RevokeTokenFamily(txCtx, existing.FamilyID); revErr != nil {
			return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_revoke_family: %w", revErr)
		}
		if commitErr := tx.Commit(txCtx); commitErr != nil {
			return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_revoke_family_commit: %w", commitErr)
		}
		return db.User{}, db.RefreshToken{}, ErrTokenReusedOrRevoked
	}

	if err := qtx.RevokeRefreshToken(txCtx, existing.ID); err != nil {
		return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_revoke_old_token: %w", err)
	}

	user, err := qtx.GetUserByID(txCtx, existing.UserID)
	if err != nil {
		return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_rotate_load_user: %w", err)
	}
	if !user.IsActive {
		return db.User{}, db.RefreshToken{}, ErrTokenReusedOrRevoked
	}

	rotated, err := qtx.CreateRefreshToken(txCtx, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: newHash,
		FamilyID:  existing.FamilyID,
		ExpiresAt: newExpiresAt,
	})
	if err != nil {
		return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_create_rotated_token: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return db.User{}, db.RefreshToken{}, fmt.Errorf("repo_rotate_commit: %w", err)
	}
	return user, rotated, nil
}

func (r *TicketingRepository) RevokeTokenByHash(ctx context.Context, tokenHash string) error {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tok, err := r.queries.GetRefreshTokenByHash(qCtx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("repo_logout_lookup: %w", err)
	}
	if err := r.queries.RevokeRefreshToken(qCtx, tok.ID); err != nil {
		return fmt.Errorf("repo_logout_revoke: %w", err)
	}
	return nil
}

// --- Events & Seat Inventory ---

func (r *TicketingRepository) ListActiveEvents(ctx context.Context) ([]db.Event, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	evts, err := r.queries.ListActiveEvents(qCtx)
	if err != nil {
		return nil, fmt.Errorf("repo_list_active_events: %w", err)
	}
	return evts, nil
}

func (r *TicketingRepository) GetEventByID(ctx context.Context, eventID uuid.UUID) (db.Event, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	evt, err := r.queries.GetEventByID(qCtx, eventID)
	if err != nil {
		return db.Event{}, fmt.Errorf("repo_get_event: %w", err)
	}
	return evt, nil
}

func (r *TicketingRepository) ListEventSections(ctx context.Context, eventID uuid.UUID) ([]db.EventSection, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	secs, err := r.queries.ListEventSections(qCtx, eventID)
	if err != nil {
		return nil, fmt.Errorf("repo_list_sections: %w", err)
	}
	return secs, nil
}

func (r *TicketingRepository) ListSeatsByEvent(ctx context.Context, eventID uuid.UUID) ([]db.ListSeatsByEventRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	seats, err := r.queries.ListSeatsByEvent(qCtx, eventID)
	if err != nil {
		return nil, fmt.Errorf("repo_list_seats: %w", err)
	}
	return seats, nil
}

// HoldSeatsAtomicallyTx locks and holds one or more seats inside a PostgreSQL transaction via sqlc.
func (r *TicketingRepository) HoldSeatsAtomicallyTx(
	ctx context.Context,
	eventID uuid.UUID,
	seatIDs []uuid.UUID,
	userID uuid.UUID,
	holdTTL time.Duration,
	maxSeatsPerOrder int,
) error {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return fmt.Errorf("hold_seats_begin_tx: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)
	now := time.Now().UTC()

	for _, seatID := range seatIDs {
		locked, err := qtx.LockSeatForUpdate(txCtx, db.LockSeatForUpdateParams{
			ID:      seatID,
			EventID: eventID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrSeatUnavailable
			}
			return fmt.Errorf("hold_seats_lock: %w", err)
		}

		isExpiredHold := locked.Status == db.SeatStatusHELD && locked.HoldExpiresAt != nil && locked.HoldExpiresAt.Before(now)
		isOwnedByMe := locked.Status == db.SeatStatusHELD && locked.HeldByUserID != nil && *locked.HeldByUserID == userID

		if locked.Status == db.SeatStatusSOLD || (locked.Status == db.SeatStatusHELD && !isExpiredHold && !isOwnedByMe) {
			return ErrSeatUnavailable
		}

		if isOwnedByMe && len(seatIDs) == 1 {
			if err := qtx.ReleaseSeatHold(txCtx, seatID); err != nil {
				return fmt.Errorf("hold_seats_release: %w", err)
			}
			continue
		}

		if maxSeatsPerOrder > 0 {
			seats, err := qtx.ListSeatsByEvent(txCtx, eventID)
			if err != nil {
				return fmt.Errorf("hold_seats_count: %w", err)
			}
			activeMine := 0
			for _, s := range seats {
				if s.Status == db.SeatStatusHELD && s.HeldByUserID != nil && *s.HeldByUserID == userID && (s.HoldExpiresAt == nil || s.HoldExpiresAt.After(now)) {
					activeMine++
				}
			}
			if activeMine >= maxSeatsPerOrder {
				return ErrMaxSeatsExceeded
			}
		}

		exp := now.Add(holdTTL)
		rowsAffected, err := qtx.HoldSeat(txCtx, db.HoldSeatParams{
			ID:            seatID,
			HeldByUserID:  &userID,
			HoldExpiresAt: &exp,
		})
		if err != nil {
			return fmt.Errorf("hold_seats_acquire: %w", err)
		}
		if rowsAffected == 0 && !isOwnedByMe {
			return ErrSeatUnavailable
		}
	}

	if err := tx.Commit(txCtx); err != nil {
		return fmt.Errorf("hold_seats_commit: %w", err)
	}
	return nil
}

func (r *TicketingRepository) ClearUserHolds(ctx context.Context, eventID, userID uuid.UUID) error {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	if err := r.queries.ClearUserHoldsForEvent(qCtx, db.ClearUserHoldsForEventParams{
		EventID:      eventID,
		HeldByUserID: &userID,
	}); err != nil {
		return fmt.Errorf("repo_clear_holds: %w", err)
	}
	return nil
}

func (r *TicketingRepository) QuoteSeats(ctx context.Context, eventID uuid.UUID, seatIDs []uuid.UUID) ([]db.QuoteSeatsByIDsRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	rows, err := r.queries.QuoteSeatsByIDs(qCtx, db.QuoteSeatsByIDsParams{
		EventID: eventID,
		SeatIDs: seatIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("repo_quote_seats: %w", err)
	}
	return rows, nil
}

func (r *TicketingRepository) CreatePendingOrderTx(
	ctx context.Context,
	orderArg db.CreateOrderParams,
	attendees []db.CreateOrderSeatParams,
) (db.Order, error) {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return db.Order{}, fmt.Errorf("create_order_begin: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)

	order, err := qtx.CreateOrder(txCtx, orderArg)
	if err != nil {
		return db.Order{}, fmt.Errorf("create_order_insert: %w", err)
	}
	for _, a := range attendees {
		a.OrderID = order.ID
		if err := qtx.CreateOrderSeat(txCtx, a); err != nil {
			return db.Order{}, fmt.Errorf("create_order_seat: %w", err)
		}
	}

	if err := tx.Commit(txCtx); err != nil {
		return db.Order{}, fmt.Errorf("create_order_commit: %w", err)
	}
	return order, nil
}

func (r *TicketingRepository) ConfirmOrderWebhookTx(
	ctx context.Context,
	razorpayOrderID string,
	ticketCode string,
	barCode string,
	gateLabel string,
	fnbCredit int32,
	expiresAt time.Time,
	qrOtpSecret *string,
	computeDigest func(ticketID uuid.UUID) string,
) (db.Order, db.Ticket, error) {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return db.Order{}, db.Ticket{}, fmt.Errorf("confirm_order_begin: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)

	order, err := qtx.ConfirmOrderByRazorpayID(txCtx, razorpayOrderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Order{}, db.Ticket{}, ErrOrderNotPending
		}
		return db.Order{}, db.Ticket{}, fmt.Errorf("confirm_order_update: %w", err)
	}

	if err := qtx.MarkOrderSeatsSold(txCtx, order.ID); err != nil {
		return db.Order{}, db.Ticket{}, fmt.Errorf("confirm_order_mark_sold: %w", err)
	}

	ticketID := uuid.New()
	qrDigest := computeDigest(ticketID)

	ticket, err := qtx.CreateTicket(txCtx, db.CreateTicketParams{
		ID:                 ticketID,
		OrderID:            order.ID,
		EventID:            order.EventID,
		UserID:             order.UserID,
		Code:               ticketCode,
		BarCode:            barCode,
		QrTokenDigest:      qrDigest,
		QrOtpSecret:        qrOtpSecret,
		GateLabel:          gateLabel,
		FoodBalance:        fnbCredit,
		InitialFoodBalance: fnbCredit,
		ExpiresAt:          &expiresAt,
	})
	if err != nil {
		return db.Order{}, db.Ticket{}, fmt.Errorf("confirm_order_create_ticket: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return db.Order{}, db.Ticket{}, fmt.Errorf("confirm_order_commit: %w", err)
	}
	return order, ticket, nil
}

func (r *TicketingRepository) ListTicketsByUser(ctx context.Context, userID uuid.UUID) ([]db.ListTicketsByUserRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	rows, err := r.queries.ListTicketsByUser(qCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("repo_list_user_tickets: %w", err)
	}
	return rows, nil
}

func (r *TicketingRepository) GetTicketByID(ctx context.Context, ticketID uuid.UUID) (db.Ticket, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tkt, err := r.queries.GetTicketByID(qCtx, ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Ticket{}, ErrTicketNotFound
		}
		return db.Ticket{}, fmt.Errorf("repo_get_ticket_by_id: %w", err)
	}
	return tkt, nil
}

func (r *TicketingRepository) ListOrderAttendees(ctx context.Context, orderID uuid.UUID) ([]db.ListOrderAttendeesRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	rows, err := r.queries.ListOrderAttendees(qCtx, orderID)
	if err != nil {
		return nil, fmt.Errorf("repo_list_order_attendees: %w", err)
	}
	return rows, nil
}

// --- Gate Scanner ---

func (r *TicketingRepository) LookupTicketByCodeOrDigest(ctx context.Context, query string) (db.GetTicketByCodeOrDigestRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.GetTicketByCodeOrDigest(qCtx, query)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.GetTicketByCodeOrDigestRow{}, ErrTicketNotFound
		}
		return db.GetTicketByCodeOrDigestRow{}, fmt.Errorf("repo_lookup_ticket: %w", err)
	}
	return row, nil
}

func (r *TicketingRepository) AdmitTicketTx(
	ctx context.Context,
	ticketID uuid.UUID,
	seatID *uuid.UUID,
	scannerUserID uuid.UUID,
) (db.Ticket, []db.ListOrderAttendeesRow, error) {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return db.Ticket{}, nil, fmt.Errorf("admit_ticket_begin: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)

	tkt, err := qtx.GetTicketForUpdate(txCtx, ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Ticket{}, nil, ErrTicketNotFound
		}
		return db.Ticket{}, nil, fmt.Errorf("admit_ticket_lock: %w", err)
	}

	if tkt.CheckedIn {
		_, _ = qtx.InsertGateScanLog(txCtx, db.InsertGateScanLogParams{
			TicketID:      &tkt.ID,
			ScannerUserID: scannerUserID,
			TicketCode:    tkt.Code,
			Status:        "ALREADY_USED",
		})
		_ = tx.Commit(txCtx)
		return db.Ticket{}, nil, ErrAlreadyCheckedIn
	}

	if err := qtx.MarkOrderSeatAdmitted(txCtx, db.MarkOrderSeatAdmittedParams{
		OrderID: tkt.OrderID,
		SeatID:  seatID,
	}); err != nil {
		return db.Ticket{}, nil, fmt.Errorf("admit_ticket_mark_seat: %w", err)
	}

	unadmitted, err := qtx.CountUnadmittedSeats(txCtx, tkt.OrderID)
	if err != nil {
		return db.Ticket{}, nil, fmt.Errorf("admit_ticket_count_unadmitted: %w", err)
	}

	if unadmitted == 0 {
		tkt, err = qtx.MarkTicketCheckedIn(txCtx, db.MarkTicketCheckedInParams{
			ID:          tkt.ID,
			CheckedInBy: &scannerUserID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return db.Ticket{}, nil, ErrAlreadyCheckedIn
			}
			return db.Ticket{}, nil, fmt.Errorf("admit_ticket_check_in: %w", err)
		}
	}

	if _, err := qtx.InsertGateScanLog(txCtx, db.InsertGateScanLogParams{
		TicketID:      &tkt.ID,
		ScannerUserID: scannerUserID,
		TicketCode:    tkt.Code,
		Status:        "VALID_ADMITTED",
	}); err != nil {
		return db.Ticket{}, nil, fmt.Errorf("admit_ticket_scan_log: %w", err)
	}

	attendees, err := qtx.ListOrderAttendees(txCtx, tkt.OrderID)
	if err != nil {
		return db.Ticket{}, nil, fmt.Errorf("admit_ticket_attendees: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return db.Ticket{}, nil, fmt.Errorf("admit_ticket_commit: %w", err)
	}
	return tkt, attendees, nil
}

func (r *TicketingRepository) ListRecentScans(ctx context.Context, limit int32) ([]db.ListRecentScansRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	rows, err := r.queries.ListRecentScans(qCtx, limit)
	if err != nil {
		return nil, fmt.Errorf("repo_list_recent_scans: %w", err)
	}
	return rows, nil
}

// --- Bartender F&B Counter ---

func (r *TicketingRepository) ListFnbMenuItems(ctx context.Context) ([]db.FnbMenuItem, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	items, err := r.queries.ListFnbMenuItems(qCtx)
	if err != nil {
		return nil, fmt.Errorf("repo_list_fnb_menu: %w", err)
	}
	return items, nil
}

func (r *TicketingRepository) CreateFnbMenuItem(ctx context.Context, arg db.CreateFnbMenuItemParams) (db.FnbMenuItem, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	item, err := r.queries.CreateFnbMenuItem(qCtx, arg)
	if err != nil {
		return db.FnbMenuItem{}, fmt.Errorf("repo_create_fnb_item: %w", err)
	}
	return item, nil
}

func (r *TicketingRepository) GetFnbMenuItemsByIDs(ctx context.Context, ids []uuid.UUID) ([]db.FnbMenuItem, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	items, err := r.queries.GetFnbMenuItemsByIDs(qCtx, ids)
	if err != nil {
		return nil, fmt.Errorf("repo_get_fnb_items: %w", err)
	}
	return items, nil
}

func (r *TicketingRepository) ListRecentFnbTransactions(ctx context.Context, limit int32) ([]db.ListRecentFnbTransactionsRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	txs, err := r.queries.ListRecentFnbTransactions(qCtx, limit)
	if err != nil {
		return nil, fmt.Errorf("repo_list_fnb_txs: %w", err)
	}
	return txs, nil
}

func (r *TicketingRepository) DeductFnBWalletTx(
	ctx context.Context,
	ticketID, bartenderID uuid.UUID,
	summary string,
	totalAmount, walletDeduct, upiDelta int32,
) (db.Ticket, db.FnbTransaction, error) {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return db.Ticket{}, db.FnbTransaction{}, fmt.Errorf("fnb_deduct_begin: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)

	locked, err := qtx.GetTicketForUpdate(txCtx, ticketID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Ticket{}, db.FnbTransaction{}, ErrTicketNotFound
		}
		return db.Ticket{}, db.FnbTransaction{}, fmt.Errorf("fnb_deduct_lock: %w", err)
	}
	if locked.ExpiresAt != nil && !locked.ExpiresAt.After(time.Now().UTC()) {
		return db.Ticket{}, db.FnbTransaction{}, ErrTicketExpired
	}
	if locked.FoodBalance < walletDeduct {
		return db.Ticket{}, db.FnbTransaction{}, ErrInsufficientFnb
	}

	updatedTicket, err := qtx.DeductTicketFoodBalance(txCtx, db.DeductTicketFoodBalanceParams{
		ID:           ticketID,
		WalletDeduct: walletDeduct,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Ticket{}, db.FnbTransaction{}, ErrInsufficientFnb
		}
		return db.Ticket{}, db.FnbTransaction{}, fmt.Errorf("fnb_deduct_balance: %w", err)
	}

	auditTx, err := qtx.CreateFnbTransaction(txCtx, db.CreateFnbTransactionParams{
		TicketID:        ticketID,
		BartenderUserID: bartenderID,
		ItemsSummary:    summary,
		TotalAmount:     totalAmount,
		WalletDeducted:  walletDeduct,
		UpiDeltaPaid:    upiDelta,
	})
	if err != nil {
		return db.Ticket{}, db.FnbTransaction{}, fmt.Errorf("fnb_deduct_audit: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return db.Ticket{}, db.FnbTransaction{}, fmt.Errorf("fnb_deduct_commit: %w", err)
	}
	return updatedTicket, auditTx, nil
}

// --- Admin Console ---

func (r *TicketingRepository) GetAdminOverviewStats(ctx context.Context) (db.GetAdminOverviewStatsRow, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	stats, err := r.queries.GetAdminOverviewStats(qCtx)
	if err != nil {
		return db.GetAdminOverviewStatsRow{}, fmt.Errorf("repo_admin_stats: %w", err)
	}
	return stats, nil
}

func (r *TicketingRepository) ListRecentOrders(ctx context.Context, limit int32) ([]db.Order, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	orders, err := r.queries.ListRecentOrders(qCtx, limit)
	if err != nil {
		return nil, fmt.Errorf("repo_list_orders: %w", err)
	}
	return orders, nil
}

func (r *TicketingRepository) UpdateEventStatus(ctx context.Context, eventID uuid.UUID, status string) (db.Event, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	evt, err := r.queries.UpdateEventStatus(qCtx, db.UpdateEventStatusParams{
		ID:     eventID,
		Status: status,
	})
	if err != nil {
		return db.Event{}, fmt.Errorf("repo_update_event_status: %w", err)
	}
	return evt, nil
}

func (r *TicketingRepository) UpdateEventLayout(ctx context.Context, eventID uuid.UUID, layoutType string, layoutJSON []byte) (db.Event, error) {
	qCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	evt, err := r.queries.UpdateEventLayout(qCtx, db.UpdateEventLayoutParams{
		ID:         eventID,
		LayoutType: layoutType,
		LayoutJson: layoutJSON,
	})
	if err != nil {
		return db.Event{}, fmt.Errorf("repo_update_event_layout: %w", err)
	}
	return evt, nil
}

func (r *TicketingRepository) CreateEventWithInventoryTx(
	ctx context.Context,
	eventArg db.CreateEventParams,
	vipPrice, generalPrice int32,
	vipFnbCredit int32,
	vipMinAge int32,
) (db.Event, []db.EventSection, error) {
	txCtx, cancel := r.withTimeout(ctx)
	defer cancel()

	tx, err := r.pool.Begin(txCtx)
	if err != nil {
		return db.Event{}, nil, fmt.Errorf("create_event_begin: %w", err)
	}
	defer tx.Rollback(txCtx)

	qtx := r.queries.WithTx(tx)

	evt, err := qtx.CreateEvent(txCtx, eventArg)
	if err != nil {
		return db.Event{}, nil, fmt.Errorf("create_event_insert: %w", err)
	}

	vipSection, err := qtx.CreateEventSection(txCtx, db.CreateEventSectionParams{
		EventID:      evt.ID,
		Name:         "VIP / Stage",
		Tier:         db.TierTypeVIP,
		SectionPrice: vipPrice,
		FnbCredit:    vipFnbCredit,
		IsAlcoholic:  vipMinAge > 0,
		MinAge:       vipMinAge,
		Description:  fmt.Sprintf("Front stage seating with INR %d F&B credit", vipFnbCredit),
	})
	if err != nil {
		return db.Event{}, nil, fmt.Errorf("create_event_vip_section: %w", err)
	}

	genSection, err := qtx.CreateEventSection(txCtx, db.CreateEventSectionParams{
		EventID:      evt.ID,
		Name:         "General",
		Tier:         db.TierTypeGENERAL,
		SectionPrice: generalPrice,
		FnbCredit:    0,
		IsAlcoholic:  false,
		MinAge:       0,
		Description:  "Main hall seating (All Ages)",
	})
	if err != nil {
		return db.Event{}, nil, fmt.Errorf("create_event_gen_section: %w", err)
	}

	rows := []string{"A", "B", "C"}
	for rIdx, rowLabel := range rows {
		secID := genSection.ID
		if rIdx == 0 {
			secID = vipSection.ID
		}
		for num := int32(1); num <= 8; num++ {
			if _, err := qtx.CreateSeatInventory(txCtx, db.CreateSeatInventoryParams{
				EventID:    evt.ID,
				SectionID:  secID,
				RowLabel:   rowLabel,
				SeatNumber: num,
				Label:      fmt.Sprintf("%s-%d", rowLabel, num),
				PosX:       num,
				PosY:       int32(rIdx + 1),
			}); err != nil {
				return db.Event{}, nil, fmt.Errorf("create_event_seat_%s_%d: %w", rowLabel, num, err)
			}
		}
	}

	if err := tx.Commit(txCtx); err != nil {
		return db.Event{}, nil, fmt.Errorf("create_event_commit: %w", err)
	}
	return evt, []db.EventSection{vipSection, genSection}, nil
}
