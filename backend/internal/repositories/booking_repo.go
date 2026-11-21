package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/db"
)

func (r *TicketingRepository) GetTicketDetailsByID(ctx context.Context, id uuid.UUID) (db.GetTicketDetailsByIDRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.GetTicketDetailsByID(tctx, id)
	if err != nil {
		return db.GetTicketDetailsByIDRow{}, fmt.Errorf("repo_get_ticket_details_by_id: %w", err)
	}
	return row, nil
}

func (r *TicketingRepository) ListGuestsByTicketID(ctx context.Context, ticketID uuid.UUID) ([]db.TicketGuest, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	guests, err := r.queries.ListGuestsByTicketID(tctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("repo_list_guests_by_ticket_id: %w", err)
	}
	return guests, nil
}

func (r *TicketingRepository) SubmitBookingUPITransactionID(ctx context.Context, arg db.SubmitBookingUPITransactionIDParams) (db.Booking, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	booking, err := r.queries.SubmitBookingUPITransactionID(tctx, arg)
	if err != nil {
		return db.Booking{}, fmt.Errorf("repo_submit_booking_upi_txn: %w", err)
	}
	return booking, nil
}

func (r *TicketingRepository) ListBookingsForVerification(ctx context.Context, arg db.ListBookingsForVerificationParams) ([]db.ListBookingsForVerificationRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	rows, err := r.queries.ListBookingsForVerification(tctx, arg)
	if err != nil {
		return nil, fmt.Errorf("repo_list_bookings_for_verification: %w", err)
	}
	return rows, nil
}
