-- name: GetEventBookingStats :one
SELECT
    COUNT(b.id)::bigint AS confirmed_bookings,
    COALESCE(SUM(b.total_amount), 0)::bigint AS ticket_revenue,
    COALESCE(SUM(t.initial_food_balance), 0)::bigint AS total_fnb_allocated,
    COALESCE(SUM(t.initial_food_balance - t.food_balance), 0)::bigint AS total_fnb_redeemed,
    COALESCE(SUM(t.food_balance), 0)::bigint AS total_fnb_remaining,
    COUNT(t.id) FILTER (WHERE t.is_checked_in = true)::bigint AS checked_in_tickets,
    COUNT(t.id)::bigint AS total_tickets
FROM bookings b
INNER JOIN tickets t ON t.booking_id = b.id
WHERE b.event_id = $1 AND b.status = 'CONFIRMED';

-- name: GetEventBarRevenueStats :one
SELECT
    COUNT(bt.id)::bigint AS total_transactions,
    COALESCE(SUM(bt.total_bill), 0)::bigint AS total_bar_bill,
    COALESCE(SUM(bt.wallet_deducted), 0)::bigint AS total_wallet_deducted,
    COALESCE(SUM(bt.upi_delta_paid), 0)::bigint AS total_upi_delta
FROM bar_transactions bt
INNER JOIN tickets t ON t.id = bt.ticket_id
WHERE t.event_id = $1;

-- name: GetEventGenderStats :one
SELECT
    COUNT(tg.id)::bigint AS total_guests,
    COUNT(tg.id) FILTER (WHERE tg.gender = 'MALE')::bigint AS male_guests,
    COUNT(tg.id) FILTER (WHERE tg.gender = 'FEMALE')::bigint AS female_guests,
    COUNT(tg.id) FILTER (WHERE tg.gender = 'OTHER')::bigint AS other_guests,
    COUNT(tg.id) FILTER (WHERE tg.free_shot_redeemed = true)::bigint AS free_shots_redeemed
FROM ticket_guests tg
INNER JOIN tickets t ON t.id = tg.ticket_id
INNER JOIN bookings b ON b.id = t.booking_id
WHERE t.event_id = $1 AND b.status = 'CONFIRMED';
