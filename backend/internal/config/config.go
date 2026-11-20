package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                   string
	Port                  string
	AppBaseURL            string
	DatabaseURL           string
	RedisURL              string
	AllowedOrigins        []string
	CookieDomain          string
	CookieSecure          bool
	JWTAccessSecret       string
	JWTRefreshSecret      string
	QRSigningSecret       string
	MFAEncryptionKey      string
	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	MFATokenTTL           time.Duration
	TicketQRTTL           time.Duration
	RotatingQREnabled     bool
	SeatHoldTTL           time.Duration
	DBQueryTimeout        time.Duration
	GoogleOAuthClientID   string
	FirebaseProjectID     string
	FirebaseServerKey     string
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
	ConvenienceFeeBps     int32
	ConvenienceGstBps     int32
	EntertainmentTaxBps   int32
	MaxSeatsPerOrder      int
	AuthRateLimitMax      int
	AuthRateLimitWindow   time.Duration
	HoldRateLimitMax      int
	HoldRateLimitWindow   time.Duration
	ScanRateLimitMax      int
	ScanRateLimitWindow   time.Duration
	WebhookRateLimitMax   int
	WebhookRateLimitWin   time.Duration
	ShutdownTimeout       time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	HTTPReadHeaderTimeout time.Duration
	RequestTimeout        time.Duration
}

func Load() (*Config, error) {
	env := getEnv("APP_ENV", "development")

	dbURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return nil, err
	}
	accessSecret, err := requireEnv("JWT_ACCESS_SECRET")
	if err != nil {
		return nil, err
	}
	refreshSecret, err := requireEnv("JWT_REFRESH_SECRET")
	if err != nil {
		return nil, err
	}
	qrSecret, err := requireEnv("QR_SIGNING_SECRET")
	if err != nil {
		return nil, err
	}
	mfaEncKey, err := requireEnv("MFA_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	webhookSecret, err := requireEnv("RAZORPAY_WEBHOOK_SECRET")
	if err != nil {
		return nil, err
	}
	googleClientID, err := requireEnv("GOOGLE_OAUTH_CLIENT_ID")
	if err != nil {
		return nil, err
	}
	firebaseProjectID, err := requireEnv("FIREBASE_PROJECT_ID")
	if err != nil {
		return nil, err
	}

	return &Config{
		Env:                   env,
		Port:                  getEnv("PORT", "8080"),
		AppBaseURL:            getEnv("APP_URL", "http://localhost:3000"),
		DatabaseURL:           dbURL,
		RedisURL:              os.Getenv("REDIS_URL"),
		AllowedOrigins:        splitCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:3000")),
		CookieDomain:          os.Getenv("COOKIE_DOMAIN"),
		CookieSecure:          getEnvBool("COOKIE_SECURE", env == "production"),
		JWTAccessSecret:       accessSecret,
		JWTRefreshSecret:      refreshSecret,
		QRSigningSecret:       qrSecret,
		MFAEncryptionKey:      mfaEncKey,
		AccessTokenTTL:        getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:       getEnvDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
		MFATokenTTL:           getEnvDuration("MFA_TOKEN_TTL", 5*time.Minute),
		TicketQRTTL:           getEnvDuration("TICKET_QR_TTL", 24*time.Hour),
		RotatingQREnabled:     getEnvBool("ROTATING_QR_ENABLED", false),
		SeatHoldTTL:           getEnvDuration("SEAT_HOLD_TTL", 9*time.Minute),
		DBQueryTimeout:        getEnvDuration("DB_QUERY_TIMEOUT", 3*time.Second),
		GoogleOAuthClientID:   googleClientID,
		FirebaseProjectID:     firebaseProjectID,
		FirebaseServerKey:     os.Getenv("FIREBASE_SERVER_KEY"),
		RazorpayKeyID:         os.Getenv("RAZORPAY_KEY_ID"),
		RazorpayKeySecret:     os.Getenv("RAZORPAY_KEY_SECRET"),
		RazorpayWebhookSecret: webhookSecret,
		ConvenienceFeeBps:     int32(getEnvInt("CONVENIENCE_FEE_BPS", 600)),
		ConvenienceGstBps:     int32(getEnvInt("CONVENIENCE_GST_BPS", 1800)),
		EntertainmentTaxBps:   int32(getEnvInt("ENTERTAINMENT_TAX_BPS", 500)),
		MaxSeatsPerOrder:      getEnvInt("MAX_SEATS_PER_ORDER", 12),
		AuthRateLimitMax:      getEnvInt("RATE_LIMIT_AUTH_MAX", 15),
		AuthRateLimitWindow:   getEnvDuration("RATE_LIMIT_AUTH_WINDOW", time.Minute),
		HoldRateLimitMax:      getEnvInt("RATE_LIMIT_HOLD_MAX", 30),
		HoldRateLimitWindow:   getEnvDuration("RATE_LIMIT_HOLD_WINDOW", time.Minute),
		ScanRateLimitMax:      getEnvInt("RATE_LIMIT_SCAN_MAX", 60),
		ScanRateLimitWindow:   getEnvDuration("RATE_LIMIT_SCAN_WINDOW", time.Minute),
		WebhookRateLimitMax:   getEnvInt("RATE_LIMIT_WEBHOOK_MAX", 60),
		WebhookRateLimitWin:   getEnvDuration("RATE_LIMIT_WEBHOOK_WINDOW", time.Minute),
		ShutdownTimeout:       getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		HTTPReadTimeout:       getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:      getEnvDuration("HTTP_WRITE_TIMEOUT", 20*time.Second),
		HTTPIdleTimeout:       getEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		HTTPReadHeaderTimeout: getEnvDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		RequestTimeout:        getEnvDuration("HTTP_REQUEST_TIMEOUT", 15*time.Second),
	}, nil
}

func requireEnv(key string) (string, error) {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return "", fmt.Errorf("missing required environment variable %s", key)
	}
	return val, nil
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
