-- name: UpsertRateLimitHit :one
INSERT INTO rate_limits (identifier, endpoint, hits, window_start)
VALUES ($1, $2, 1, $3)
ON CONFLICT (identifier, endpoint, window_start)
DO UPDATE SET hits = rate_limits.hits + 1
RETURNING hits;

-- name: DeleteExpiredRateLimits :exec
DELETE FROM rate_limits
WHERE window_start < $1;
