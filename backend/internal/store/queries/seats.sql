-- name: ListSeatsByEvent :many
SELECT s.id, s.event_id, s.section_id, s.row_label, s.seat_number, s.label, s.pos_x, s.pos_y, s.status, s.held_by_user_id, s.hold_expires_at, sec.tier
FROM seat_inventory s
JOIN event_sections sec ON sec.id = s.section_id
WHERE s.event_id = $1
ORDER BY s.pos_y ASC, s.pos_x ASC;

-- name: CreateSeatInventory :one
INSERT INTO seat_inventory (event_id, section_id, row_label, seat_number, label, pos_x, pos_y, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'AVAILABLE')
RETURNING id, event_id, section_id, row_label, seat_number, label, pos_x, pos_y, status, held_by_user_id, hold_expires_at;

-- name: LockSeatForUpdate :one
SELECT id, event_id, section_id, row_label, seat_number, label, status, held_by_user_id, hold_expires_at
FROM seat_inventory
WHERE id = $1 AND event_id = $2
FOR UPDATE;

-- name: HoldSeat :execrows
UPDATE seat_inventory
SET status = 'HELD',
    held_by_user_id = $2,
    hold_expires_at = $3
WHERE id = $1
  AND (status = 'AVAILABLE' OR (status = 'HELD' AND hold_expires_at < NOW()));

-- name: ConfirmSeat :one
UPDATE seat_inventory
SET status = 'SOLD',
    hold_expires_at = NULL
WHERE id = $1
  AND held_by_user_id = $2
RETURNING id, event_id, section_id, row_label, seat_number, label, pos_x, pos_y, status, held_by_user_id, hold_expires_at;

-- name: ReleaseSeatHold :exec
UPDATE seat_inventory
SET status = 'AVAILABLE',
    held_by_user_id = NULL,
    hold_expires_at = NULL
WHERE id = $1;

-- name: ClearUserHoldsForEvent :exec
UPDATE seat_inventory
SET status = 'AVAILABLE',
    held_by_user_id = NULL,
    hold_expires_at = NULL
WHERE event_id = $1
  AND held_by_user_id = $2
  AND status = 'HELD';

-- name: QuoteSeatsByIDs :many
SELECT s.id, s.section_id, s.label, s.status, s.held_by_user_id, s.hold_expires_at, sec.name AS section_name, sec.tier, sec.section_price, sec.fnb_credit, sec.is_alcoholic, sec.min_age
FROM seat_inventory s
JOIN event_sections sec ON sec.id = s.section_id
WHERE s.event_id = $1
  AND s.id = ANY($2::uuid[]);

-- name: MarkOrderSeatsSold :exec
UPDATE seat_inventory
SET status = 'SOLD',
    hold_expires_at = NULL
WHERE id IN (
    SELECT os.seat_id
    FROM order_seats os
    WHERE os.order_id = $1
);
