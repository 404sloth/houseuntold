package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings loaded strictly from environment variables.
type Config struct {
	Env                string
	Port               string
	AppBaseURL         string
	DatabaseURL        string
	DBMaxConns         int32
	DBMinConns         int32
	DBMaxConnLifetime  time.Duration
	DBMaxConnIdleTime  time.Duration
	DBQueryTimeout     time.Duration
	AllowedOrigins     []string
	CookieDomain       string
	CookieSecure       bool
	StaffCacheTTL      time.Duration

	// Staff JWT & Gate QR HMAC
	JWTAccessSecret  string
	JWTRefreshSecret string
	JWTIssuer        string
	JWTAudience      string
	QRSigningSecret  string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	TicketQRTTL      time.Duration

	// Direct UPI QR Payment (Manual Admin Verification — No Razorpay / No PhonePe)
	UPIVPA          string
	UPIMerchantName string
	UPIQRImageURL   string

	// Official Meta WhatsApp Cloud API
	WhatsAppAccessToken   string
	WhatsAppPhoneNumberID string
	WhatsAppAPIVersion    string
	WhatsAppWorkers       int
	WhatsAppHTTPTimeout   time.Duration

	// Business Rules
	First99FemaleLimit int64
	MinGuestAge        int32

	// PostgreSQL Rate Limiting
	AuthRateLimitMax        int
	AuthRateLimitWindow     time.Duration
	CheckoutRateLimitMax    int
	CheckoutRateLimitWindow time.Duration
	ScanRateLimitMax        int
	ScanRateLimitWindow     time.Duration
	BarRateLimitMax         int
	BarRateLimitWindow      time.Duration

	// HTTP Server & Graceful Shutdown Timeouts
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
	upiVPA, err := requireEnv("UPI_VPA")
	if err != nil {
		return nil, err
	}

	return &Config{
		Env:               env,
		Port:              getEnv("PORT", "8080"),
		AppBaseURL:        strings.TrimRight(getEnv("APP_URL", "http://localhost:3000"), "/"),
		DatabaseURL:       dbURL,
		DBMaxConns:        int32(getEnvInt("DB_MAX_CONNS", 50)),
		DBMinConns:        int32(getEnvInt("DB_MIN_CONNS", 10)),
		DBMaxConnLifetime: getEnvDuration("DB_MAX_CONN_LIFETIME", 30*time.Minute),
		DBMaxConnIdleTime: getEnvDuration("DB_MAX_CONN_IDLE_TIME", 5*time.Minute),
		DBQueryTimeout:    getEnvDuration("DB_QUERY_TIMEOUT", 3*time.Second),
		AllowedOrigins:    splitCSV(getEnv("ALLOWED_ORIGINS", "http://localhost:3000")),
		CookieDomain:      strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),
		CookieSecure:      getEnvBool("COOKIE_SECURE", env == "production"),
		StaffCacheTTL:     getEnvDuration("STAFF_CACHE_TTL", 30*time.Second),

		JWTAccessSecret:  accessSecret,
		JWTRefreshSecret: refreshSecret,
		JWTIssuer:        getEnv("JWT_ISSUER", "venu-api"),
		JWTAudience:      getEnv("JWT_AUDIENCE", "venu-staff"),
		QRSigningSecret:  qrSecret,
		AccessTokenTTL:   getEnvDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:  getEnvDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour),
		TicketQRTTL:      getEnvDuration("TICKET_QR_TTL", 24*time.Hour),

		UPIVPA:          upiVPA,
		UPIMerchantName: getEnv("UPI_MERCHANT_NAME", "Venu Live Events"),
		UPIQRImageURL:   strings.TrimSpace(os.Getenv("UPI_QR_IMAGE_URL")),

		WhatsAppAccessToken:   strings.TrimSpace(os.Getenv("WHATSAPP_ACCESS_TOKEN")),
		WhatsAppPhoneNumberID: strings.TrimSpace(os.Getenv("WHATSAPP_PHONE_NUMBER_ID")),
		WhatsAppAPIVersion:    getEnv("WHATSAPP_API_VERSION", "v21.0"),
		WhatsAppWorkers:       getEnvInt("WHATSAPP_WORKERS", 6),
		WhatsAppHTTPTimeout:   getEnvDuration("WHATSAPP_HTTP_TIMEOUT", 10*time.Second),

		First99FemaleLimit: int64(getEnvInt("FIRST_99_FEMALE_LIMIT", 99)),
		MinGuestAge:        int32(getEnvInt("MIN_GUEST_AGE", 21)),

		AuthRateLimitMax:        getEnvInt("RATE_LIMIT_AUTH_MAX", 15),
		AuthRateLimitWindow:     getEnvDuration("RATE_LIMIT_AUTH_WINDOW", time.Minute),
		CheckoutRateLimitMax:    getEnvInt("RATE_LIMIT_CHECKOUT_MAX", 20),
		CheckoutRateLimitWindow: getEnvDuration("RATE_LIMIT_CHECKOUT_WINDOW", time.Minute),
		ScanRateLimitMax:        getEnvInt("RATE_LIMIT_SCAN_MAX", 120),
		ScanRateLimitWindow:     getEnvDuration("RATE_LIMIT_SCAN_WINDOW", time.Minute),
		BarRateLimitMax:         getEnvInt("RATE_LIMIT_BAR_MAX", 180),
		BarRateLimitWindow:      getEnvDuration("RATE_LIMIT_BAR_WINDOW", time.Minute),

		ShutdownTimeout:       getEnvDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
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
		return "", fmt.Errorf("config: missing required environment variable %s", key)
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
