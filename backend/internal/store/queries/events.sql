-- name: ListEvents :many
SELECT id, slug, title, subtitle, category, venue_name, venue_city, venue_address, transit_note, layout_type, layout_json, image_url, date_label, time_label, doors_open, starts_at, status, created_at
FROM events
ORDER BY starts_at ASC;

-- name: GetEventByID :one
SELECT id, slug, title, subtitle, category, venue_name, venue_city, venue_address, transit_note, layout_type, layout_json, image_url, date_label, time_label, doors_open, starts_at, status, created_at
FROM events
WHERE id = $1
LIMIT 1;

-- name: ListEventSections :many
SELECT id, event_id, name, tier, section_price, fnb_credit, is_alcoholic, min_age, description
FROM event_sections
WHERE event_id = $1
ORDER BY section_price ASC;

-- name: CreateEvent :one
INSERT INTO events (slug, title, subtitle, category, venue_name, venue_city, venue_address, transit_note, layout_type, layout_json, image_url, date_label, time_label, doors_open, starts_at, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING id, slug, title, subtitle, category, venue_name, venue_city, venue_address, transit_note, layout_type, layout_json, image_url, date_label, time_label, doors_open, starts_at, status, created_at;

-- name: CreateEventSection :one
INSERT INTO event_sections (event_id, name, tier, section_price, fnb_credit, is_alcoholic, min_age, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, event_id, name, tier, section_price, fnb_credit, is_alcoholic, min_age, description;

-- name: UpdateEventStatus :one
UPDATE events
SET status = $2
WHERE id = $1
RETURNING id, slug, title, subtitle, category, venue_name, venue_city, venue_address, transit_note, layout_type, layout_json, image_url, date_label, time_label, doors_open, starts_at, status, created_at;

-- name: ListActiveEvents :many
SELECT id, slug, title, subtitle, category, venue_name, venue_city, venue_address, transit_note, layout_type, layout_json, image_url, date_label, time_label, doors_open, starts_at, status, created_at
FROM events
WHERE status IN ('ON_SALE', 'ALMOST_FULL')
ORDER BY starts_at ASC;

-- name: UpdateEventLayout :one
UPDATE events
SET layout_type = $2,
    layout_json = $3
WHERE id = $1
RETURNING id, slug, title, subtitle, category, venue_name, venue_city, venue_address, transit_note, layout_type, layout_json, image_url, date_label, time_label, doors_open, starts_at, status, created_at;
