-- name: ListFnbMenuItems :many
SELECT id, name, category, price, is_alcoholic, is_available
FROM fnb_menu_items
WHERE is_available = TRUE
ORDER BY category ASC, name ASC;

-- name: GetFnbMenuItemsByIDs :many
SELECT id, name, category, price, is_alcoholic, is_available
FROM fnb_menu_items
WHERE id = ANY($1::uuid[])
  AND is_available = TRUE;

-- name: CreateFnbMenuItem :one
INSERT INTO fnb_menu_items (name, category, price, is_alcoholic, is_available)
VALUES ($1, $2, $3, $4, TRUE)
RETURNING id, name, category, price, is_alcoholic, is_available;

-- name: DeductTicketFoodBalance :one
UPDATE tickets
SET food_balance = food_balance - $2
WHERE id = $1
  AND food_balance >= $2
  AND (expires_at IS NULL OR expires_at > NOW())
RETURNING id, order_id, event_id, user_id, code, bar_code, qr_token_digest, qr_otp_secret, gate_label, food_balance, initial_food_balance, checked_in, checked_in_at, checked_in_by, expires_at;

-- name: CreateFnbTransaction :one
INSERT INTO fnb_transactions (ticket_id, bartender_user_id, items_summary, total_amount, wallet_deducted, upi_delta_paid)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, ticket_id, bartender_user_id, items_summary, total_amount, wallet_deducted, upi_delta_paid, created_at;

-- name: ListRecentFnbTransactions :many
SELECT ft.id, ft.ticket_id, ft.bartender_user_id, ft.items_summary, ft.total_amount, ft.wallet_deducted, ft.upi_delta_paid, ft.created_at,
       t.code AS ticket_code, u.name AS bartender_name
FROM fnb_transactions ft
JOIN tickets t ON t.id = ft.ticket_id
JOIN users u ON u.id = ft.bartender_user_id
ORDER BY ft.created_at DESC
LIMIT $1;
