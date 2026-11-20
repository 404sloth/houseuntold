package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/db"
)

var (
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrTicketTokenExpired = errors.New("ticket QR token has expired")
)

type AccessClaims struct {
	UserID uuid.UUID   `json:"uid"`
	Role   db.UserRole `json:"role"`
	Email  string      `json:"email"`
	jwt.RegisteredClaims
}

type MFAChallengeClaims struct {
	UserID  uuid.UUID   `json:"uid"`
	Role    db.UserRole `json:"role"`
	Purpose string      `json:"purpose"`
	jwt.RegisteredClaims
}

// TicketQRPayload represents the compact HMAC-SHA256-signed QR payload (`payloadB64.signatureB64`).
type TicketQRPayload struct {
	TicketID     uuid.UUID `json:"ticket_id"`
	OrderID      uuid.UUID `json:"order_id"`
	EventID      uuid.UUID `json:"event_id"`
	Code         string    `json:"code"`
	IssuedAt     int64     `json:"iat"`
	ExpiresAt    int64     `json:"exp"`
	RotatingCode string    `json:"rot,omitempty"`
}

type TokenManager struct {
	accessSecret []byte
	qrSecret     []byte
	accessTTL    time.Duration
	mfaTTL       time.Duration
}

func NewTokenManager(accessSecret, qrSecret string, accessTTL, mfaTTL time.Duration) *TokenManager {
	if mfaTTL <= 0 {
		mfaTTL = 5 * time.Minute
	}
	return &TokenManager{
		accessSecret: []byte(accessSecret),
		qrSecret:     []byte(qrSecret),
		accessTTL:    accessTTL,
		mfaTTL:       mfaTTL,
	}
}

func (m *TokenManager) CreateAccessToken(user db.User) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(m.accessTTL)
	claims := AccessClaims{
		UserID: user.ID,
		Role:   user.Role,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    "venu-api",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.accessSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign_access_jwt: %w", err)
	}
	return signed, exp, nil
}

func (m *TokenManager) VerifyAccessToken(raw string) (*AccessClaims, error) {
	tok, err := jwt.ParseWithClaims(raw, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.accessSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(*AccessClaims)
	if !ok || !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (m *TokenManager) CreateMFAChallengeToken(user db.User) (string, error) {
	now := time.Now().UTC()
	claims := MFAChallengeClaims{
		UserID:  user.ID,
		Role:    user.Role,
		Purpose: "admin_mfa",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    "venu-mfa",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.mfaTTL)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.accessSecret)
	if err != nil {
		return "", fmt.Errorf("sign_mfa_jwt: %w", err)
	}
	return signed, nil
}

func (m *TokenManager) VerifyMFAChallengeToken(raw string) (*MFAChallengeClaims, error) {
	tok, err := jwt.ParseWithClaims(raw, &MFAChallengeClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.accessSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(*MFAChallengeClaims)
	if !ok || !tok.Valid || claims.Purpose != "admin_mfa" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ComputeTicketDigest returns a deterministic versioned HMAC-SHA256(ticket_id, QR_SIGNING_SECRET)
// for database storage in tickets.qr_token_digest — plaintext QR tokens are never persisted in DB.
func (m *TokenManager) ComputeTicketDigest(ticketID uuid.UUID) string {
	mac := hmac.New(sha256.New, m.qrSecret)
	mac.Write([]byte("v1:" + ticketID.String()))
	return "v1:" + hex.EncodeToString(mac.Sum(nil))
}

// CreateTicketQRToken produces an HMAC-SHA256-signed token (`payload.signature`) containing
// ticket_id, order_id, iat, exp, and an optional rotating 30s OTP value.
func (m *TokenManager) CreateTicketQRToken(
	ticketID, orderID, eventID uuid.UUID,
	code string,
	expiresAt time.Time,
	rotatingOtpSecret *string,
) (string, error) {
	now := time.Now().UTC()
	if expiresAt.IsZero() {
		expiresAt = now.Add(24 * time.Hour)
	}
	payload := TicketQRPayload{
		TicketID:  ticketID,
		OrderID:   orderID,
		EventID:   eventID,
		Code:      code,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}
	if rotatingOtpSecret != nil && strings.TrimSpace(*rotatingOtpSecret) != "" {
		rot, err := GenerateTOTP(*rotatingOtpSecret, now)
		if err != nil {
			return "", fmt.Errorf("ticket_rotating_totp: %w", err)
		}
		payload.RotatingCode = rot
	}

	rawJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal_qr_payload: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(rawJSON)
	sig := m.signQRPayload(payloadB64)
	return payloadB64 + "." + sig, nil
}

// VerifyTicketQRToken validates the HMAC-SHA256 signature and expiration timestamp of `payload.signature`.
func (m *TokenManager) VerifyTicketQRToken(raw string, now time.Time) (*TicketQRPayload, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrInvalidToken
	}

	expectedSig := m.signQRPayload(parts[0])
	if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expectedSig)) != 1 {
		return nil, ErrInvalidToken
	}

	rawJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var payload TicketQRPayload
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		return nil, ErrInvalidToken
	}
	if payload.TicketID == uuid.Nil || payload.OrderID == uuid.Nil {
		return nil, ErrInvalidToken
	}
	if payload.ExpiresAt <= now.UTC().Unix() {
		return nil, ErrTicketTokenExpired
	}
	return &payload, nil
}

func (m *TokenManager) signQRPayload(payloadB64 string) string {
	mac := hmac.New(sha256.New, m.qrSecret)
	mac.Write([]byte(payloadB64))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// GenerateOpaqueRefreshToken creates a cryptographically random 32-byte token + SHA-256 hash for DB storage.
func GenerateOpaqueRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("rand_refresh_token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	hash = HashRefreshToken(raw)
	return raw, hash, nil
}

func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
