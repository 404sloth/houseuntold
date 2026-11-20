-- name: GetAdminOverviewStats :one
SELECT
    (SELECT COUNT(*)::int FROM events WHERE status IN ('ON_SALE', 'ALMOST_FULL')) AS active_events,
    (SELECT COUNT(*)::int FROM seat_inventory WHERE status = 'SOLD') AS seats_booked,
    (SELECT COUNT(*)::int FROM seat_inventory) AS total_seats_capacity,
    (SELECT COALESCE(SUM(total_payable), 0)::bigint FROM orders WHERE status = 'CONFIRMED') AS gross_ticket_revenue,
    (SELECT COUNT(*)::int FROM order_seats WHERE admitted = TRUE) AS gate_admitted_today;
