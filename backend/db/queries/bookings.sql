-- name: CreateBooking :one
INSERT INTO bookings (event_id, category_id, contact_phone, contact_email, total_amount, upi_transaction_id, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, event_id, category_id, contact_phone, contact_email, total_amount, upi_transaction_id, status, verified_by, verified_at, rejection_reason, created_at;

-- name: SubmitBookingUPITransactionID :one
UPDATE bookings
SET upi_transaction_id = $2
WHERE id = $1 AND status = 'PENDING'
RETURNING id, event_id, category_id, contact_phone, contact_email, total_amount, upi_transaction_id, status, verified_by, verified_at, rejection_reason, created_at;

-- name: GetBookingByIDForUpdate :one
SELECT id, event_id, category_id, contact_phone, contact_email, total_amount, upi_transaction_id, status, verified_by, verified_at, rejection_reason, created_at
FROM bookings
WHERE id = $1
FOR UPDATE;

-- name: ApproveBookingAtomic :one
UPDATE bookings
SET status = 'CONFIRMED',
    verified_by = $2,
    verified_at = NOW(),
    rejection_reason = NULL
WHERE id = $1 AND status = 'PENDING'
RETURNING id, event_id, category_id, contact_phone, contact_email, total_amount, upi_transaction_id, status, verified_by, verified_at, rejection_reason, created_at;

-- name: RejectBookingAtomic :one
UPDATE bookings
SET status = 'FAILED',
    verified_by = $2,
    verified_at = NOW(),
    rejection_reason = $3
WHERE id = $1 AND status = 'PENDING'
RETURNING id, event_id, category_id, contact_phone, contact_email, total_amount, upi_transaction_id, status, verified_by, verified_at, rejection_reason, created_at;

-- name: ListBookingsForVerification :many
SELECT
    b.id,
    b.event_id,
    b.category_id,
    b.contact_phone,
    b.contact_email,
    b.total_amount,
    b.upi_transaction_id,
    b.status,
    b.verified_by,
    b.verified_at,
    b.rejection_reason,
    b.created_at,
    tc.name AS category_name,
    tc.category AS category_tier,
    tc.base_price AS tier_base_price,
    tc.fnb_credit AS tier_fnb_credit,
    tc.entry_count,
    e.title AS event_title,
    t.id AS ticket_id,
    t.bar_code
FROM bookings b
INNER JOIN ticket_categories tc ON tc.id = b.category_id
INNER JOIN events e ON e.id = b.event_id
INNER JOIN tickets t ON t.booking_id = b.id
WHERE b.status = $1
  AND ($2::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR b.event_id = $2)
ORDER BY b.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CreateTicket :one
INSERT INTO tickets (id, booking_id, event_id, qr_token_digest, bar_code, food_balance, initial_food_balance, is_checked_in)
VALUES ($1, $2, $3, $4, $5, $6, $7, false)
RETURNING id, booking_id, event_id, qr_token_digest, bar_code, food_balance, initial_food_balance, is_checked_in, checked_in_at;

-- name: CreateTicketGuest :one
INSERT INTO ticket_guests (ticket_id, name, age, gender, is_primary_contact, free_shot_redeemed)
VALUES ($1, $2, $3, $4, $5, false)
RETURNING id, ticket_id, name, age, gender, is_primary_contact, free_shot_redeemed;

-- name: GetTicketByBookingID :one
SELECT id, booking_id, event_id, qr_token_digest, bar_code, food_balance, initial_food_balance, is_checked_in, checked_in_at
FROM tickets
WHERE booking_id = $1;

-- name: GetTicketDetailsByID :one
SELECT
    t.id,
    t.booking_id,
    t.event_id,
    t.qr_token_digest,
    t.bar_code,
    t.food_balance,
    t.initial_food_balance,
    t.is_checked_in,
    t.checked_in_at,
    b.status AS booking_status,
    b.total_amount,
    b.upi_transaction_id,
    b.contact_phone,
    b.contact_email,
    tc.name AS category_name,
    tc.category AS category_tier,
    tc.entry_count,
    e.title AS event_title,
    e.starts_at AS event_starts_at,
    e.ends_at AS event_ends_at,
    e.location AS event_location
FROM tickets t
INNER JOIN bookings b ON b.id = t.booking_id
INNER JOIN ticket_categories tc ON tc.id = b.category_id
INNER JOIN events e ON e.id = t.event_id
WHERE t.id = $1;

-- name: ListGuestsByTicketID :many
SELECT id, ticket_id, name, age, gender, is_primary_contact, free_shot_redeemed
FROM ticket_guests
WHERE ticket_id = $1
ORDER BY is_primary_contact DESC, name ASC;
