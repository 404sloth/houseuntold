-- Venu Ticketing Platform — BCNF Normalized PostgreSQL Schema (Migration 001)

CREATE TYPE user_role AS ENUM ('USER', 'ADMIN', 'SCANNER', 'BARTENDER');
CREATE TYPE auth_provider AS ENUM ('GOOGLE', 'LOCAL_ADMIN', 'FIREBASE_PHONE');
CREATE TYPE seat_status AS ENUM ('AVAILABLE', 'HELD', 'SOLD');
CREATE TYPE order_status AS ENUM ('PENDING', 'CONFIRMED', 'FAILED', 'EXPIRED');
CREATE TYPE tier_type AS ENUM ('VIP', 'COUPLE', 'GENERAL');

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    email           TEXT NOT NULL UNIQUE,
    auth_provider   auth_provider NOT NULL DEFAULT 'GOOGLE',
    google_sub      TEXT UNIQUE,
    firebase_uid    TEXT UNIQUE,
    password_hash   TEXT,
    totp_secret     TEXT,
    phone           TEXT NOT NULL DEFAULT '',
    phone_verified  BOOLEAN NOT NULL DEFAULT FALSE,
    role            user_role NOT NULL DEFAULT 'USER',
    age             INT NOT NULL DEFAULT 18 CHECK (age >= 5),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    family_id   UUID NOT NULL,
    revoked     BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_family_id ON refresh_tokens (family_id);

CREATE TABLE events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    title           TEXT NOT NULL,
    subtitle        TEXT NOT NULL,
    category        TEXT NOT NULL,
    venue_name      TEXT NOT NULL,
    venue_city      TEXT NOT NULL,
    venue_address   TEXT NOT NULL,
    transit_note    TEXT NOT NULL DEFAULT '',
    layout_type     TEXT NOT NULL CHECK (layout_type IN ('PROSCENIUM', 'CLUB')),
    layout_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    image_url       TEXT NOT NULL DEFAULT '',
    date_label      TEXT NOT NULL,
    time_label      TEXT NOT NULL,
    doors_open      TEXT NOT NULL DEFAULT '',
    starts_at       TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'ON_SALE' CHECK (status IN ('ON_SALE', 'ALMOST_FULL', 'DRAFT', 'PAUSED')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE event_sections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    tier            tier_type NOT NULL,
    section_price   INT NOT NULL CHECK (section_price >= 0),
    fnb_credit      INT NOT NULL DEFAULT 0 CHECK (fnb_credit >= 0),
    is_alcoholic    BOOLEAN NOT NULL DEFAULT FALSE,
    min_age         INT NOT NULL DEFAULT 0 CHECK (min_age >= 0),
    description     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE seat_inventory (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    section_id      UUID NOT NULL REFERENCES event_sections(id) ON DELETE CASCADE,
    row_label       TEXT NOT NULL,
    seat_number     INT NOT NULL,
    label           TEXT NOT NULL,
    pos_x           INT NOT NULL,
    pos_y           INT NOT NULL,
    status          seat_status NOT NULL DEFAULT 'AVAILABLE',
    held_by_user_id UUID REFERENCES users(id),
    hold_expires_at TIMESTAMPTZ,
    UNIQUE (event_id, row_label, seat_number)
);

CREATE INDEX idx_seat_inventory_event_status ON seat_inventory (event_id, status);

CREATE TABLE orders (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id),
    event_id            UUID NOT NULL REFERENCES events(id),
    razorpay_order_id   TEXT NOT NULL UNIQUE,
    base_amount         INT NOT NULL,
    convenience_fee     INT NOT NULL,
    gst_amount          INT NOT NULL,
    total_payable       INT NOT NULL,
    contact_email       TEXT NOT NULL,
    contact_phone       TEXT NOT NULL,
    status              order_status NOT NULL DEFAULT 'PENDING',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at        TIMESTAMPTZ
);

CREATE TABLE order_seats (
    order_id        UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    seat_id         UUID NOT NULL REFERENCES seat_inventory(id),
    attendee_name   TEXT NOT NULL,
    attendee_age    INT NOT NULL,
    admitted        BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (order_id, seat_id)
);

CREATE TABLE tickets (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id             UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    event_id             UUID NOT NULL REFERENCES events(id),
    user_id              UUID NOT NULL REFERENCES users(id),
    code                 TEXT NOT NULL UNIQUE,
    qr_token             TEXT NOT NULL UNIQUE,
    gate_label           TEXT NOT NULL,
    food_balance         INT NOT NULL DEFAULT 0 CHECK (food_balance >= 0),
    initial_food_balance INT NOT NULL DEFAULT 0 CHECK (initial_food_balance >= 0),
    checked_in           BOOLEAN NOT NULL DEFAULT FALSE,
    checked_in_at        TIMESTAMPTZ
);

CREATE TABLE gate_scan_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id       UUID REFERENCES tickets(id),
    scanner_user_id UUID NOT NULL REFERENCES users(id),
    ticket_code     TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('VALID_ADMITTED', 'ALREADY_USED', 'INVALID')),
    scanned_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE fnb_menu_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    category     TEXT NOT NULL,
    price        INT NOT NULL CHECK (price > 0),
    is_alcoholic BOOLEAN NOT NULL DEFAULT FALSE,
    is_available BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE fnb_transactions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id           UUID NOT NULL REFERENCES tickets(id),
    bartender_user_id   UUID NOT NULL REFERENCES users(id),
    items_summary       TEXT NOT NULL,
    total_amount        INT NOT NULL CHECK (total_amount > 0),
    wallet_deducted     INT NOT NULL CHECK (wallet_deducted >= 0),
    upi_delta_paid      INT NOT NULL DEFAULT 0 CHECK (upi_delta_paid >= 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
