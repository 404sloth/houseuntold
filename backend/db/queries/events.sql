-- name: GetEventByID :one
SELECT id, title, starts_at, ends_at, location, status, created_at
FROM events
WHERE id = $1;

-- name: GetEventByIDForUpdate :one
SELECT id, title, starts_at, ends_at, location, status, created_at
FROM events
WHERE id = $1
FOR UPDATE;

-- name: CreateEvent :one
INSERT INTO events (title, starts_at, ends_at, location, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, title, starts_at, ends_at, location, status, created_at;

-- name: ListEventCategories :many
SELECT id, event_id, name, category, base_price, fnb_credit, entry_count, is_gender_strict
FROM ticket_categories
WHERE event_id = $1
ORDER BY base_price ASC, entry_count ASC;

-- name: GetCategoryByID :one
SELECT id, event_id, name, category, base_price, fnb_credit, entry_count, is_gender_strict
FROM ticket_categories
WHERE id = $1;

-- name: CreateTicketCategory :one
INSERT INTO ticket_categories (event_id, name, category, base_price, fnb_credit, entry_count, is_gender_strict)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, event_id, name, category, base_price, fnb_credit, entry_count, is_gender_strict;

-- name: CountConfirmedFemaleStagByEvent :one
SELECT COUNT(b.id)::bigint AS confirmed_count
FROM bookings b
INNER JOIN ticket_categories tc ON tc.id = b.category_id
WHERE b.event_id = $1
  AND b.status = 'CONFIRMED'
  AND UPPER(tc.name) = 'STAG_FEMALE';
