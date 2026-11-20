# Venu Backend Update Prompt & API Contract

Use this prompt to update, regenerate, or audit the Go backend (`backend/`) following GopherSocial layered architecture (`cmd/api`, `internal/handlers`, `internal/services`, `internal/repositories`, `internal/store`, `internal/mailer`), `sqlc`, zero hardcoded configuration, strict RBAC, JWT + rotating refresh token auto-refresh, Google OAuth, two-step Admin MFA, and graceful shutdown.

---

## Prompt

Update the Venu Go backend (`backend/`) with the following strict architectural rules:

1. **sqlc Only — Zero Manual Inline SQL & Zero `SELECT *`**
   - Define PostgreSQL migrations in `backend/db/migrations/*.sql` and SQL statements in `backend/internal/store/queries/*.sql` with **explicit column projections** (never `SELECT *`).
   - Run `sqlc generate` (configured via `backend/sqlc.yaml` pointing `schema: "db/migrations"` and `sql_package: "pgx/v5"`).
   - `internal/repositories/ticketing_repo.go` must execute **every** query and transaction exclusively through `*db.Queries` and `queries.WithTx(tx)`.
   - **Strict 3s DB Timeout**: Every `sqlc` execution and transaction MUST be wrapped in `context.WithTimeout(ctx, r.dbTimeout)` (default `DB_QUERY_TIMEOUT="3s"`).

2. **Unified JSON & Error Contract (`internal/httpx`)**
   - Errors MUST return `{"error": "message"}`.
   - Wrap errors with context across layers (`fmt.Errorf("layer_op: %w", err)`), log **once** at the handler boundary via `httpx.HandleError`, and map `AppError`, `context.DeadlineExceeded` -> `504`, `pgx.ErrNoRows` -> `404`, and `pgconn.PgError` (`23505` -> `409`, `23503`/`23514` -> `400`).

3. **Zero Hardcoded Values (`internal/config/config.go`)**
   - Load DB URL, Redis URL, port, JWT/QR secrets, TTLs (`ACCESS_TOKEN_TTL`, `REFRESH_TOKEN_TTL`, `MFA_TOKEN_TTL`, `SEAT_HOLD_TTL`, `DB_QUERY_TIMEOUT`), Razorpay keys, Google OAuth client ID, Firebase project ID/server key, fee BPS, and rate limits strictly into a typed `Config` struct.

4. **Backend-Driven Authentication, Two-Step MFA & Strict RBAC**
   - **Patrons (Google OAuth)**: `POST /api/auth/google` verifies the Google ID token and upserts user with hardcoded `USER` role.
   - **Staff & Admin Login**: `POST /api/auth/admin/login` validates email + Argon2id password without trusting any client-provided role:
     - `USER` -> `403` (patrons sign in with Google OAuth).
     - `SCANNER` / `BARTENDER` -> immediately issues Access + Refresh cookies with authoritative DB role.
     - `ADMIN` -> returns `{"requires_mfa": true, "mfa_challenge_token": "..."}`.
   - **Admin MFA Step 2**: `POST /api/auth/admin/mfa` validates TOTP (`VerifyTOTP`) + challenge JWT and issues Access + Refresh cookies (`venu_refresh` is `HttpOnly`, `Secure`, `SameSite=Strict`, `Path: /api/auth`).
   - **Auto-Refresh & Logout**: `POST /api/auth/refresh` (with family replay revocation) and `POST /api/auth/logout`.
   - **RBAC Middleware**: `rbac.Authenticate` injects `UserID`, `Role`, and `db.User` into `context.Context`; `rbac.RequireRole("ADMIN")`, `rbac.RequireRole("SCANNER", "ADMIN")`, and `rbac.RequireRole("BARTENDER", "ADMIN")` reject unauthorized callers with HTTP `403`.

5. **Complete API Route Table (`/api`)**
   - Public: `GET /api/health`, `GET /api/events`, `GET /api/events/{id}`, `GET /api/events/{id}/seats`, `POST /api/auth/google`, `POST /api/auth/admin/login`, `POST /api/auth/admin/mfa`, `POST /api/auth/refresh`, `POST /api/auth/logout`, `POST /api/webhooks/razorpay`
   - User (Authenticated): `GET /api/auth/me`, `POST /api/checkout/hold`, `DELETE /api/checkout/hold/{id}`, `POST /api/checkout/pay`, `GET /api/tickets`, `GET /api/tickets/{id}/qr`
   - Scanner (`SCANNER | ADMIN`, rate-limited): `POST /api/scan/validate`, `POST /api/scan/check-in`, `GET /api/scan/recent`
   - Bartender (`BARTENDER | ADMIN`): `GET /api/counter/menu`, `GET /api/counter/transactions`, `POST /api/counter/validate`, `POST /api/counter/redeem`
   - Admin (`ADMIN`): `GET /api/admin/stats`, `POST /api/admin/events`, `PATCH /api/admin/events/{id}/layout`, `PATCH /api/admin/events/{id}/status`, `GET /api/admin/orders`, `PATCH /api/admin/users/{id}/role`

6. **Razorpay SDK, Firebase Pass Mailer & Graceful Shutdown**
   - Use official `github.com/razorpay/razorpay-go` (`Order.Create` + `utils.VerifyWebhookSignature`) and `internal/mailer.FirebasePassMailer` buffered worker pool drained cleanly during `SIGINT`/`SIGTERM` shutdown (`srv.Shutdown`, `passMailer.CloseAndWait`, `redisClient.Close`, `pool.Close`).
