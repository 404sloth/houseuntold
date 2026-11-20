-- name: CreateOrder :one
INSERT INTO orders (user_id, event_id, razorpay_order_id, base_amount, convenience_fee, gst_amount, total_payable, contact_email, contact_phone, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'PENDING')
RETURNING id, user_id, event_id, razorpay_order_id, base_amount, convenience_fee, gst_amount, total_payable, contact_email, contact_phone, status, created_at, confirmed_at;

-- name: CreateOrderSeat :exec
INSERT INTO order_seats (order_id, seat_id, attendee_name, attendee_age, attendee_gender)
VALUES ($1, $2, $3, $4, $5);

-- name: ConfirmOrderByRazorpayID :one
UPDATE orders
SET status = 'CONFIRMED',
    confirmed_at = NOW()
WHERE razorpay_order_id = $1
  AND status = 'PENDING'
RETURNING id, user_id, event_id, razorpay_order_id, base_amount, convenience_fee, gst_amount, total_payable, contact_email, contact_phone, status, created_at, confirmed_at;

-- name: CreateTicket :one
INSERT INTO tickets (id, order_id, event_id, user_id, code, bar_code, qr_token_digest, qr_otp_secret, gate_label, food_balance, initial_food_balance, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, order_id, event_id, user_id, code, bar_code, qr_token_digest, qr_otp_secret, gate_label, food_balance, initial_food_balance, checked_in, checked_in_at, checked_in_by, expires_at;

-- name: ListTicketsByUser :many
SELECT t.id, t.order_id, t.event_id, t.user_id, t.code, t.bar_code, t.qr_token_digest, t.qr_otp_secret, t.gate_label, t.food_balance, t.initial_food_balance, t.checked_in, t.checked_in_at, t.checked_in_by, t.expires_at,
       e.title AS event_title, e.category AS event_category, e.venue_name, e.date_label, e.time_label, e.starts_at AS event_starts_at, o.total_payable, o.contact_email, o.contact_phone
FROM tickets t
JOIN events e ON e.id = t.event_id
JOIN orders o ON o.id = t.order_id
WHERE t.user_id = $1
ORDER BY e.starts_at ASC, o.created_at DESC;

-- name: GetTicketByCodeOrDigest :one
SELECT t.id, t.order_id, t.event_id, t.user_id, t.code, t.bar_code, t.qr_token_digest, t.qr_otp_secret, t.gate_label, t.food_balance, t.initial_food_balance, t.checked_in, t.checked_in_at, t.checked_in_by, t.expires_at,
       e.title AS event_title, e.category AS event_category, e.venue_name, e.date_label, e.time_label, e.starts_at AS event_starts_at
FROM tickets t
JOIN events e ON e.id = t.event_id
WHERE lower(t.code) = lower($1)
   OR (t.bar_code <> '' AND lower(t.bar_code) = lower($1))
   OR t.qr_token_digest = $1
   OR t.id::text = $1
LIMIT 1;

-- name: GetTicketForUpdate :one
SELECT id, order_id, event_id, user_id, code, bar_code, qr_token_digest, qr_otp_secret, gate_label, food_balance, initial_food_balance, checked_in, checked_in_at, checked_in_by, expires_at
FROM tickets
WHERE id = $1
FOR UPDATE;

-- name: ListOrderAttendees :many
SELECT os.order_id, os.seat_id, os.attendee_name, os.attendee_age, os.attendee_gender, os.admitted, si.label AS seat_label, sec.tier
FROM order_seats os
JOIN seat_inventory si ON si.id = os.seat_id
JOIN event_sections sec ON sec.id = si.section_id
WHERE os.order_id = $1
ORDER BY si.label ASC;

-- name: MarkOrderSeatAdmitted :exec
UPDATE order_seats
SET admitted = TRUE
WHERE order_id = $1
  AND ($2::uuid IS NULL OR seat_id = $2);

-- name: CountUnadmittedSeats :one
SELECT COUNT(*)::int AS unadmitted_count
FROM order_seats
WHERE order_id = $1
  AND admitted = FALSE;

-- name: MarkTicketCheckedIn :one
UPDATE tickets
SET checked_in = TRUE,
    checked_in_at = NOW(),
    checked_in_by = $2
WHERE id = $1
  AND checked_in = FALSE
RETURNING id, order_id, event_id, user_id, code, bar_code, qr_token_digest, qr_otp_secret, gate_label, food_balance, initial_food_balance, checked_in, checked_in_at, checked_in_by, expires_at;

-- name: InsertGateScanLog :one
INSERT INTO gate_scan_logs (ticket_id, scanner_user_id, ticket_code, status)
VALUES ($1, $2, $3, $4)
RETURNING id, ticket_id, scanner_user_id, ticket_code, status, scanned_at;

-- name: ListRecentScans :many
SELECT gsl.id, gsl.ticket_code, gsl.status, gsl.scanned_at
FROM gate_scan_logs gsl
ORDER BY gsl.scanned_at DESC
LIMIT $1;

-- name: GetTicketByID :one
SELECT id, order_id, event_id, user_id, code, bar_code, qr_token_digest, qr_otp_secret, gate_label, food_balance, initial_food_balance, checked_in, checked_in_at, checked_in_by, expires_at
FROM tickets
WHERE id = $1
LIMIT 1;

-- name: UpdateOrderStatus :one
UPDATE orders
SET status = $2
WHERE id = $1
RETURNING id, user_id, event_id, razorpay_order_id, base_amount, convenience_fee, gst_amount, total_payable, contact_email, contact_phone, status, created_at, confirmed_at;

-- name: ListRecentOrders :many
SELECT id, user_id, event_id, razorpay_order_id, base_amount, convenience_fee, gst_amount, total_payable, contact_email, contact_phone, status, created_at, confirmed_at
FROM orders
ORDER BY created_at DESC
LIMIT $1;
