package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/db"
)

func (r *TicketingRepository) GetTicketDetailsByQRDigest(ctx context.Context, digest string) (db.GetTicketDetailsByQRDigestRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.GetTicketDetailsByQRDigest(tctx, digest)
	if err != nil {
		return db.GetTicketDetailsByQRDigestRow{}, fmt.Errorf("repo_get_ticket_by_qr_digest: %w", err)
	}
	return row, nil
}

func (r *TicketingRepository) CheckInTicketAtomic(ctx context.Context, ticketID uuid.UUID) (db.Ticket, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	ticket, err := r.queries.CheckInTicketAtomic(tctx, ticketID)
	if err != nil {
		return db.Ticket{}, fmt.Errorf("repo_check_in_ticket_atomic: %w", err)
	}
	return ticket, nil
}
