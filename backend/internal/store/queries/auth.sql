-- name: GetUserByID :one
SELECT id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at
FROM users
WHERE id = $1
LIMIT 1;

-- name: GetUserForMFAUpdate :one
SELECT id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at
FROM users
WHERE id = $1
FOR UPDATE;

-- name: GetUserByEmail :one
SELECT id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at
FROM users
WHERE lower(email) = lower($1)
LIMIT 1;

-- name: GetUserByGoogleSub :one
SELECT id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at
FROM users
WHERE google_sub = $1
LIMIT 1;

-- name: UpsertGoogleUser :one
INSERT INTO users (name, email, auth_provider, google_sub, role)
VALUES ($1, $2, 'GOOGLE', $3, 'USER')
ON CONFLICT (email) DO UPDATE
SET name = EXCLUDED.name,
    google_sub = COALESCE(users.google_sub, EXCLUDED.google_sub),
    updated_at = NOW()
RETURNING id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at;

-- name: UpsertFirebasePhoneUser :one
INSERT INTO users (name, email, auth_provider, firebase_uid, phone, phone_verified, role)
VALUES ($1, $2, 'FIREBASE_PHONE', $3, $4, TRUE, 'USER')
ON CONFLICT (email) DO UPDATE
SET firebase_uid = COALESCE(users.firebase_uid, EXCLUDED.firebase_uid),
    phone = EXCLUDED.phone,
    phone_verified = TRUE,
    updated_at = NOW()
RETURNING id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at;

-- name: MarkUserPhoneVerified :one
UPDATE users
SET firebase_uid = $2,
    phone = $3,
    phone_verified = TRUE,
    updated_at = NOW()
WHERE id = $1
RETURNING id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at;

-- name: EnrollUserTOTP :one
UPDATE users
SET totp_secret = $2,
    totp_last_accepted_step = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at;

-- name: AdvanceTOTPLastAcceptedStep :one
UPDATE users
SET totp_last_accepted_step = $2,
    updated_at = NOW()
WHERE id = $1
  AND (totp_last_accepted_step IS NULL OR totp_last_accepted_step < $2)
RETURNING id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at;

-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, token_hash, family_id, revoked, expires_at, created_at;

-- name: GetRefreshTokenByHash :one
SELECT id, user_id, token_hash, family_id, revoked, expires_at, created_at
FROM refresh_tokens
WHERE token_hash = $1
LIMIT 1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked = TRUE
WHERE id = $1;

-- name: RevokeTokenFamily :exec
UPDATE refresh_tokens
SET revoked = TRUE
WHERE family_id = $1;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens
SET revoked = TRUE
WHERE user_id = $1;

-- name: UpdateUserRole :one
UPDATE users
SET role = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING id, name, email, auth_provider, google_sub, firebase_uid, password_hash, totp_secret, totp_last_accepted_step, phone, phone_verified, role, age, is_active, created_at, updated_at;

-- name: CreateStaffUser :one
INSERT INTO users (name, email, auth_provider, password_hash, role, phone, phone_verified, age, is_active)
VALUES ($1, $2, 'LOCAL_ADMIN', $3, $4, '+91 98200 00000', TRUE, 28, TRUE)
RETURNING id, name, email, role, is_active, created_at;

-- name: ListStaffUsers :many
SELECT id, name, email, role, is_active, created_at
FROM users
WHERE role IN ('SCANNER', 'BARTENDER')
ORDER BY created_at DESC;

