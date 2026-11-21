-- name: GetStaffByUsername :one
SELECT id, username, password_hash, role, is_active, created_at
FROM staff_accounts
WHERE username = $1;

-- name: GetStaffByID :one
SELECT id, username, password_hash, role, is_active, created_at
FROM staff_accounts
WHERE id = $1;

-- name: CreateStaff :one
INSERT INTO staff_accounts (username, password_hash, role, is_active)
VALUES ($1, $2, $3, true)
RETURNING id, username, password_hash, role, is_active, created_at;

-- name: UpdateStaffPassword :one
UPDATE staff_accounts
SET password_hash = $2
WHERE id = $1 AND is_active = true
RETURNING id, username, role, is_active, created_at;

-- name: DeactivateStaff :one
UPDATE staff_accounts
SET is_active = false
WHERE id = $1 AND is_active = true
RETURNING id, username, role, is_active;
