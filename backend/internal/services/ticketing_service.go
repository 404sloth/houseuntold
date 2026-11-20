package services

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	razorpay "github.com/razorpay/razorpay-go"
	rpputils "github.com/razorpay/razorpay-go/utils"
	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/mailer"
	"github.com/venu-live/venu-api/internal/repositories"
)

var (
	ErrUnderageForAlcoholZone   = errors.New("attendee age is below minimum age for alcohol zone")
	ErrInvalidCoupleComposition = errors.New("couple zone requires pairs of 2 guests with 1 male and 1 female")
	ErrInvalidWebhookHMAC       = errors.New("invalid razorpay webhook signature")
	ErrUpiDeltaUnconfirmed      = errors.New("upi delta confirmation required when order exceeds wallet balance")
)

type OrderPricing struct {
	EventID        uuid.UUID   `json:"eventId"`
	SeatIDs        []uuid.UUID `json:"seatIds"`
	SeatCount      int         `json:"seatCount"`
	BaseAmount     int32       `json:"baseAmount"`
	ConvenienceFee int32       `json:"convenienceFee"`
	GSTAmount      int32       `json:"gstAmount"`
	FnBCreditTotal int32       `json:"fnbCreditTotal"`
	TotalPayable   int32       `json:"totalPayable"`
	HoldExpiresAt  *time.Time  `json:"holdExpiresAt"`
}

type AttendeeInput struct {
	SeatID    uuid.UUID `json:"seatId"`
	SeatLabel string    `json:"seatLabel"`
	FullName  string    `json:"fullName"`
	Age       int32     `json:"age"`
	Gender    string    `json:"gender"`
}

type AuthResult struct {
	User            db.User   `json:"user"`
	AccessToken     string    `json:"-"`
	RefreshToken    string    `json:"-"`
	AccessExpiresAt time.Time `json:"accessExpiresAt"`
}

type AdminLoginOutcome struct {
	RequiresMFA       bool        `json:"requires_mfa"`
	MFAChallengeToken string      `json:"mfa_challenge_token,omitempty"`
	Session           *AuthResult `json:"-"`
}

type SeatMapPayload struct {
	Event     db.Event          `json:"event"`
	Sections  []db.EventSection `json:"sections"`
	Seats     []SeatDTO         `json:"seats"`
	Breakdown OrderPricing      `json:"breakdown"`
}

type SeatDTO struct {
	ID        uuid.UUID   `json:"id"`
	EventID   uuid.UUID   `json:"eventId"`
	SectionID uuid.UUID   `json:"sectionId"`
	Row       string      `json:"row"`
	Number    int32       `json:"number"`
	Label     string      `json:"label"`
	Tier      db.TierType `json:"tier"`
	Status    string      `json:"status"`
	X         int32       `json:"x"`
	Y         int32       `json:"y"`
}

type CheckoutInitResult struct {
	RazorpayOrderID string       `json:"razorpayOrderId"`
	AmountPaise     int64        `json:"amountPaise"`
	Currency        string       `json:"currency"`
	Breakdown       OrderPricing `json:"breakdown"`
}

type FnbCartEntry struct {
	ItemID   uuid.UUID `json:"itemId"`
	Quantity int32     `json:"quantity"`
}

type FnbQuoteResult struct {
	TicketID         *uuid.UUID `json:"ticketId"`
	WalletBalance    int32      `json:"walletBalance"`
	CartTotal        int32      `json:"cartTotal"`
	WalletDeduction  int32      `json:"walletDeduction"`
	UpiDeltaRequired int32      `json:"upiDeltaRequired"`
	RemainingBalance int32      `json:"remainingBalance"`
	ExceedsWallet    bool       `json:"exceedsWallet"`
}

type FnbSettleResult struct {
	Ticket      db.Ticket         `json:"ticket"`
	Transaction db.FnbTransaction `json:"transaction"`
}

type CreateStaffUserInput struct {
	Name     string      `json:"name"`
	Email    string      `json:"email"`
	Password string      `json:"password"`
	Role     db.UserRole `json:"role"`
}

type CreateFnbMenuItemInput struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Price       int32  `json:"price"`
	IsAlcoholic bool   `json:"isAlcoholic"`
}

type AdminMFAEnrollmentResult struct {
	OtpAuthURI string `json:"otpauthUri"`
	Secret     string `json:"secret"`
	Issuer     string `json:"issuer"`
	Algorithm  string `json:"algorithm"`
	Digits     int    `json:"digits"`
	Period     int    `json:"period"`
}

type TicketQRResult struct {
	TicketID         uuid.UUID `json:"ticketId"`
	Code             string    `json:"code"`
	QrToken          string    `json:"qrToken"`
	Rotating         bool      `json:"rotating"`
	ExpiresInSeconds int       `json:"expiresInSeconds,omitempty"`
}

type UserTicketDTO struct {
	db.ListTicketsByUserRow
	QrToken string `json:"qrToken"`
}

type ScanLookupResult struct {
	Valid     bool                           `json:"valid"`
	Reason    string                         `json:"reason"`
	Ticket    *db.GetTicketByCodeOrDigestRow `json:"ticket"`
	Attendees []db.ListOrderAttendeesRow     `json:"attendees"`
}

type CreateEventInput struct {
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
	Category     string `json:"category"`
	VenueName    string `json:"venueName"`
	VenueCity    string `json:"venueCity"`
	VenueAddress string `json:"venueAddress"`
	DateLabel    string `json:"dateLabel"`
	TimeLabel    string `json:"timeLabel"`
	LayoutType   string `json:"layoutType"`
	VIPPrice     int32  `json:"vipPrice"`
	GeneralPrice int32  `json:"generalPrice"`
	VIPFnbCredit int32  `json:"vipFnbCredit"`
	VIPMinAge    int32  `json:"vipMinAge"`
}

type TokenVerifier interface {
	VerifyGoogleIDToken(ctx context.Context, idToken string) (*auth.GoogleIdentity, error)
}

type TicketingService struct {
	cfg            *config.Config
	store          repositories.TicketingStore
	tokens         *auth.TokenManager
	verifier       TokenVerifier
	mailer         mailer.Mailer
	razorpayClient *razorpay.Client
	logger         *slog.Logger
}

func NewTicketingService(
	cfg *config.Config,
	store repositories.TicketingStore,
	tokens *auth.TokenManager,
	verifier TokenVerifier,
	passMailer mailer.Mailer,
	logger *slog.Logger,
) *TicketingService {
	if logger == nil {
		logger = slog.Default()
	}
	var rzp *razorpay.Client
	if cfg != nil && cfg.RazorpayKeyID != "" && cfg.RazorpayKeySecret != "" {
		rzp = razorpay.NewClient(cfg.RazorpayKeyID, cfg.RazorpayKeySecret)
	}
	return &TicketingService{
		cfg:            cfg,
		store:          store,
		tokens:         tokens,
		verifier:       verifier,
		mailer:         passMailer,
		razorpayClient: rzp,
		logger:         logger,
	}
}

// ComputeAuthoritativePricing calculates ticket fees & GST on the server using env-configured BPS.
func (s *TicketingService) ComputeAuthoritativePricing(eventID uuid.UUID, quotes []db.QuoteSeatsByIDsRow, holdExpiresAt *time.Time) OrderPricing {
	if len(quotes) == 0 {
		return OrderPricing{
			EventID: eventID,
			SeatIDs: []uuid.UUID{},
		}
	}

	var base, fnb int32
	seatIDs := make([]uuid.UUID, 0, len(quotes))
	for _, q := range quotes {
		base += q.SectionPrice
		fnb += q.FnbCredit
		seatIDs = append(seatIDs, q.ID)
	}

	conv := int32(math.Round(float64(base) * float64(s.cfg.ConvenienceFeeBps) / 10000.0))
	gst := int32(math.Round(
		float64(conv)*float64(s.cfg.ConvenienceGstBps)/10000.0 +
			float64(base)*float64(s.cfg.EntertainmentTaxBps)/10000.0,
	))

	return OrderPricing{
		EventID:        eventID,
		SeatIDs:        seatIDs,
		SeatCount:      len(quotes),
		BaseAmount:     base,
		ConvenienceFee: conv,
		GSTAmount:      gst,
		FnBCreditTotal: fnb,
		TotalPayable:   base + conv + gst,
		HoldExpiresAt:  holdExpiresAt,
	}
}

// ValidateAgeCompliance verifies every attendee meets the minimum age of their selected section.
func ValidateAgeCompliance(quotes []db.QuoteSeatsByIDsRow, attendees []AttendeeInput) error {
	ageBySeat := make(map[uuid.UUID]int32, len(attendees))
	for _, a := range attendees {
		ageBySeat[a.SeatID] = a.Age
	}
	for _, q := range quotes {
		if q.MinAge <= 0 {
			continue
		}
		age, ok := ageBySeat[q.ID]
		if !ok || age < q.MinAge {
			return fmt.Errorf("%w: seat %s requires %d+", ErrUnderageForAlcoholZone, q.Label, q.MinAge)
		}
	}
	return nil
}

// ValidateCoupleComposition enforces that COUPLE tier seats are booked in pairs of 2 with 1 MALE and 1 FEMALE guest.
func ValidateCoupleComposition(quotes []db.QuoteSeatsByIDsRow, attendees []AttendeeInput) error {
	quoteByID := make(map[uuid.UUID]db.QuoteSeatsByIDsRow, len(quotes))
	coupleCount := 0
	for _, q := range quotes {
		quoteByID[q.ID] = q
		if q.Tier == db.TierTypeCOUPLE {
			coupleCount++
		}
	}
	if coupleCount == 0 {
		return nil
	}
	if coupleCount%2 != 0 {
		return fmt.Errorf("%w: select seats in pairs of 2", ErrInvalidCoupleComposition)
	}
	var males, females int
	for _, a := range attendees {
		q, ok := quoteByID[a.SeatID]
		if !ok || q.Tier != db.TierTypeCOUPLE {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(a.Gender)) {
		case "MALE":
			males++
		case "FEMALE":
			females++
		}
	}
	expectedPerGender := coupleCount / 2
	if males != expectedPerGender || females != expectedPerGender {
		return fmt.Errorf("%w: got %d male and %d female for %d couple seats", ErrInvalidCoupleComposition, males, females, coupleCount)
	}
	return nil
}

// --- Authentication, Google OAuth, Staff/Admin Two-Step Login & Rotating Refresh Tokens ---

func (s *TicketingService) issueSession(ctx context.Context, user db.User, familyID uuid.UUID) (AuthResult, error) {
	accessTok, exp, err := s.tokens.CreateAccessToken(user)
	if err != nil {
		return AuthResult{}, fmt.Errorf("service_issue_access: %w", err)
	}
	rawRefresh, hashRefresh, err := auth.GenerateOpaqueRefreshToken()
	if err != nil {
		return AuthResult{}, fmt.Errorf("service_issue_refresh: %w", err)
	}
	if _, err := s.store.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: hashRefresh,
		FamilyID:  familyID,
		ExpiresAt: time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
	}); err != nil {
		return AuthResult{}, fmt.Errorf("service_persist_refresh: %w", err)
	}

	return AuthResult{
		User:            user,
		AccessToken:     accessTok,
		RefreshToken:    rawRefresh,
		AccessExpiresAt: exp,
	}, nil
}

func (s *TicketingService) LoginWithGoogle(ctx context.Context, idToken string) (AuthResult, error) {
	if strings.TrimSpace(idToken) == "" {
		return AuthResult{}, httpx.NewBadRequest("idToken is required", nil)
	}
	ident, err := s.verifier.VerifyGoogleIDToken(ctx, idToken)
	if err != nil {
		return AuthResult{}, httpx.NewUnauthorized("Google OAuth verification failed", err)
	}
	sub := ident.Sub
	user, err := s.store.UpsertGoogleUser(ctx, db.UpsertGoogleUserParams{
		Name:      ident.Name,
		Email:     ident.Email,
		GoogleSub: &sub,
	})
	if err != nil {
		return AuthResult{}, fmt.Errorf("service_google_upsert_user: %w", err)
	}
	if !user.IsActive {
		return AuthResult{}, httpx.NewForbidden("account deactivated")
	}
	return s.issueSession(ctx, user, uuid.New())
}

// LoginAdminOrStaff verifies Argon2id password for operators.
// ADMIN accounts require step-2 TOTP MFA; SCANNER and BARTENDER staff receive JWTs immediately
// with their authoritative DB role (never chosen on client).
func (s *TicketingService) LoginAdminOrStaff(ctx context.Context, email, password string) (AdminLoginOutcome, error) {
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return AdminLoginOutcome{}, httpx.NewUnauthorized("invalid operator credentials", err)
	}
	if !user.IsActive || user.Role == db.UserRoleUSER {
		return AdminLoginOutcome{}, httpx.NewForbidden("operator role required")
	}
	if user.PasswordHash == nil {
		return AdminLoginOutcome{}, httpx.NewForbidden("password login not configured for account")
	}
	ok, err := auth.VerifyArgon2id(password, *user.PasswordHash)
	if err != nil || !ok {
		return AdminLoginOutcome{}, httpx.NewUnauthorized("invalid operator credentials", err)
	}

	if user.Role == db.UserRoleADMIN {
		challengeTok, err := s.tokens.CreateMFAChallengeToken(user)
		if err != nil {
			return AdminLoginOutcome{}, fmt.Errorf("service_create_mfa_challenge: %w", err)
		}
		return AdminLoginOutcome{
			RequiresMFA:       true,
			MFAChallengeToken: challengeTok,
		}, nil
	}

	// Staff (SCANNER or BARTENDER): issue JWT directly with backend DB role
	session, err := s.issueSession(ctx, user, uuid.New())
	if err != nil {
		return AdminLoginOutcome{}, fmt.Errorf("service_staff_session: %w", err)
	}
	return AdminLoginOutcome{
		RequiresMFA: false,
		Session:     &session,
	}, nil
}

// EnrollAdminMFA generates a 160-bit Base32 TOTP secret, builds the otpauth:// URI for one-time
// authenticator app enrollment, and stores the AES-256-GCM encrypted secret at rest.
func (s *TicketingService) EnrollAdminMFA(ctx context.Context, userID uuid.UUID) (AdminMFAEnrollmentResult, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return AdminMFAEnrollmentResult{}, fmt.Errorf("service_enroll_mfa_user: %w", err)
	}
	if user.Role != db.UserRoleADMIN || !user.IsActive {
		return AdminMFAEnrollmentResult{}, httpx.NewForbidden("only active admins can enroll TOTP MFA")
	}

	enrollment, err := auth.GenerateTOTPEnrollment("Venu", user.Email, s.cfg.MFAEncryptionKey)
	if err != nil {
		return AdminMFAEnrollmentResult{}, fmt.Errorf("service_enroll_mfa_gen: %w", err)
	}
	if _, err := s.store.EnrollUserTOTPSecret(ctx, user.ID, enrollment.EncryptedSecret); err != nil {
		return AdminMFAEnrollmentResult{}, fmt.Errorf("service_enroll_mfa_persist: %w", err)
	}

	return AdminMFAEnrollmentResult{
		OtpAuthURI: enrollment.OtpAuthURI,
		Secret:     enrollment.PlaintextSecret,
		Issuer:     "Venu",
		Algorithm:  "SHA1",
		Digits:     6,
		Period:     30,
	}, nil
}

// VerifyAdminMFA completes step-2 TOTP verification with ±1 clock-skew tolerance and atomic
// high-water-mark replay protection before issuing Access + Refresh JWTs.
func (s *TicketingService) VerifyAdminMFA(ctx context.Context, mfaToken, totpCode string) (AuthResult, error) {
	claims, err := s.tokens.VerifyMFAChallengeToken(mfaToken)
	if err != nil {
		return AuthResult{}, httpx.NewUnauthorized("MFA session expired or invalid", err)
	}

	user, err := s.store.VerifyAndAdvanceTOTPStepTx(ctx, claims.UserID, func(u db.User) (int64, error) {
		if u.Role != db.UserRoleADMIN || !u.IsActive || u.TotpSecret == nil {
			return 0, httpx.NewForbidden("admin MFA not provisioned")
		}
		plainSecret, decErr := auth.DecryptTOTPSecret(*u.TotpSecret, s.cfg.MFAEncryptionKey)
		if decErr != nil {
			return 0, fmt.Errorf("service_mfa_decrypt_secret: %w", decErr)
		}
		return auth.VerifyTOTPWithStep(plainSecret, totpCode, u.TotpLastAcceptedStep, time.Now().UTC())
	})
	if err != nil {
		var appErr *httpx.AppError
		if errors.As(err, &appErr) {
			return AuthResult{}, err
		}
		if errors.Is(err, auth.ErrTOTPCodeReused) {
			return AuthResult{}, httpx.NewUnauthorized("TOTP code already used; wait for the next 30-second code", err)
		}
		if errors.Is(err, auth.ErrInvalidTOTPCode) {
			return AuthResult{}, httpx.NewUnauthorized("invalid authenticator TOTP code", err)
		}
		return AuthResult{}, fmt.Errorf("service_mfa_verify: %w", err)
	}

	return s.issueSession(ctx, user, uuid.New())
}

func (s *TicketingService) RefreshSession(ctx context.Context, rawRefreshToken string) (AuthResult, error) {
	if strings.TrimSpace(rawRefreshToken) == "" {
		return AuthResult{}, httpx.NewUnauthorized("missing refresh token", nil)
	}
	incomingHash := auth.HashRefreshToken(rawRefreshToken)
	nextRaw, nextHash, err := auth.GenerateOpaqueRefreshToken()
	if err != nil {
		return AuthResult{}, fmt.Errorf("service_refresh_gen: %w", err)
	}

	user, _, err := s.store.RotateRefreshTokenTx(
		ctx,
		incomingHash,
		nextHash,
		time.Now().UTC().Add(s.cfg.RefreshTokenTTL),
	)
	if err != nil {
		if errors.Is(err, repositories.ErrTokenReusedOrRevoked) {
			return AuthResult{}, httpx.NewUnauthorized("session expired or refresh token revoked", err)
		}
		return AuthResult{}, fmt.Errorf("service_refresh_rotate: %w", err)
	}

	accessTok, exp, err := s.tokens.CreateAccessToken(user)
	if err != nil {
		return AuthResult{}, fmt.Errorf("service_refresh_issue_access: %w", err)
	}

	return AuthResult{
		User:            user,
		AccessToken:     accessTok,
		RefreshToken:    nextRaw,
		AccessExpiresAt: exp,
	}, nil
}

func (s *TicketingService) Logout(ctx context.Context, rawRefreshToken string) error {
	if strings.TrimSpace(rawRefreshToken) == "" {
		return nil
	}
	return s.store.RevokeTokenByHash(ctx, auth.HashRefreshToken(rawRefreshToken))
}

// --- Events & Seat Selection ---

func (s *TicketingService) ListActiveEvents(ctx context.Context) ([]db.Event, error) {
	return s.store.ListActiveEvents(ctx)
}

func (s *TicketingService) GetEventDetail(ctx context.Context, eventID uuid.UUID) (db.Event, []db.EventSection, error) {
	evt, err := s.store.GetEventByID(ctx, eventID)
	if err != nil {
		return db.Event{}, nil, fmt.Errorf("service_get_event: %w", err)
	}
	secs, err := s.store.ListEventSections(ctx, eventID)
	if err != nil {
		return db.Event{}, nil, fmt.Errorf("service_list_sections: %w", err)
	}
	return evt, secs, nil
}

func (s *TicketingService) GetSeatMap(ctx context.Context, eventID uuid.UUID, currentUserID *uuid.UUID) (SeatMapPayload, error) {
	evt, err := s.store.GetEventByID(ctx, eventID)
	if err != nil {
		return SeatMapPayload{}, fmt.Errorf("service_seatmap_event: %w", err)
	}
	secs, err := s.store.ListEventSections(ctx, eventID)
	if err != nil {
		return SeatMapPayload{}, fmt.Errorf("service_seatmap_sections: %w", err)
	}
	rawSeats, err := s.store.ListSeatsByEvent(ctx, eventID)
	if err != nil {
		return SeatMapPayload{}, fmt.Errorf("service_seatmap_seats: %w", err)
	}

	now := time.Now().UTC()
	var mySeatIDs []uuid.UUID
	var holdExpiry *time.Time
	seatsDTO := make([]SeatDTO, 0, len(rawSeats))

	for _, seat := range rawSeats {
		status := string(seat.Status)
		if seat.Status == db.SeatStatusHELD {
			if seat.HoldExpiresAt != nil && seat.HoldExpiresAt.Before(now) {
				status = string(db.SeatStatusAVAILABLE)
			} else if currentUserID != nil && seat.HeldByUserID != nil && *seat.HeldByUserID == *currentUserID {
				status = "SELECTED_BY_ME"
				mySeatIDs = append(mySeatIDs, seat.ID)
				if holdExpiry == nil && seat.HoldExpiresAt != nil {
					exp := *seat.HoldExpiresAt
					holdExpiry = &exp
				}
			}
		}
		seatsDTO = append(seatsDTO, SeatDTO{
			ID:        seat.ID,
			EventID:   seat.EventID,
			SectionID: seat.SectionID,
			Row:       seat.RowLabel,
			Number:    seat.SeatNumber,
			Label:     seat.Label,
			Tier:      seat.Tier,
			Status:    status,
			X:         seat.PosX,
			Y:         seat.PosY,
		})
	}

	var breakdown OrderPricing
	if len(mySeatIDs) > 0 {
		quotes, err := s.store.QuoteSeats(ctx, eventID, mySeatIDs)
		if err != nil {
			return SeatMapPayload{}, fmt.Errorf("service_seatmap_quote: %w", err)
		}
		breakdown = s.ComputeAuthoritativePricing(eventID, quotes, holdExpiry)
	} else {
		breakdown = OrderPricing{EventID: eventID, SeatIDs: []uuid.UUID{}}
	}

	return SeatMapPayload{
		Event:     evt,
		Sections:  secs,
		Seats:     seatsDTO,
		Breakdown: breakdown,
	}, nil
}

func (s *TicketingService) HoldSeats(ctx context.Context, eventID uuid.UUID, seatIDs []uuid.UUID, userID uuid.UUID) (SeatMapPayload, error) {
	if len(seatIDs) == 0 || len(seatIDs) > s.cfg.MaxSeatsPerOrder {
		return SeatMapPayload{}, httpx.NewBadRequest(fmt.Sprintf("select between 1 and %d seats", s.cfg.MaxSeatsPerOrder), nil)
	}
	err := s.store.HoldSeatsAtomicallyTx(ctx, eventID, seatIDs, userID, s.cfg.SeatHoldTTL, s.cfg.MaxSeatsPerOrder)
	if err != nil {
		if errors.Is(err, repositories.ErrSeatUnavailable) {
			return SeatMapPayload{}, httpx.NewConflict("seat no longer available", err)
		}
		if errors.Is(err, repositories.ErrMaxSeatsExceeded) {
			return SeatMapPayload{}, httpx.NewBadRequest(fmt.Sprintf("maximum %d seats per order", s.cfg.MaxSeatsPerOrder), err)
		}
		return SeatMapPayload{}, fmt.Errorf("service_hold_seats: %w", err)
	}
	return s.GetSeatMap(ctx, eventID, &userID)
}

func (s *TicketingService) ClearHolds(ctx context.Context, eventID, userID uuid.UUID) (SeatMapPayload, error) {
	if err := s.store.ClearUserHolds(ctx, eventID, userID); err != nil {
		return SeatMapPayload{}, fmt.Errorf("service_clear_holds: %w", err)
	}
	return s.GetSeatMap(ctx, eventID, &userID)
}

func (s *TicketingService) QuoteSeats(ctx context.Context, eventID uuid.UUID, seatIDs []uuid.UUID) (OrderPricing, error) {
	if len(seatIDs) == 0 {
		return OrderPricing{EventID: eventID, SeatIDs: []uuid.UUID{}}, nil
	}
	quotes, err := s.store.QuoteSeats(ctx, eventID, seatIDs)
	if err != nil {
		return OrderPricing{}, fmt.Errorf("service_quote_seats: %w", err)
	}
	return s.ComputeAuthoritativePricing(eventID, quotes, nil), nil
}

// --- Checkout & Payment Confirmation ---

func (s *TicketingService) InitCheckoutPay(ctx context.Context, eventID, userID uuid.UUID, seatIDs []uuid.UUID, contactEmail, contactPhone string, attendees []AttendeeInput) (CheckoutInitResult, error) {
	if len(seatIDs) == 0 || len(seatIDs) > s.cfg.MaxSeatsPerOrder {
		return CheckoutInitResult{}, httpx.NewBadRequest("invalid seat count for checkout", nil)
	}

	quotes, err := s.store.QuoteSeats(ctx, eventID, seatIDs)
	if err != nil {
		return CheckoutInitResult{}, fmt.Errorf("service_checkout_quote: %w", err)
	}
	if len(quotes) != len(seatIDs) {
		return CheckoutInitResult{}, httpx.NewBadRequest("one or more seats not found for event", nil)
	}

	now := time.Now().UTC()
	for _, q := range quotes {
		if q.Status == db.SeatStatusSOLD {
			return CheckoutInitResult{}, httpx.NewConflict("one or more seats already sold", nil)
		}
		if q.Status == db.SeatStatusHELD && (q.HeldByUserID == nil || *q.HeldByUserID != userID || (q.HoldExpiresAt != nil && q.HoldExpiresAt.Before(now))) {
			return CheckoutInitResult{}, httpx.NewConflict("seat hold expired or owned by another user", nil)
		}
	}

	if len(attendees) > 0 {
		if err := ValidateAgeCompliance(quotes, attendees); err != nil {
			return CheckoutInitResult{}, httpx.NewBadRequest(err.Error(), err)
		}
		if err := ValidateCoupleComposition(quotes, attendees); err != nil {
			return CheckoutInitResult{}, httpx.NewBadRequest(err.Error(), err)
		}
	}

	exp := now.Add(s.cfg.SeatHoldTTL)
	pricing := s.ComputeAuthoritativePricing(eventID, quotes, &exp)
	amountPaise := int64(pricing.TotalPayable) * 100

	razorpayOrderID := fmt.Sprintf("order_VNU_%s", strings.ToUpper(uuid.NewString()[:8]))
	if s.razorpayClient != nil {
		rpBody, rpErr := s.razorpayClient.Order.Create(map[string]any{
			"amount":   amountPaise,
			"currency": "INR",
			"receipt":  razorpayOrderID,
		}, nil)
		if rpErr != nil {
			return CheckoutInitResult{}, fmt.Errorf("service_razorpay_create_order: %w", rpErr)
		}
		if idStr, ok := rpBody["id"].(string); ok && idStr != "" {
			razorpayOrderID = idStr
		}
	}

	orderSeats := make([]db.CreateOrderSeatParams, 0, len(attendees))
	for _, a := range attendees {
		g := strings.ToUpper(strings.TrimSpace(a.Gender))
		if g != "MALE" && g != "FEMALE" {
			g = "OTHER"
		}
		orderSeats = append(orderSeats, db.CreateOrderSeatParams{
			SeatID:         a.SeatID,
			AttendeeName:   a.FullName,
			AttendeeAge:    a.Age,
			AttendeeGender: g,
		})
	}
	if _, err := s.store.CreatePendingOrderTx(ctx, db.CreateOrderParams{
		UserID:          userID,
		EventID:         eventID,
		RazorpayOrderID: razorpayOrderID,
		BaseAmount:      pricing.BaseAmount,
		ConvenienceFee:  pricing.ConvenienceFee,
		GstAmount:       pricing.GSTAmount,
		TotalPayable:    pricing.TotalPayable,
		ContactEmail:    contactEmail,
		ContactPhone:    contactPhone,
	}, orderSeats); err != nil {
		return CheckoutInitResult{}, fmt.Errorf("service_persist_pending_order: %w", err)
	}

	return CheckoutInitResult{
		RazorpayOrderID: razorpayOrderID,
		AmountPaise:     amountPaise,
		Currency:        "INR",
		Breakdown:       pricing,
	}, nil
}

// VerifyRazorpaySignature uses official Razorpay SDK webhook signature verification.
func (s *TicketingService) VerifyRazorpaySignature(payload []byte, signature string) error {
	if !rpputils.VerifyWebhookSignature(string(payload), signature, s.cfg.RazorpayWebhookSecret) {
		return ErrInvalidWebhookHMAC
	}
	return nil
}

func (s *TicketingService) HandleRazorpayWebhook(
	ctx context.Context,
	rawBody []byte,
	signature string,
	razorpayOrderID string,
	phone string,
	eventTitle string,
	gateLabel string,
	fnbCredit int32,
) (db.Ticket, error) {
	if err := s.VerifyRazorpaySignature(rawBody, signature); err != nil {
		return db.Ticket{}, httpx.NewUnauthorized("invalid webhook signature", err)
	}

	ttl := s.cfg.TicketQRTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)

	var encOtpSecret *string
	if s.cfg.RotatingQREnabled {
		rawSecret, genErr := auth.GenerateRandomBase32Secret()
		if genErr != nil {
			return db.Ticket{}, fmt.Errorf("service_webhook_gen_qr_otp: %w", genErr)
		}
		enc, encErr := auth.EncryptTOTPSecret(rawSecret, s.cfg.MFAEncryptionKey)
		if encErr != nil {
			return db.Ticket{}, fmt.Errorf("service_webhook_enc_qr_otp: %w", encErr)
		}
		encOtpSecret = &enc
	}

	ticketCode := fmt.Sprintf("VNU-%s", strings.ToUpper(uuid.NewString()[:6]))
	barCode := fmt.Sprintf("BAR-%s", strings.ToUpper(uuid.NewString()[:4]))
	_, ticket, err := s.store.ConfirmOrderWebhookTx(
		ctx,
		razorpayOrderID,
		ticketCode,
		barCode,
		gateLabel,
		fnbCredit,
		expiresAt,
		encOtpSecret,
		s.tokens.ComputeTicketDigest,
	)
	if err != nil {
		if errors.Is(err, repositories.ErrOrderNotPending) {
			return db.Ticket{}, httpx.NewConflict("order already processed or not found", err)
		}
		return db.Ticket{}, fmt.Errorf("service_webhook_confirm_order: %w", err)
	}

	if s.mailer != nil {
		s.mailer.EnqueuePassNotification(ctx, mailer.TicketNotification{
			Phone:      phone,
			EventTitle: eventTitle,
			TicketID:   ticket.ID.String(),
			TicketCode: ticket.Code,
		})
	}

	return ticket, nil
}

func (s *TicketingService) ListUserTickets(ctx context.Context, userID uuid.UUID) ([]UserTicketDTO, error) {
	rows, err := s.store.ListTicketsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]UserTicketDTO, 0, len(rows))
	for _, r := range rows {
		exp := time.Now().UTC().Add(24 * time.Hour)
		if r.ExpiresAt != nil {
			exp = *r.ExpiresAt
		}
		var plainOtp *string
		if r.QrOtpSecret != nil && s.cfg.RotatingQREnabled {
			if dec, decErr := auth.DecryptTOTPSecret(*r.QrOtpSecret, s.cfg.MFAEncryptionKey); decErr == nil {
				plainOtp = &dec
			}
		}
		tok := ""
		if s.tokens != nil {
			tok, _ = s.tokens.CreateTicketQRToken(r.ID, r.OrderID, r.EventID, r.Code, exp, plainOtp)
		}
		out = append(out, UserTicketDTO{
			ListTicketsByUserRow: r,
			QrToken:              tok,
		})
	}
	return out, nil
}

// GenerateTicketQR re-generates an HMAC-SHA256-signed QR token on demand (never stored plaintext in DB).
func (s *TicketingService) GenerateTicketQR(ctx context.Context, ticketID, userID uuid.UUID, rotating bool) (TicketQRResult, error) {
	tkt, err := s.store.GetTicketByID(ctx, ticketID)
	if err != nil {
		if errors.Is(err, repositories.ErrTicketNotFound) {
			return TicketQRResult{}, httpx.NewNotFound("ticket not found")
		}
		return TicketQRResult{}, fmt.Errorf("service_get_ticket_qr: %w", err)
	}
	if tkt.UserID != userID {
		return TicketQRResult{}, httpx.NewForbidden("ticket does not belong to current user")
	}

	exp := time.Now().UTC().Add(24 * time.Hour)
	if tkt.ExpiresAt != nil {
		exp = *tkt.ExpiresAt
	}

	var plainOtp *string
	useRotating := false
	if (rotating || s.cfg.RotatingQREnabled) && tkt.QrOtpSecret != nil {
		dec, decErr := auth.DecryptTOTPSecret(*tkt.QrOtpSecret, s.cfg.MFAEncryptionKey)
		if decErr != nil {
			return TicketQRResult{}, fmt.Errorf("service_decrypt_ticket_otp: %w", decErr)
		}
		plainOtp = &dec
		useRotating = true
	}

	tok, err := s.tokens.CreateTicketQRToken(tkt.ID, tkt.OrderID, tkt.EventID, tkt.Code, exp, plainOtp)
	if err != nil {
		return TicketQRResult{}, fmt.Errorf("service_sign_ticket_qr: %w", err)
	}

	res := TicketQRResult{
		TicketID: tkt.ID,
		Code:     tkt.Code,
		QrToken:  tok,
		Rotating: useRotating,
	}
	if useRotating {
		rem := 30 - int(time.Now().UTC().Unix()%30)
		res.ExpiresInSeconds = rem
	}
	return res, nil
}

// --- Gate Scanner (RBAC: ADMIN, SCANNER) ---

func (s *TicketingService) ValidateScanTicket(ctx context.Context, query string) (ScanLookupResult, error) {
	cleaned := strings.TrimSpace(query)
	if cleaned == "" {
		return ScanLookupResult{Valid: false, Reason: "NOT_FOUND"}, nil
	}

	now := time.Now().UTC()
	var verifiedPayload *auth.TicketQRPayload
	lookupKey := cleaned

	if strings.Contains(cleaned, ".") {
		payload, err := s.tokens.VerifyTicketQRToken(cleaned, now)
		if err != nil {
			if errors.Is(err, auth.ErrTicketTokenExpired) {
				return ScanLookupResult{Valid: false, Reason: "EXPIRED"}, nil
			}
			return ScanLookupResult{Valid: false, Reason: "INVALID_SIGNATURE"}, nil
		}
		verifiedPayload = payload
		lookupKey = s.tokens.ComputeTicketDigest(payload.TicketID)
	}

	tkt, err := s.store.LookupTicketByCodeOrDigest(ctx, lookupKey)
	if err != nil {
		if errors.Is(err, repositories.ErrTicketNotFound) {
			return ScanLookupResult{Valid: false, Reason: "NOT_FOUND"}, nil
		}
		return ScanLookupResult{}, fmt.Errorf("service_scan_lookup: %w", err)
	}

	if verifiedPayload != nil {
		expectedDigest := s.tokens.ComputeTicketDigest(verifiedPayload.TicketID)
		if subtle.ConstantTimeCompare([]byte(tkt.QrTokenDigest), []byte(expectedDigest)) != 1 {
			return ScanLookupResult{Valid: false, Reason: "INVALID_SIGNATURE"}, nil
		}
		if verifiedPayload.RotatingCode != "" && tkt.QrOtpSecret != nil {
			plainOtp, decErr := auth.DecryptTOTPSecret(*tkt.QrOtpSecret, s.cfg.MFAEncryptionKey)
			if decErr != nil {
				return ScanLookupResult{}, fmt.Errorf("service_scan_decrypt_otp: %w", decErr)
			}
			if _, totpErr := auth.VerifyTOTPWithStep(plainOtp, verifiedPayload.RotatingCode, nil, now); totpErr != nil {
				return ScanLookupResult{Valid: false, Reason: "ROTATING_QR_EXPIRED"}, nil
			}
		}
	}

	if tkt.ExpiresAt != nil && !tkt.ExpiresAt.After(now) {
		return ScanLookupResult{Valid: false, Reason: "EXPIRED"}, nil
	}

	attendees, err := s.store.ListOrderAttendees(ctx, tkt.OrderID)
	if err != nil {
		return ScanLookupResult{}, fmt.Errorf("service_scan_attendees: %w", err)
	}
	if tkt.CheckedIn {
		return ScanLookupResult{
			Valid:     false,
			Reason:    "ALREADY_CHECKED_IN",
			Ticket:    &tkt,
			Attendees: attendees,
		}, nil
	}
	return ScanLookupResult{
		Valid:     true,
		Reason:    "VALID",
		Ticket:    &tkt,
		Attendees: attendees,
	}, nil
}

func (s *TicketingService) CheckInTicket(ctx context.Context, ticketID uuid.UUID, seatID *uuid.UUID, scannerUserID uuid.UUID) (db.Ticket, []db.ListOrderAttendeesRow, error) {
	tkt, attendees, err := s.store.AdmitTicketTx(ctx, ticketID, seatID, scannerUserID)
	if err != nil {
		if errors.Is(err, repositories.ErrTicketNotFound) {
			return db.Ticket{}, nil, httpx.NewNotFound("ticket not found")
		}
		if errors.Is(err, repositories.ErrAlreadyCheckedIn) {
			return db.Ticket{}, nil, httpx.NewConflict("ticket already checked in", err)
		}
		return db.Ticket{}, nil, fmt.Errorf("service_checkin_ticket: %w", err)
	}
	return tkt, attendees, nil
}

// --- Bartender F&B Counter (RBAC: ADMIN, BARTENDER) ---

func ComputeFnbSplit(walletBalance, cartTotal int32) (walletDeduct int32, upiDelta int32) {
	if walletBalance >= cartTotal {
		return cartTotal, 0
	}
	return walletBalance, cartTotal - walletBalance
}

func (s *TicketingService) ValidateCounterTicket(ctx context.Context, query string) (ScanLookupResult, error) {
	cleaned := strings.TrimSpace(query)
	if cleaned == "" {
		return ScanLookupResult{Valid: false, Reason: "NOT_FOUND"}, nil
	}
	tkt, err := s.store.LookupTicketByCodeOrDigest(ctx, cleaned)
	if err != nil {
		if errors.Is(err, repositories.ErrTicketNotFound) {
			return ScanLookupResult{Valid: false, Reason: "NOT_FOUND"}, nil
		}
		return ScanLookupResult{}, fmt.Errorf("service_counter_lookup: %w", err)
	}
	attendees, err := s.store.ListOrderAttendees(ctx, tkt.OrderID)
	if err != nil {
		return ScanLookupResult{}, fmt.Errorf("service_counter_attendees: %w", err)
	}
	if tkt.ExpiresAt != nil && !tkt.ExpiresAt.After(time.Now().UTC()) {
		return ScanLookupResult{Valid: false, Reason: "EXPIRED", Ticket: &tkt, Attendees: attendees}, nil
	}
	return ScanLookupResult{Valid: true, Reason: "VALID", Ticket: &tkt, Attendees: attendees}, nil
}

func (s *TicketingService) QuoteCounterOrder(ctx context.Context, ticketID *uuid.UUID, entries []FnbCartEntry) (FnbQuoteResult, error) {
	var walletBalance int32
	if ticketID != nil && *ticketID != uuid.Nil {
		tkt, err := s.store.GetTicketByID(ctx, *ticketID)
		if err != nil {
			if errors.Is(err, repositories.ErrTicketNotFound) {
				return FnbQuoteResult{}, httpx.NewNotFound("ticket pass not found")
			}
			return FnbQuoteResult{}, fmt.Errorf("service_quote_ticket: %w", err)
		}
		if tkt.ExpiresAt != nil && !tkt.ExpiresAt.After(time.Now().UTC()) {
			return FnbQuoteResult{}, httpx.NewConflict("ticket has expired and F&B credits are no longer redeemable", repositories.ErrTicketExpired)
		}
		walletBalance = tkt.FoodBalance
	}

	if len(entries) == 0 {
		return FnbQuoteResult{
			TicketID:         ticketID,
			WalletBalance:    walletBalance,
			RemainingBalance: walletBalance,
		}, nil
	}

	ids := make([]uuid.UUID, 0, len(entries))
	qtyByID := make(map[uuid.UUID]int32, len(entries))
	for _, e := range entries {
		if e.Quantity <= 0 {
			return FnbQuoteResult{}, httpx.NewBadRequest("item quantity must be positive", nil)
		}
		ids = append(ids, e.ItemID)
		qtyByID[e.ItemID] += e.Quantity
	}

	menuItems, err := s.store.GetFnbMenuItemsByIDs(ctx, ids)
	if err != nil {
		return FnbQuoteResult{}, fmt.Errorf("service_quote_fnb_menu: %w", err)
	}
	var cartTotal int32
	for _, item := range menuItems {
		cartTotal += item.Price * qtyByID[item.ID]
	}
	walletDeduct, upiDelta := ComputeFnbSplit(walletBalance, cartTotal)
	return FnbQuoteResult{
		TicketID:         ticketID,
		WalletBalance:    walletBalance,
		CartTotal:        cartTotal,
		WalletDeduction:  walletDeduct,
		UpiDeltaRequired: upiDelta,
		RemainingBalance: walletBalance - walletDeduct,
		ExceedsWallet:    upiDelta > 0,
	}, nil
}

func (s *TicketingService) CreateFnbMenuItem(ctx context.Context, in CreateFnbMenuItemInput) (db.FnbMenuItem, error) {
	if strings.TrimSpace(in.Name) == "" || in.Price <= 0 {
		return db.FnbMenuItem{}, httpx.NewBadRequest("item name and positive price are required", nil)
	}
	switch in.Category {
	case "Cocktails", "Zero-Proof", "Small Plates":
	default:
		return db.FnbMenuItem{}, httpx.NewBadRequest("category must be Cocktails, Zero-Proof, or Small Plates", nil)
	}
	return s.store.CreateFnbMenuItem(ctx, db.CreateFnbMenuItemParams{
		Name:        strings.TrimSpace(in.Name),
		Category:    in.Category,
		Price:       in.Price,
		IsAlcoholic: in.IsAlcoholic,
	})
}

func (s *TicketingService) RedeemCounterBalance(
	ctx context.Context,
	ticketID uuid.UUID,
	bartenderUserID uuid.UUID,
	entries []FnbCartEntry,
	upiDeltaConfirmed bool,
) (FnbSettleResult, error) {
	if len(entries) == 0 {
		return FnbSettleResult{}, httpx.NewBadRequest("at least one menu item is required", nil)
	}
	ids := make([]uuid.UUID, 0, len(entries))
	qtyByID := make(map[uuid.UUID]int32, len(entries))
	for _, e := range entries {
		if e.Quantity <= 0 {
			return FnbSettleResult{}, httpx.NewBadRequest("item quantity must be positive", nil)
		}
		ids = append(ids, e.ItemID)
		qtyByID[e.ItemID] += e.Quantity
	}

	menuItems, err := s.store.GetFnbMenuItemsByIDs(ctx, ids)
	if err != nil {
		return FnbSettleResult{}, fmt.Errorf("service_fnb_menu: %w", err)
	}
	if len(menuItems) == 0 {
		return FnbSettleResult{}, httpx.NewBadRequest("no valid F&B menu items found", nil)
	}

	var cartTotal int32
	summaryParts := make([]string, 0, len(menuItems))
	for _, item := range menuItems {
		qty := qtyByID[item.ID]
		cartTotal += item.Price * qty
		summaryParts = append(summaryParts, fmt.Sprintf("%dx %s", qty, item.Name))
	}

	lookup, err := s.store.LookupTicketByCodeOrDigest(ctx, ticketID.String())
	if err != nil {
		if errors.Is(err, repositories.ErrTicketNotFound) {
			return FnbSettleResult{}, httpx.NewNotFound("ticket pass not found")
		}
		return FnbSettleResult{}, fmt.Errorf("service_fnb_ticket_lookup: %w", err)
	}
	if lookup.ExpiresAt != nil && !lookup.ExpiresAt.After(time.Now().UTC()) {
		return FnbSettleResult{}, httpx.NewConflict("ticket has expired and F&B credits are no longer redeemable", repositories.ErrTicketExpired)
	}

	walletDeduct, upiDelta := ComputeFnbSplit(lookup.FoodBalance, cartTotal)
	if upiDelta > 0 && !upiDeltaConfirmed {
		return FnbSettleResult{}, httpx.NewConflict(
			fmt.Sprintf("order exceeds wallet by INR %d; confirm UPI delta settlement", upiDelta),
			ErrUpiDeltaUnconfirmed,
		)
	}

	updatedTicket, txRecord, err := s.store.DeductFnBWalletTx(
		ctx,
		ticketID,
		bartenderUserID,
		strings.Join(summaryParts, ", "),
		cartTotal,
		walletDeduct,
		upiDelta,
	)
	if err != nil {
		if errors.Is(err, repositories.ErrTicketExpired) {
			return FnbSettleResult{}, httpx.NewConflict("ticket has expired and F&B credits are no longer redeemable", err)
		}
		if errors.Is(err, repositories.ErrInsufficientFnb) {
			return FnbSettleResult{}, httpx.NewConflict("concurrent wallet deduction exceeded balance", err)
		}
		return FnbSettleResult{}, fmt.Errorf("service_fnb_deduct: %w", err)
	}

	return FnbSettleResult{
		Ticket:      updatedTicket,
		Transaction: txRecord,
	}, nil
}

// --- Admin Operations (RBAC: ADMIN) ---

func (s *TicketingService) ListRecentScans(ctx context.Context, limit int32) ([]db.ListRecentScansRow, error) {
	return s.store.ListRecentScans(ctx, limit)
}

func (s *TicketingService) ListFnbMenu(ctx context.Context) ([]db.FnbMenuItem, error) {
	return s.store.ListFnbMenuItems(ctx)
}

func (s *TicketingService) ListRecentFnbTransactions(ctx context.Context, limit int32) ([]db.ListRecentFnbTransactionsRow, error) {
	return s.store.ListRecentFnbTransactions(ctx, limit)
}

func (s *TicketingService) GetAdminStats(ctx context.Context) (db.GetAdminOverviewStatsRow, error) {
	return s.store.GetAdminOverviewStats(ctx)
}

func (s *TicketingService) CreateEvent(ctx context.Context, in CreateEventInput) (db.Event, []db.EventSection, error) {
	if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.VenueName) == "" {
		return db.Event{}, nil, httpx.NewBadRequest("title and venueName are required", nil)
	}
	if in.LayoutType != "PROSCENIUM" && in.LayoutType != "CLUB" {
		return db.Event{}, nil, httpx.NewBadRequest("layoutType must be PROSCENIUM or CLUB", nil)
	}

	slugBase := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(in.Title), " ", "-"))
	slug := fmt.Sprintf("%s-%s", slugBase, uuid.NewString()[:6])

	layoutJSON, _ := json.Marshal(map[string]any{"rows": []string{"A", "B", "C"}, "cols": 8})
	return s.store.CreateEventWithInventoryTx(ctx, db.CreateEventParams{
		Slug:         slug,
		Title:        in.Title,
		Subtitle:     in.Subtitle,
		Category:     in.Category,
		VenueName:    in.VenueName,
		VenueCity:    in.VenueCity,
		VenueAddress: in.VenueAddress,
		TransitNote:  "",
		LayoutType:   in.LayoutType,
		LayoutJson:   layoutJSON,
		ImageUrl:     "",
		DateLabel:    in.DateLabel,
		TimeLabel:    in.TimeLabel,
		DoorsOpen:    "Doors open 45m prior",
		StartsAt:     time.Now().UTC().Add(7 * 24 * time.Hour),
		Status:       "ON_SALE",
	}, in.VIPPrice, in.GeneralPrice, in.VIPFnbCredit, in.VIPMinAge)
}

func (s *TicketingService) UpdateEventStatus(ctx context.Context, eventID uuid.UUID, status string) (db.Event, error) {
	switch status {
	case "ON_SALE", "ALMOST_FULL", "SOLD_OUT", "DRAFT":
	default:
		return db.Event{}, httpx.NewBadRequest("invalid event status", nil)
	}
	return s.store.UpdateEventStatus(ctx, eventID, status)
}

func (s *TicketingService) UpdateEventLayout(ctx context.Context, eventID uuid.UUID, layoutType string, layoutJSON json.RawMessage) (db.Event, error) {
	if layoutType != "PROSCENIUM" && layoutType != "CLUB" {
		return db.Event{}, httpx.NewBadRequest("layoutType must be PROSCENIUM or CLUB", nil)
	}
	return s.store.UpdateEventLayout(ctx, eventID, layoutType, layoutJSON)
}

func (s *TicketingService) ListRecentOrders(ctx context.Context, limit int32) ([]db.Order, error) {
	return s.store.ListRecentOrders(ctx, limit)
}

func (s *TicketingService) UpdateUserRole(ctx context.Context, targetUserID uuid.UUID, newRole db.UserRole) (db.User, error) {
	switch newRole {
	case db.UserRoleUSER, db.UserRoleADMIN, db.UserRoleSCANNER, db.UserRoleBARTENDER:
	default:
		return db.User{}, httpx.NewBadRequest("invalid user role", nil)
	}
	return s.store.UpdateUserRole(ctx, targetUserID, newRole)
}

func (s *TicketingService) ListStaffUsers(ctx context.Context) ([]db.StaffUserRow, error) {
	return s.store.ListStaffUsers(ctx)
}

func (s *TicketingService) CreateStaffUser(ctx context.Context, in CreateStaffUserInput) (db.StaffUserRow, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Email) == "" || len(in.Password) < 6 {
		return db.StaffUserRow{}, httpx.NewBadRequest("name, email, and password (min 6 chars) are required", nil)
	}
	if in.Role != db.UserRoleSCANNER && in.Role != db.UserRoleBARTENDER {
		return db.StaffUserRow{}, httpx.NewBadRequest("staff role must be SCANNER or BARTENDER", nil)
	}
	hash, err := auth.HashArgon2id(in.Password)
	if err != nil {
		return db.StaffUserRow{}, fmt.Errorf("service_create_staff_hash: %w", err)
	}
	return s.store.CreateStaffUser(ctx, db.CreateStaffUserParams{
		Name:         strings.TrimSpace(in.Name),
		Email:        strings.TrimSpace(in.Email),
		PasswordHash: &hash,
		Role:         in.Role,
	})
}
