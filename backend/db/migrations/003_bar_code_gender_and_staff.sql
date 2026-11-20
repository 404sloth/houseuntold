-- Add bartender secret bar_code on tickets and attendee_gender on order_seats.
ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS bar_code TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_tickets_bar_code_unique
    ON tickets (lower(bar_code))
    WHERE bar_code <> '';

ALTER TABLE order_seats
    ADD COLUMN IF NOT EXISTS attendee_gender TEXT NOT NULL DEFAULT 'OTHER';
