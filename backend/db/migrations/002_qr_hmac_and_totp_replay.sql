-- Venu Ticketing Platform — Migration 002
-- 1. Never store plaintext QR tokens in tickets: store HMAC-SHA256(ticket_id) digest + optional per-ticket rotating OTP secret + checked_in_by + expires_at.
-- 2. Add totp_last_accepted_step high-water mark to users for atomic RFC 6238 TOTP replay protection.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS totp_last_accepted_step BIGINT DEFAULT NULL;

ALTER TABLE tickets
    DROP COLUMN IF EXISTS qr_token;

ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS qr_token_digest TEXT NOT NULL UNIQUE,
    ADD COLUMN IF NOT EXISTS qr_otp_secret TEXT,
    ADD COLUMN IF NOT EXISTS checked_in_by UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_tickets_qr_token_digest ON tickets (qr_token_digest);
