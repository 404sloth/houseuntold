package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/db"
)

func (r *TicketingRepository) GetEventByID(ctx context.Context, id uuid.UUID) (db.Event, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	event, err := r.queries.GetEventByID(tctx, id)
	if err != nil {
		return db.Event{}, fmt.Errorf("repo_get_event_by_id: %w", err)
	}
	return event, nil
}

func (r *TicketingRepository) CreateEvent(ctx context.Context, arg db.CreateEventParams) (db.Event, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	event, err := r.queries.CreateEvent(tctx, arg)
	if err != nil {
		return db.Event{}, fmt.Errorf("repo_create_event: %w", err)
	}
	return event, nil
}

func (r *TicketingRepository) ListEventCategories(ctx context.Context, eventID uuid.UUID) ([]db.TicketCategory, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	cats, err := r.queries.ListEventCategories(tctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("repo_list_event_categories: %w", err)
	}
	return cats, nil
}

func (r *TicketingRepository) GetCategoryByID(ctx context.Context, id uuid.UUID) (db.TicketCategory, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	cat, err := r.queries.GetCategoryByID(tctx, id)
	if err != nil {
		return db.TicketCategory{}, fmt.Errorf("repo_get_category_by_id: %w", err)
	}
	return cat, nil
}

func (r *TicketingRepository) CreateTicketCategory(ctx context.Context, arg db.CreateTicketCategoryParams) (db.TicketCategory, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	cat, err := r.queries.CreateTicketCategory(tctx, arg)
	if err != nil {
		return db.TicketCategory{}, fmt.Errorf("repo_create_ticket_category: %w", err)
	}
	return cat, nil
}

func (r *TicketingRepository) CountConfirmedFemaleStagByEvent(ctx context.Context, eventID uuid.UUID) (int64, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	count, err := r.queries.CountConfirmedFemaleStagByEvent(tctx, eventID)
	if err != nil {
		return 0, fmt.Errorf("repo_count_confirmed_female_stag: %w", err)
	}
	return count, nil
}

func (r *TicketingRepository) GetEventBookingStats(ctx context.Context, eventID uuid.UUID) (db.GetEventBookingStatsRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.GetEventBookingStats(tctx, eventID)
	if err != nil {
		return db.GetEventBookingStatsRow{}, fmt.Errorf("repo_get_event_booking_stats: %w", err)
	}
	return row, nil
}

func (r *TicketingRepository) GetEventBarRevenueStats(ctx context.Context, eventID uuid.UUID) (db.GetEventBarRevenueStatsRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.GetEventBarRevenueStats(tctx, eventID)
	if err != nil {
		return db.GetEventBarRevenueStatsRow{}, fmt.Errorf("repo_get_event_bar_revenue_stats: %w", err)
	}
	return row, nil
}

func (r *TicketingRepository) GetEventGenderStats(ctx context.Context, eventID uuid.UUID) (db.GetEventGenderStatsRow, error) {
	tctx, cancel := r.withTimeout(ctx)
	defer cancel()

	row, err := r.queries.GetEventGenderStats(tctx, eventID)
	if err != nil {
		return db.GetEventGenderStatsRow{}, fmt.Errorf("repo_get_event_gender_stats: %w", err)
	}
	return row, nil
}
