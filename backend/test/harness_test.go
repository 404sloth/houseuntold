package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/cache"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/handlers"
	"github.com/venu-live/venu-api/internal/middleware"
	"github.com/venu-live/venu-api/internal/ratelimit"
	"github.com/venu-live/venu-api/internal/repositories"
	"github.com/venu-live/venu-api/internal/server"
	"github.com/venu-live/venu-api/internal/services"
	"github.com/venu-live/venu-api/internal/whatsapp"
)

// recordingNotifier captures WhatsApp ticket confirmations dispatched during checkout/admin approval.
type recordingNotifier struct {
	mu   sync.Mutex
	msgs []whatsapp.TicketPassMessage
}

func (n *recordingNotifier) EnqueueTicketConfirmation(msg whatsapp.TicketPassMessage) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.msgs = append(n.msgs, msg)
}

func (n *recordingNotifier) CloseAndWait(context.Context) error { return nil }

func (n *recordingNotifier) Messages() []whatsapp.TicketPassMessage {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]whatsapp.TicketPassMessage, len(n.msgs))
	copy(out, n.msgs)
	return out
}

// memStore implements db.Querier in memory with PostgreSQL-faithful uniqueness and atomic update semantics.
type memStore struct {
	mu           sync.Mutex
	txMu         sync.Mutex
	pingErr      error
	staff        map[uuid.UUID]db.StaffAccount
	events       map[uuid.UUID]db.Event
	categories   map[uuid.UUID]db.TicketCategory
	bookings     map[uuid.UUID]db.Booking
	tickets      map[uuid.UUID]db.Ticket
	guests       map[uuid.UUID]db.TicketGuest
	menuItems    map[uuid.UUID]db.MenuItem
	barTxs       map[uuid.UUID]db.BarTransaction
	barTxOrder   []uuid.UUID
	lineItems    []db.TransactionLineItem
	rateLimits   map[string]int32
}

var _ db.Querier = (*memStore)(nil)

func newMemStore() *memStore {
	return &memStore{
		staff:      make(map[uuid.UUID]db.StaffAccount),
		events:     make(map[uuid.UUID]db.Event),
		categories: make(map[uuid.UUID]db.TicketCategory),
		bookings:   make(map[uuid.UUID]db.Booking),
		tickets:    make(map[uuid.UUID]db.Ticket),
		guests:     make(map[uuid.UUID]db.TicketGuest),
		menuItems:  make(map[uuid.UUID]db.MenuItem),
		barTxs:     make(map[uuid.UUID]db.BarTransaction),
		rateLimits: make(map[string]int32),
	}
}

func (s *memStore) withTx(ctx context.Context, fn func(context.Context, db.Querier) error) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	return fn(ctx, s)
}

func (s *memStore) ping(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingErr
}

func (s *memStore) setPingErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingErr = err
}

func pgUniqueErr(constraint string) error {
	return &pgconn.PgError{Code: "23505", ConstraintName: constraint}
}

// --- Staff Queries ---

func (s *memStore) GetStaffByUsername(_ context.Context, username string) (db.StaffAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.staff {
		if st.Username == username {
			return st, nil
		}
	}
	return db.StaffAccount{}, pgx.ErrNoRows
}

func (s *memStore) GetStaffByID(_ context.Context, id uuid.UUID) (db.StaffAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.staff[id]
	if !ok {
		return db.StaffAccount{}, pgx.ErrNoRows
	}
	return st, nil
}

func (s *memStore) CreateStaff(_ context.Context, arg db.CreateStaffParams) (db.StaffAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.staff {
		if st.Username == arg.Username {
			return db.StaffAccount{}, pgUniqueErr("staff_accounts_username_key")
		}
	}
	st := db.StaffAccount{
		ID:           uuid.New(),
		Username:     arg.Username,
		PasswordHash: arg.PasswordHash,
		Role:         arg.Role,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}
	s.staff[st.ID] = st
	return st, nil
}

func (s *memStore) UpdateStaffPassword(_ context.Context, arg db.UpdateStaffPasswordParams) (db.UpdateStaffPasswordRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.staff[arg.ID]
	if !ok || !st.IsActive {
		return db.UpdateStaffPasswordRow{}, pgx.ErrNoRows
	}
	st.PasswordHash = arg.PasswordHash
	s.staff[st.ID] = st
	return db.UpdateStaffPasswordRow{
		ID:        st.ID,
		Username:  st.Username,
		Role:      st.Role,
		IsActive:  st.IsActive,
		CreatedAt: st.CreatedAt,
	}, nil
}

func (s *memStore) DeactivateStaff(_ context.Context, id uuid.UUID) (db.DeactivateStaffRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.staff[id]
	if !ok || !st.IsActive {
		return db.DeactivateStaffRow{}, pgx.ErrNoRows
	}
	st.IsActive = false
	s.staff[st.ID] = st
	return db.DeactivateStaffRow{
		ID:       st.ID,
		Username: st.Username,
		Role:     st.Role,
		IsActive: st.IsActive,
	}, nil
}

// --- Event & Category Queries ---

func (s *memStore) GetEventByID(_ context.Context, id uuid.UUID) (db.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, ok := s.events[id]
	if !ok {
		return db.Event{}, pgx.ErrNoRows
	}
	return ev, nil
}

func (s *memStore) GetEventByIDForUpdate(ctx context.Context, id uuid.UUID) (db.Event, error) {
	return s.GetEventByID(ctx, id)
}

func (s *memStore) CreateEvent(_ context.Context, arg db.CreateEventParams) (db.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := db.Event{
		ID:        uuid.New(),
		Title:     arg.Title,
		StartsAt:  arg.StartsAt,
		EndsAt:    arg.EndsAt,
		Location:  arg.Location,
		Status:    arg.Status,
		CreatedAt: time.Now().UTC(),
	}
	s.events[ev.ID] = ev
	return ev, nil
}

func (s *memStore) ListEventCategories(_ context.Context, eventID uuid.UUID) ([]db.TicketCategory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.TicketCategory, 0)
	for _, cat := range s.categories {
		if cat.EventID == eventID {
			out = append(out, cat)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BasePrice < out[j].BasePrice })
	return out, nil
}

func (s *memStore) GetCategoryByID(_ context.Context, id uuid.UUID) (db.TicketCategory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cat, ok := s.categories[id]
	if !ok {
		return db.TicketCategory{}, pgx.ErrNoRows
	}
	return cat, nil
}

func (s *memStore) CreateTicketCategory(_ context.Context, arg db.CreateTicketCategoryParams) (db.TicketCategory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cat := db.TicketCategory{
		ID:             uuid.New(),
		EventID:        arg.EventID,
		Name:           arg.Name,
		Category:       arg.Category,
		BasePrice:      arg.BasePrice,
		FnbCredit:      arg.FnbCredit,
		EntryCount:     arg.EntryCount,
		IsGenderStrict: arg.IsGenderStrict,
	}
	s.categories[cat.ID] = cat
	return cat, nil
}

func (s *memStore) CountConfirmedFemaleStagByEvent(_ context.Context, eventID uuid.UUID) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for _, b := range s.bookings {
		if b.EventID == eventID && b.Status == db.BookingStatusCONFIRMED {
			if cat, ok := s.categories[b.CategoryID]; ok && strings.EqualFold(cat.Name, "STAG_FEMALE") {
				count++
			}
		}
	}
	return count, nil
}

// --- Booking, Ticket & Guest Queries ---

func (s *memStore) CreateBooking(_ context.Context, arg db.CreateBookingParams) (db.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if arg.UpiTransactionID.Valid && arg.UpiTransactionID.String != "" {
		for _, b := range s.bookings {
			if b.UpiTransactionID.Valid && b.UpiTransactionID.String == arg.UpiTransactionID.String {
				return db.Booking{}, pgUniqueErr("idx_bookings_upi_txn_id")
			}
		}
	}
	b := db.Booking{
		ID:               uuid.New(),
		EventID:          arg.EventID,
		CategoryID:       arg.CategoryID,
		ContactPhone:     arg.ContactPhone,
		ContactEmail:     arg.ContactEmail,
		TotalAmount:      arg.TotalAmount,
		UpiTransactionID: arg.UpiTransactionID,
		Status:           arg.Status,
		CreatedAt:        time.Now().UTC(),
	}
	s.bookings[b.ID] = b
	return b, nil
}

func (s *memStore) SubmitBookingUPITransactionID(_ context.Context, arg db.SubmitBookingUPITransactionIDParams) (db.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bookings[arg.ID]
	if !ok || b.Status != db.BookingStatusPENDING {
		return db.Booking{}, pgx.ErrNoRows
	}
	for id, other := range s.bookings {
		if id != arg.ID && other.UpiTransactionID.Valid && other.UpiTransactionID.String == arg.UpiTransactionID.String {
			return db.Booking{}, pgUniqueErr("idx_bookings_upi_txn_id")
		}
	}
	b.UpiTransactionID = arg.UpiTransactionID
	s.bookings[b.ID] = b
	return b, nil
}

func (s *memStore) GetBookingByIDForUpdate(_ context.Context, id uuid.UUID) (db.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bookings[id]
	if !ok {
		return db.Booking{}, pgx.ErrNoRows
	}
	return b, nil
}

func (s *memStore) ApproveBookingAtomic(_ context.Context, arg db.ApproveBookingAtomicParams) (db.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bookings[arg.ID]
	if !ok || b.Status != db.BookingStatusPENDING {
		return db.Booking{}, pgx.ErrNoRows
	}
	b.Status = db.BookingStatusCONFIRMED
	b.VerifiedBy = arg.VerifiedBy
	b.VerifiedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	b.RejectionReason = pgtype.Text{Valid: false}
	s.bookings[b.ID] = b
	return b, nil
}

func (s *memStore) RejectBookingAtomic(_ context.Context, arg db.RejectBookingAtomicParams) (db.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bookings[arg.ID]
	if !ok || b.Status != db.BookingStatusPENDING {
		return db.Booking{}, pgx.ErrNoRows
	}
	b.Status = db.BookingStatusFAILED
	b.VerifiedBy = arg.VerifiedBy
	b.VerifiedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	b.RejectionReason = arg.RejectionReason
	s.bookings[b.ID] = b
	return b, nil
}

func (s *memStore) ListBookingsForVerification(_ context.Context, arg db.ListBookingsForVerificationParams) ([]db.ListBookingsForVerificationRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.ListBookingsForVerificationRow, 0)
	for _, b := range s.bookings {
		if b.Status != arg.Status {
			continue
		}
		if arg.EventID != uuid.Nil && b.EventID != arg.EventID {
			continue
		}
		cat := s.categories[b.CategoryID]
		ev := s.events[b.EventID]
		var tk db.Ticket
		for _, t := range s.tickets {
			if t.BookingID == b.ID {
				tk = t
				break
			}
		}
		out = append(out, db.ListBookingsForVerificationRow{
			ID:               b.ID,
			EventID:          b.EventID,
			CategoryID:       b.CategoryID,
			ContactPhone:     b.ContactPhone,
			ContactEmail:     b.ContactEmail,
			TotalAmount:      b.TotalAmount,
			UpiTransactionID: b.UpiTransactionID,
			Status:           b.Status,
			VerifiedBy:       b.VerifiedBy,
			VerifiedAt:       b.VerifiedAt,
			RejectionReason:  b.RejectionReason,
			CreatedAt:        b.CreatedAt,
			CategoryName:     cat.Name,
			CategoryTier:     cat.Category,
			TierBasePrice:    cat.BasePrice,
			TierFnbCredit:    cat.FnbCredit,
			EntryCount:       cat.EntryCount,
			EventTitle:       ev.Title,
			TicketID:         tk.ID,
			BarCode:          tk.BarCode,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	start := int(arg.Offset)
	if start >= len(out) {
		return []db.ListBookingsForVerificationRow{}, nil
	}
	end := start + int(arg.Limit)
	if end > len(out) {
		end = len(out)
	}
	return out[start:end], nil
}

func (s *memStore) CreateTicket(_ context.Context, arg db.CreateTicketParams) (db.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tickets {
		if t.BarCode == arg.BarCode {
			return db.Ticket{}, pgUniqueErr("tickets_bar_code_key")
		}
	}
	t := db.Ticket{
		ID:                 arg.ID,
		BookingID:          arg.BookingID,
		EventID:            arg.EventID,
		QrTokenDigest:      arg.QrTokenDigest,
		BarCode:            arg.BarCode,
		FoodBalance:        arg.FoodBalance,
		InitialFoodBalance: arg.InitialFoodBalance,
		IsCheckedIn:        false,
	}
	s.tickets[t.ID] = t
	return t, nil
}

func (s *memStore) CreateTicketGuest(_ context.Context, arg db.CreateTicketGuestParams) (db.TicketGuest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := db.TicketGuest{
		ID:               uuid.New(),
		TicketID:         arg.TicketID,
		Name:             arg.Name,
		Age:              arg.Age,
		Gender:           arg.Gender,
		IsPrimaryContact: arg.IsPrimaryContact,
		FreeShotRedeemed: false,
	}
	s.guests[g.ID] = g
	return g, nil
}

func (s *memStore) GetTicketByBookingID(_ context.Context, bookingID uuid.UUID) (db.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tickets {
		if t.BookingID == bookingID {
			return t, nil
		}
	}
	return db.Ticket{}, pgx.ErrNoRows
}

func (s *memStore) GetTicketDetailsByID(_ context.Context, id uuid.UUID) (db.GetTicketDetailsByIDRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	if !ok {
		return db.GetTicketDetailsByIDRow{}, pgx.ErrNoRows
	}
	b := s.bookings[t.BookingID]
	cat := s.categories[b.CategoryID]
	ev := s.events[t.EventID]
	return db.GetTicketDetailsByIDRow{
		ID:                 t.ID,
		BookingID:          t.BookingID,
		EventID:            t.EventID,
		QrTokenDigest:      t.QrTokenDigest,
		BarCode:            t.BarCode,
		FoodBalance:        t.FoodBalance,
		InitialFoodBalance: t.InitialFoodBalance,
		IsCheckedIn:        t.IsCheckedIn,
		CheckedInAt:        t.CheckedInAt,
		BookingStatus:      b.Status,
		TotalAmount:        b.TotalAmount,
		UpiTransactionID:   b.UpiTransactionID,
		ContactPhone:       b.ContactPhone,
		ContactEmail:       b.ContactEmail,
		CategoryName:       cat.Name,
		CategoryTier:       cat.Category,
		EntryCount:         cat.EntryCount,
		EventTitle:         ev.Title,
		EventStartsAt:      ev.StartsAt,
		EventEndsAt:        ev.EndsAt,
		EventLocation:      ev.Location,
	}, nil
}

func (s *memStore) ListGuestsByTicketID(_ context.Context, ticketID uuid.UUID) ([]db.TicketGuest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.TicketGuest, 0)
	for _, g := range s.guests {
		if g.TicketID == ticketID {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsPrimaryContact != out[j].IsPrimaryContact {
			return out[i].IsPrimaryContact
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// --- Gate Queries ---

func (s *memStore) GetTicketDetailsByQRDigest(_ context.Context, qrTokenDigest string) (db.GetTicketDetailsByQRDigestRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tickets {
		if t.QrTokenDigest == qrTokenDigest {
			b := s.bookings[t.BookingID]
			cat := s.categories[b.CategoryID]
			ev := s.events[t.EventID]
			return db.GetTicketDetailsByQRDigestRow{
				ID:                 t.ID,
				BookingID:          t.BookingID,
				EventID:            t.EventID,
				QrTokenDigest:      t.QrTokenDigest,
				BarCode:            t.BarCode,
				FoodBalance:        t.FoodBalance,
				InitialFoodBalance: t.InitialFoodBalance,
				IsCheckedIn:        t.IsCheckedIn,
				CheckedInAt:        t.CheckedInAt,
				BookingStatus:      b.Status,
				CategoryName:       cat.Name,
				CategoryTier:       cat.Category,
				EntryCount:         cat.EntryCount,
				EventTitle:         ev.Title,
				EventStartsAt:      ev.StartsAt,
				EventEndsAt:        ev.EndsAt,
				EventLocation:      ev.Location,
			}, nil
		}
	}
	return db.GetTicketDetailsByQRDigestRow{}, pgx.ErrNoRows
}

func (s *memStore) CheckInTicketAtomic(_ context.Context, id uuid.UUID) (db.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	if !ok || t.IsCheckedIn {
		return db.Ticket{}, pgx.ErrNoRows
	}
	t.IsCheckedIn = true
	t.CheckedInAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	s.tickets[t.ID] = t
	return t, nil
}

// --- Bar Menu, Wallet & Audit Queries ---

func (s *memStore) ListActiveMenuItems(_ context.Context) ([]db.MenuItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.MenuItem, 0)
	for _, m := range s.menuItems {
		if m.IsActive {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *memStore) CreateMenuItem(_ context.Context, arg db.CreateMenuItemParams) (db.MenuItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := db.MenuItem{
		ID:        uuid.New(),
		Name:      arg.Name,
		Category:  arg.Category,
		UnitPrice: arg.UnitPrice,
		IsActive:  true,
	}
	s.menuItems[m.ID] = m
	return m, nil
}

func (s *memStore) DeactivateMenuItem(_ context.Context, id uuid.UUID) (db.MenuItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.menuItems[id]
	if !ok || !m.IsActive {
		return db.MenuItem{}, pgx.ErrNoRows
	}
	m.IsActive = false
	s.menuItems[m.ID] = m
	return m, nil
}

func (s *memStore) GetActiveMenuItemsByIDs(_ context.Context, ids []uuid.UUID) ([]db.MenuItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.MenuItem, 0, len(ids))
	for _, id := range ids {
		if m, ok := s.menuItems[id]; ok && m.IsActive {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *memStore) GetTicketDetailsByBarCode(_ context.Context, barCode string) (db.GetTicketDetailsByBarCodeRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tickets {
		if t.BarCode == barCode {
			b := s.bookings[t.BookingID]
			cat := s.categories[b.CategoryID]
			ev := s.events[t.EventID]
			return db.GetTicketDetailsByBarCodeRow{
				ID:                 t.ID,
				BookingID:          t.BookingID,
				EventID:            t.EventID,
				BarCode:            t.BarCode,
				FoodBalance:        t.FoodBalance,
				InitialFoodBalance: t.InitialFoodBalance,
				IsCheckedIn:        t.IsCheckedIn,
				CheckedInAt:        t.CheckedInAt,
				BookingStatus:      b.Status,
				CategoryName:       cat.Name,
				CategoryTier:       cat.Category,
				EntryCount:         cat.EntryCount,
				EventTitle:         ev.Title,
				EventStartsAt:      ev.StartsAt,
				EventEndsAt:        ev.EndsAt,
			}, nil
		}
	}
	return db.GetTicketDetailsByBarCodeRow{}, pgx.ErrNoRows
}

func (s *memStore) GetTicketForUpdateByBarCode(_ context.Context, barCode string) (db.GetTicketForUpdateByBarCodeRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tickets {
		if t.BarCode == barCode {
			b := s.bookings[t.BookingID]
			ev := s.events[t.EventID]
			return db.GetTicketForUpdateByBarCodeRow{
				ID:                 t.ID,
				BookingID:          t.BookingID,
				EventID:            t.EventID,
				BarCode:            t.BarCode,
				FoodBalance:        t.FoodBalance,
				InitialFoodBalance: t.InitialFoodBalance,
				IsCheckedIn:        t.IsCheckedIn,
				BookingStatus:      b.Status,
				EventEndsAt:        ev.EndsAt,
			}, nil
		}
	}
	return db.GetTicketForUpdateByBarCodeRow{}, pgx.ErrNoRows
}

func (s *memStore) DeductTicketFoodBalanceAtomic(_ context.Context, arg db.DeductTicketFoodBalanceAtomicParams) (db.DeductTicketFoodBalanceAtomicRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[arg.ID]
	if !ok || t.FoodBalance < arg.FoodBalance {
		return db.DeductTicketFoodBalanceAtomicRow{}, pgx.ErrNoRows
	}
	t.FoodBalance -= arg.FoodBalance
	s.tickets[t.ID] = t
	return db.DeductTicketFoodBalanceAtomicRow{
		ID:                 t.ID,
		FoodBalance:        t.FoodBalance,
		InitialFoodBalance: t.InitialFoodBalance,
	}, nil
}

func (s *memStore) RedeemGuestFreeShotAtomic(_ context.Context, arg db.RedeemGuestFreeShotAtomicParams) (db.RedeemGuestFreeShotAtomicRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.guests[arg.ID]
	if !ok || g.TicketID != arg.TicketID || g.Gender != db.GenderTypeFEMALE || g.FreeShotRedeemed {
		return db.RedeemGuestFreeShotAtomicRow{}, pgx.ErrNoRows
	}
	g.FreeShotRedeemed = true
	s.guests[g.ID] = g
	return db.RedeemGuestFreeShotAtomicRow{
		ID:               g.ID,
		TicketID:         g.TicketID,
		Name:             g.Name,
		Gender:           g.Gender,
		FreeShotRedeemed: g.FreeShotRedeemed,
	}, nil
}

func (s *memStore) GetBarTransactionByIdempotencyKey(_ context.Context, key pgtype.Text) (db.BarTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tx := range s.barTxs {
		if tx.IdempotencyKey.Valid && key.Valid && tx.IdempotencyKey.String == key.String {
			return tx, nil
		}
	}
	return db.BarTransaction{}, pgx.ErrNoRows
}

func (s *memStore) CreateBarTransaction(_ context.Context, arg db.CreateBarTransactionParams) (db.BarTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if arg.IdempotencyKey.Valid && arg.IdempotencyKey.String != "" {
		for _, existing := range s.barTxs {
			if existing.IdempotencyKey.Valid && existing.IdempotencyKey.String == arg.IdempotencyKey.String {
				return db.BarTransaction{}, pgUniqueErr("idx_bar_transactions_idempotency")
			}
		}
	}
	tx := db.BarTransaction{
		ID:             uuid.New(),
		TicketID:       arg.TicketID,
		BartenderID:    arg.BartenderID,
		IdempotencyKey: arg.IdempotencyKey,
		TotalBill:      arg.TotalBill,
		WalletDeducted: arg.WalletDeducted,
		UpiDeltaPaid:   arg.UpiDeltaPaid,
		BalanceBefore:  arg.BalanceBefore,
		BalanceAfter:   arg.BalanceAfter,
		CreatedAt:      time.Now().UTC(),
	}
	s.barTxs[tx.ID] = tx
	s.barTxOrder = append(s.barTxOrder, tx.ID)
	return tx, nil
}

func (s *memStore) CreateTransactionLineItem(_ context.Context, arg db.CreateTransactionLineItemParams) (db.TransactionLineItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	li := db.TransactionLineItem{
		ID:              uuid.New(),
		TransactionID:   arg.TransactionID,
		MenuItemID:      arg.MenuItemID,
		Quantity:        arg.Quantity,
		UnitPriceAtSale: arg.UnitPriceAtSale,
	}
	s.lineItems = append(s.lineItems, li)
	return li, nil
}

func (s *memStore) ListBarTransactionsByTicketID(_ context.Context, arg db.ListBarTransactionsByTicketIDParams) ([]db.ListBarTransactionsByTicketIDRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.ListBarTransactionsByTicketIDRow, 0)
	for i := len(s.barTxOrder) - 1; i >= 0; i-- {
		tx := s.barTxs[s.barTxOrder[i]]
		if tx.TicketID != arg.TicketID {
			continue
		}
		uname := ""
		if st, ok := s.staff[tx.BartenderID]; ok {
			uname = st.Username
		}
		out = append(out, db.ListBarTransactionsByTicketIDRow{
			ID:                tx.ID,
			TicketID:          tx.TicketID,
			BartenderID:       tx.BartenderID,
			BartenderUsername: uname,
			IdempotencyKey:    tx.IdempotencyKey,
			TotalBill:         tx.TotalBill,
			WalletDeducted:    tx.WalletDeducted,
			UpiDeltaPaid:      tx.UpiDeltaPaid,
			BalanceBefore:     tx.BalanceBefore,
			BalanceAfter:      tx.BalanceAfter,
			CreatedAt:         tx.CreatedAt,
		})
		if int32(len(out)) >= arg.Limit {
			break
		}
	}
	return out, nil
}

func (s *memStore) ListLineItemsByTransactionIDs(_ context.Context, txIDs []uuid.UUID) ([]db.ListLineItemsByTransactionIDsRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := make(map[uuid.UUID]struct{}, len(txIDs))
	for _, id := range txIDs {
		wanted[id] = struct{}{}
	}
	out := make([]db.ListLineItemsByTransactionIDsRow, 0)
	for _, li := range s.lineItems {
		if _, ok := wanted[li.TransactionID]; !ok {
			continue
		}
		name := ""
		if m, ok := s.menuItems[li.MenuItemID]; ok {
			name = m.Name
		}
		out = append(out, db.ListLineItemsByTransactionIDsRow{
			ID:              li.ID,
			TransactionID:   li.TransactionID,
			MenuItemID:      li.MenuItemID,
			MenuItemName:    name,
			Quantity:        li.Quantity,
			UnitPriceAtSale: li.UnitPriceAtSale,
		})
	}
	return out, nil
}

// --- Analytics Queries ---

func (s *memStore) GetEventBookingStats(_ context.Context, eventID uuid.UUID) (db.GetEventBookingStatsRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var row db.GetEventBookingStatsRow
	for _, b := range s.bookings {
		if b.EventID != eventID || b.Status != db.BookingStatusCONFIRMED {
			continue
		}
		row.ConfirmedBookings++
		row.TicketRevenue += int64(b.TotalAmount)
		for _, t := range s.tickets {
			if t.BookingID == b.ID {
				row.TotalTickets++
				row.TotalFnbAllocated += int64(t.InitialFoodBalance)
				row.TotalFnbRemaining += int64(t.FoodBalance)
				row.TotalFnbRedeemed += int64(t.InitialFoodBalance - t.FoodBalance)
				if t.IsCheckedIn {
					row.CheckedInTickets++
				}
			}
		}
	}
	return row, nil
}

func (s *memStore) GetEventBarRevenueStats(_ context.Context, eventID uuid.UUID) (db.GetEventBarRevenueStatsRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var row db.GetEventBarRevenueStatsRow
	for _, tx := range s.barTxs {
		t, ok := s.tickets[tx.TicketID]
		if !ok || t.EventID != eventID {
			continue
		}
		row.TotalTransactions++
		row.TotalBarBill += int64(tx.TotalBill)
		row.TotalWalletDeducted += int64(tx.WalletDeducted)
		row.TotalUpiDelta += int64(tx.UpiDeltaPaid)
	}
	return row, nil
}

func (s *memStore) GetEventGenderStats(_ context.Context, eventID uuid.UUID) (db.GetEventGenderStatsRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var row db.GetEventGenderStatsRow
	for _, g := range s.guests {
		t, ok := s.tickets[g.TicketID]
		if !ok || t.EventID != eventID {
			continue
		}
		if b, ok := s.bookings[t.BookingID]; !ok || b.Status != db.BookingStatusCONFIRMED {
			continue
		}
		row.TotalGuests++
		switch g.Gender {
		case db.GenderTypeMALE:
			row.MaleGuests++
		case db.GenderTypeFEMALE:
			row.FemaleGuests++
		case db.GenderTypeOTHER:
			row.OtherGuests++
		}
		if g.FreeShotRedeemed {
			row.FreeShotsRedeemed++
		}
	}
	return row, nil
}

// --- Rate Limit Queries ---

func (s *memStore) UpsertRateLimitHit(_ context.Context, arg db.UpsertRateLimitHitParams) (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := arg.Identifier + "|" + arg.Endpoint + "|" + arg.WindowStart.UTC().Format(time.RFC3339)
	s.rateLimits[key]++
	return s.rateLimits[key], nil
}

func (s *memStore) DeleteExpiredRateLimits(_ context.Context, _ time.Time) error { return nil }

// --- Test Environment Harness ---

type testEnv struct {
	t           *testing.T
	cfg         *config.Config
	store       *memStore
	notifier    *recordingNotifier
	roleCache   *cache.StaffRoleCache
	qrSigner    *auth.QRSigner
	router      http.Handler
	adminID     uuid.UUID
	scannerID   uuid.UUID
	bartenderID uuid.UUID
	eventID     uuid.UUID
	coupleCatID uuid.UUID
	femaleCatID uuid.UUID
	maleCatID   uuid.UUID
	drinkID     uuid.UUID
	foodID      uuid.UUID
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	cfg := &config.Config{
		Env:                     "test",
		Port:                    "8080",
		AppBaseURL:              "https://venu.test",
		DBQueryTimeout:          3 * time.Second,
		AllowedOrigins:          []string{"https://venu.test"},
		CookieSecure:            false,
		StaffCacheTTL:           30 * time.Second,
		JWTAccessSecret:         "test-access-secret-32-bytes-minimum!!",
		JWTRefreshSecret:        "test-refresh-secret-32-bytes-minimum!",
		JWTIssuer:               "venu-api",
		JWTAudience:             "venu-staff",
		QRSigningSecret:         "test-qr-signing-secret-32-bytes-min!",
		AccessTokenTTL:          15 * time.Minute,
		RefreshTokenTTL:         24 * time.Hour,
		TicketQRTTL:             24 * time.Hour,
		UPIVPA:                  "venu.club@upi",
		UPIMerchantName:         "Venu Nightclub",
		UPIQRImageURL:           "https://venu.test/static/upi-qr.png",
		First99FemaleLimit:      99,
		MinGuestAge:             21,
		AuthRateLimitMax:        100,
		AuthRateLimitWindow:     time.Minute,
		CheckoutRateLimitMax:    100,
		CheckoutRateLimitWindow: time.Minute,
		ScanRateLimitMax:        100,
		ScanRateLimitWindow:     time.Minute,
		BarRateLimitMax:         100,
		BarRateLimitWindow:      time.Minute,
		RequestTimeout:          10 * time.Second,
	}

	ms := newMemStore()
	notifier := &recordingNotifier{}
	roleCache := cache.NewStaffRoleCache(cfg.StaffCacheTTL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tokenMgr := auth.NewTokenManager(
		cfg.JWTAccessSecret, cfg.JWTRefreshSecret,
		cfg.JWTAudience, cfg.JWTIssuer,
		cfg.AccessTokenTTL, cfg.RefreshTokenTTL,
	)
	qrSigner := auth.NewQRSigner(cfg.QRSigningSecret)
	repo := repositories.NewTicketingRepositoryWithQuerier(ms, ms.ping, ms.withTx, cfg.DBQueryTimeout)
	limiter := ratelimit.NewPostgresRateLimiter(ms, cfg.DBQueryTimeout, logger)

	ticketingSvc := services.NewTicketingService(cfg, repo, tokenMgr, qrSigner, roleCache, notifier, logger)
	adminSvc := services.NewAdminService(repo, roleCache, notifier, cfg.AppBaseURL, logger)
	gateSvc := services.NewGateService(repo, qrSigner, logger)
	barSvc := services.NewBarService(repo, logger)

	router := server.NewRouter(server.Deps{
		Cfg:       cfg,
		Handler:   handlers.NewTicketingHandler(cfg, ticketingSvc, tokenMgr, logger),
		AdminHdlr: handlers.NewAdminHandler(adminSvc, logger),
		GateHdlr:  handlers.NewGateHandler(gateSvc, logger),
		BarHdlr:   handlers.NewBarHandler(barSvc, logger),
		RBAC:      middleware.NewRBAC(tokenMgr, ms, roleCache, cfg.DBQueryTimeout, cfg.AllowedOrigins),
		Limiter:   limiter,
	})

	env := &testEnv{
		t:         t,
		cfg:       cfg,
		store:     ms,
		notifier:  notifier,
		roleCache: roleCache,
		qrSigner:  qrSigner,
		router:    router,
	}
	env.seedFixtures()
	return env
}

func (e *testEnv) seedFixtures() {
	e.t.Helper()
	ctx := context.Background()

	seedStaff := func(username, password string, role db.StaffRole) uuid.UUID {
		hash, err := auth.HashPassword(password)
		if err != nil {
			e.t.Fatalf("hash seed staff %s: %v", username, err)
		}
		st, err := e.store.CreateStaff(ctx, db.CreateStaffParams{
			Username:     username,
			PasswordHash: hash,
			Role:         role,
		})
		if err != nil {
			e.t.Fatalf("create seed staff %s: %v", username, err)
		}
		return st.ID
	}

	e.adminID = seedStaff("admin_user", "AdminPass!123", db.StaffRoleADMIN)
	e.scannerID = seedStaff("gate_user", "GatePass!123", db.StaffRoleSCANNER)
	e.bartenderID = seedStaff("bar_user", "BarPass!123", db.StaffRoleBARTENDER)

	now := time.Now().UTC()
	ev, _ := e.store.CreateEvent(ctx, db.CreateEventParams{
		Title:    "Saturday Noir",
		StartsAt: now.Add(-1 * time.Hour),
		EndsAt:   now.Add(6 * time.Hour),
		Location: "The Velvet Room, Bengaluru",
		Status:   db.EventStatusONSALE,
	})
	e.eventID = ev.ID

	coupleCat, _ := e.store.CreateTicketCategory(ctx, db.CreateTicketCategoryParams{
		EventID:        ev.ID,
		Name:           "COUPLE",
		Category:       pgtype.Text{String: "MAIN_FLOOR", Valid: true},
		BasePrice:      3000,
		FnbCredit:      2000,
		EntryCount:     2,
		IsGenderStrict: true,
	})
	e.coupleCatID = coupleCat.ID

	femaleCat, _ := e.store.CreateTicketCategory(ctx, db.CreateTicketCategoryParams{
		EventID:        ev.ID,
		Name:           "STAG_FEMALE",
		BasePrice:      1500,
		FnbCredit:      1000,
		EntryCount:     1,
		IsGenderStrict: false,
	})
	e.femaleCatID = femaleCat.ID

	maleCat, _ := e.store.CreateTicketCategory(ctx, db.CreateTicketCategoryParams{
		EventID:        ev.ID,
		Name:           "STAG_MALE",
		BasePrice:      2000,
		FnbCredit:      1000,
		EntryCount:     1,
		IsGenderStrict: false,
	})
	e.maleCatID = maleCat.ID

	drink, _ := e.store.CreateMenuItem(ctx, db.CreateMenuItemParams{
		Name:      "Smoked Negroni",
		Category:  db.MenuCategoryDRINK,
		UnitPrice: 750,
	})
	e.drinkID = drink.ID

	food, _ := e.store.CreateMenuItem(ctx, db.CreateMenuItemParams{
		Name:      "Truffle Fries",
		Category:  db.MenuCategoryFOOD,
		UnitPrice: 500,
	})
	e.foodID = food.ID
}

func (e *testEnv) doRequest(method, path string, body any, headers map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "10.0.0.15:54321"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) loginAs(username, password string) (string, []*http.Cookie) {
	e.t.Helper()
	rec := e.doRequest(http.MethodPost, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("loginAs(%s) failed with status %d: %s", username, rec.Code, rec.Body.String())
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	decodeBody(e.t, rec, &payload)
	return payload.AccessToken, rec.Result().Cookies()
}

func bearerHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode JSON (status %d): %v | raw=%s", rec.Code, err, rec.Body.String())
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("got HTTP %d, want %d | body=%s", rec.Code, want, rec.Body.String())
	}
}

var errSimulatedDBDown = errors.New("connection refused")
