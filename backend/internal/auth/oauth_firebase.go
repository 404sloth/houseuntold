package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func decodeB64Fallback(s string) ([]byte, error) {
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

type GoogleIdentity struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
}

type FirebasePhoneIdentity struct {
	UID         string
	PhoneNumber string
	Email       string
	Name        string
}

type IdentityVerifier struct {
	httpClient        *http.Client
	googleClientID    string
	firebaseProjectID string
}

func NewIdentityVerifier(googleClientID, firebaseProjectID string) *IdentityVerifier {
	return &IdentityVerifier{
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
		googleClientID:    googleClientID,
		firebaseProjectID: firebaseProjectID,
	}
}

// VerifyGoogleIDToken validates a Google OAuth 2.0 ID token against Google's tokeninfo endpoint and checks audience.
func (v *IdentityVerifier) VerifyGoogleIDToken(ctx context.Context, idToken string) (*GoogleIdentity, error) {
	endpoint := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(idToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("google_tokeninfo_req: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_tokeninfo_do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("google id token rejected")
	}

	var payload struct {
		Sub           string `json:"sub"`
		Aud           string `json:"aud"`
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("google_tokeninfo_decode: %w", err)
	}

	if payload.Aud != v.googleClientID {
		return nil, errors.New("google token audience mismatch")
	}
	if payload.EmailVerified != "true" || payload.Email == "" {
		return nil, errors.New("google account email not verified")
	}
	if payload.Name == "" {
		payload.Name = payload.Email
	}

	return &GoogleIdentity{
		Sub:           payload.Sub,
		Email:         payload.Email,
		EmailVerified: true,
		Name:          payload.Name,
	}, nil
}

// VerifyFirebaseToken validates a Firebase Auth ID token (Phone Auth / OAuth) via Google Identity Toolkit lookup
// or JWT issuer/audience validation against cfg.FirebaseProjectID.
func (v *IdentityVerifier) VerifyFirebaseToken(ctx context.Context, idToken string) (*FirebasePhoneIdentity, error) {
	endpoint := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(idToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("firebase_token_req: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firebase_token_do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("firebase id token rejected")
	}

	var payload struct {
		Sub         string `json:"sub"`
		Aud         string `json:"aud"`
		Iss         string `json:"iss"`
		PhoneNumber string `json:"phone_number"`
		Email       string `json:"email"`
		Name        string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("firebase_token_decode: %w", err)
	}

	expectedIss := "https://securetoken.google.com/" + v.firebaseProjectID
	if payload.Aud != v.firebaseProjectID || (payload.Iss != "" && payload.Iss != expectedIss) {
		return nil, errors.New("firebase token project mismatch")
	}
	if payload.Sub == "" {
		return nil, errors.New("firebase token missing subject uid")
	}

	return &FirebasePhoneIdentity{
		UID:         payload.Sub,
		PhoneNumber: payload.PhoneNumber,
		Email:       payload.Email,
		Name:        payload.Name,
	}, nil
}
