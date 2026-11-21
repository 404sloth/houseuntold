package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/db"
)

func (r *TicketingRepository) GetStaffByUsername(ctx context.Context, username string) (db.StaffAccount, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	staff, err := r.queries.GetStaffByUsername(tctx, username)
	if err != nil {
		return db.StaffAccount{}, fmt.Errorf("repo_get_staff_by_username: %w", err)
	}
	return staff, nil
}

func (r *TicketingRepository) GetStaffByID(ctx context.Context, id uuid.UUID) (db.StaffAccount, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	staff, err := r.queries.GetStaffByID(tctx, id)
	if err != nil {
		return db.StaffAccount{}, fmt.Errorf("repo_get_staff_by_id: %w", err)
	}
	return staff, nil
}

func (r *TicketingRepository) CreateStaff(ctx context.Context, arg db.CreateStaffParams) (db.StaffAccount, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	staff, err := r.queries.CreateStaff(tctx, arg)
	if err != nil {
		return db.StaffAccount{}, fmt.Errorf("repo_create_staff: %w", err)
	}
	return staff, nil
}

func (r *TicketingRepository) UpdateStaffPassword(ctx context.Context, arg db.UpdateStaffPasswordParams) (db.UpdateStaffPasswordRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.UpdateStaffPassword(tctx, arg)
	if err != nil {
		return db.UpdateStaffPasswordRow{}, fmt.Errorf("repo_update_staff_password: %w", err)
	}
	return row, nil
}

func (r *TicketingRepository) DeactivateStaff(ctx context.Context, id uuid.UUID) (db.DeactivateStaffRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.DeactivateStaff(tctx, id)
	if err != nil {
		return db.DeactivateStaffRow{}, fmt.Errorf("repo_deactivate_staff: %w", err)
	}
	return row, nil
}
