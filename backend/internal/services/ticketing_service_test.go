package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/config"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/mailer"
	"github.com/venu-live/venu-api/internal/repositories"
	"golang.org/x/crypto/argon2"
)

type stubTokenVerifier struct {
	identity *auth.GoogleIdentity
	err      error
}

func (s *stubTokenVerifier) VerifyGoogleIDToken(ctx context.Context, idToken string) (*auth.GoogleIdentity, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.identity, nil
}

type spyMailer struct {
	sent []mailer.TicketNotification
}

func (m *spyMailer) EnqueuePassNotification(ctx context.Context, msg mailer.TicketNotification) {
	m.sent = append(m.sent, msg)
}

func (m *spyMailer) CloseAndWait(ctx context.Context) error {
	return nil
}

type mockStore struct {
	repositories.TicketingStore

	usersByEmail map[string]db.User
	usersByID    map[uuid.UUID]db.User
	tickets      map[uuid.UUID]db.Ticket
	menuItems    []db.FnbMenuItem

	errOnGetByEmail error
	errOnListEvents error
}

func newMockStore() *mockStore {
	return &mockStore{
		usersByEmail: make(map[string]db.User),
		usersByID:    make(map[uuid.UUID]db.User),
		tickets:      make(map[uuid.UUID]db.Ticket),
	}
}

func (m *mockStore) GetUserByEmail(ctx context.Context, email string) (db.User, error) {
	if m.errOnGetByEmail != nil {
		return db.User{}, m.errOnGetByEmail
	}
	u, ok := m.usersByEmail[email]
	if !ok {
		return db.User{}, errors.New("not found")
	}
	return u, nil
}

func (m *mockStore) GetUserByID(ctx context.Context, id uuid.UUID) (db.User, error) {
	u, ok := m.usersByID[id]
	if !ok {
		return db.User{}, errors.New("not found")
	}
	return u, nil
}

func (m *mockStore) UpsertGoogleUser(ctx context.Context, arg db.UpsertGoogleUserParams) (db.User, error) {
	if existing, ok := m.usersByEmail[arg.Email]; ok {
		return existing, nil
	}
	u := db.User{
		ID:           uuid.New(),
		Name:         arg.Name,
		Email:        arg.Email,
		AuthProvider: db.AuthProviderGOOGLE,
		GoogleSub:    arg.GoogleSub,
		Role:         db.UserRoleUSER,
		IsActive:     true,
	}
	m.usersByEmail[u.Email] = u
	m.usersByID[u.ID] = u
	return u, nil
}

func (m *mockStore) CreateRefreshToken(ctx context.Context, arg db.CreateRefreshTokenParams) (db.RefreshToken, error) {
	return db.RefreshToken{
		ID:        uuid.New(),
		UserID:    arg.UserID,
		TokenHash: arg.TokenHash,
		FamilyID:  arg.FamilyID,
		ExpiresAt: arg.ExpiresAt,
	}, nil
}

func (m *mockStore) ListActiveEvents(ctx context.Context) ([]db.Event, error) {
	if m.errOnListEvents != nil {
		return nil, m.errOnListEvents
	}
	return []db.Event{}, nil
}

func (m *mockStore) EnrollUserTOTPSecret(ctx context.Context, userID uuid.UUID, encryptedSecret string) (db.User, error) {
	u, ok := m.usersByID[userID]
	if !ok {
		return db.User{}, errors.New("not found")
	}
	u.TotpSecret = &encryptedSecret
	u.TotpLastAcceptedStep = nil
	m.usersByID[u.ID] = u
	m.usersByEmail[u.Email] = u
	return u, nil
}

func (m *mockStore) VerifyAndAdvanceTOTPStepTx(ctx context.Context, userID uuid.UUID, verifyFn func(user db.User) (int64, error)) (db.User, error) {
	u, ok := m.usersByID[userID]
	if !ok {
		return db.User{}, errors.New("not found")
	}
	step, err := verifyFn(u)
	if err != nil {
		return db.User{}, err
	}
	if u.TotpLastAcceptedStep != nil && *u.TotpLastAcceptedStep >= step {
		return db.User{}, auth.ErrTOTPCodeReused
	}
	u.TotpLastAcceptedStep = &step
	m.usersByID[u.ID] = u
	m.usersByEmail[u.Email] = u
	return u, nil
}

func (m *mockStore) ConfirmOrderWebhookTx(
	ctx context.Context,
	razorpayOrderID, ticketCode, barCode, gateLabel string,
	fnbCredit int32,
	expiresAt time.Time,
	qrOtpSecret *string,
	computeDigest func(ticketID uuid.UUID) string,
) (db.Order, db.Ticket, error) {
	tid := uuid.New()
	eid := uuid.New()
	tkt := db.Ticket{
		ID:                 tid,
		OrderID:            uuid.New(),
		EventID:            eid,
		UserID:             uuid.New(),
		Code:               ticketCode,
		BarCode:            barCode,
		QrTokenDigest:      computeDigest(tid),
		QrOtpSecret:        qrOtpSecret,
		GateLabel:          gateLabel,
		FoodBalance:        fnbCredit,
		InitialFoodBalance: fnbCredit,
		ExpiresAt:          &expiresAt,
	}
	m.tickets[tkt.ID] = tkt
	return db.Order{ID: tkt.OrderID, EventID: eid, Status: db.OrderStatusCONFIRMED}, tkt, nil
}

func (m *mockStore) GetTicketByID(ctx context.Context, ticketID uuid.UUID) (db.Ticket, error) {
	tkt, ok := m.tickets[ticketID]
	if !ok {
		return db.Ticket{}, repositories.ErrTicketNotFound
	}
	return tkt, nil
}

func (m *mockStore) ListOrderAttendees(ctx context.Context, orderID uuid.UUID) ([]db.ListOrderAttendeesRow, error) {
	return []db.ListOrderAttendeesRow{}, nil
}

func (m *mockStore) AdmitTicketTx(ctx context.Context, ticketID uuid.UUID, seatID *uuid.UUID, scannerUserID uuid.UUID) (db.Ticket, []db.ListOrderAttendeesRow, error) {
	tkt, ok := m.tickets[ticketID]
	if !ok {
		return db.Ticket{}, nil, repositories.ErrTicketNotFound
	}
	if tkt.CheckedIn {
		return db.Ticket{}, nil, repositories.ErrAlreadyCheckedIn
	}
	now := time.Now().UTC()
	tkt.CheckedIn = true
	tkt.CheckedInAt = &now
	tkt.CheckedInBy = &scannerUserID
	m.tickets[ticketID] = tkt
	return tkt, []db.ListOrderAttendeesRow{}, nil
}

func (m *mockStore) GetFnbMenuItemsByIDs(ctx context.Context, ids []uuid.UUID) ([]db.FnbMenuItem, error) {
	return m.menuItems, nil
}

func (m *mockStore) LookupTicketByCodeOrDigest(ctx context.Context, query string) (db.GetTicketByCodeOrDigestRow, error) {
	for _, t := range m.tickets {
		if t.ID.String() == query || t.Code == query || t.QrTokenDigest == query {
			return db.GetTicketByCodeOrDigestRow{
				ID:                 t.ID,
				OrderID:            t.OrderID,
				EventID:            t.EventID,
				UserID:             t.UserID,
				Code:               t.Code,
				QrTokenDigest:      t.QrTokenDigest,
				QrOtpSecret:        t.QrOtpSecret,
				GateLabel:          t.GateLabel,
				FoodBalance:        t.FoodBalance,
				InitialFoodBalance: t.InitialFoodBalance,
				CheckedIn:          t.CheckedIn,
				CheckedInBy:        t.CheckedInBy,
				ExpiresAt:          t.ExpiresAt,
			}, nil
		}
	}
	return db.GetTicketByCodeOrDigestRow{}, repositories.ErrTicketNotFound
}

func (m *mockStore) DeductFnBWalletTx(
	ctx context.Context,
	ticketID, bartenderID uuid.UUID,
	summary string,
	totalAmount, walletDeduct, upiDelta int32,
) (db.Ticket, db.FnbTransaction, error) {
	tkt := m.tickets[ticketID]
	if tkt.FoodBalance < walletDeduct {
		return db.Ticket{}, db.FnbTransaction{}, repositories.ErrInsufficientFnb
	}
	tkt.FoodBalance -= walletDeduct
	m.tickets[ticketID] = tkt
	return tkt, db.FnbTransaction{
		ID:              uuid.New(),
		TicketID:        ticketID,
		BartenderUserID: bartenderID,
		ItemsSummary:    summary,
		TotalAmount:     totalAmount,
		WalletDeducted:  walletDeduct,
		UpiDeltaPaid:    upiDelta,
	}, nil
}

func testServiceConfig() *config.Config {
	return &config.Config{
		MFAEncryptionKey:      "test-mfa-envelope-key-32-bytes!!",
		ConvenienceFeeBps:     600,
		ConvenienceGstBps:     1800,
		EntertainmentTaxBps:   500,
		MaxSeatsPerOrder:      12,
		AccessTokenTTL:        15 * time.Minute,
		RefreshTokenTTL:       7 * 24 * time.Hour,
		MFATokenTTL:           5 * time.Minute,
		TicketQRTTL:           24 * time.Hour,
		SeatHoldTTL:           9 * time.Minute,
		DBQueryTimeout:        3 * time.Second,
		RazorpayWebhookSecret: "test_webhook_secret",
	}
}

func encodeTestArgon2id(password string) string {
	salt := []byte("venutestsalt1234")
	digest := argon2.IDKey([]byte(password), salt, 1, 16384, 1, 32)
	return fmt.Sprintf(
		"$argon2id$v=19$m=16384,t=1,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	)
}

func TestComputeAuthoritativePricing(t *testing.T) {
	svc := NewTicketingService(testServiceConfig(), newMockStore(), nil, nil, nil, nil)
	eventID := uuid.New()

	tests := []struct {
		name      string
		quotes    []db.QuoteSeatsByIDsRow
		wantBase  int32
		wantTotal int32
		wantFnB   int32
	}{
		{
			name:      "empty seats returns zero",
			quotes:    nil,
			wantBase:  0,
			wantTotal: 0,
			wantFnB:   0,
		},
		{
			name: "two seats compute configured BPS fee and GST",
			quotes: []db.QuoteSeatsByIDsRow{
				{ID: uuid.New(), SectionPrice: 1800, FnbCredit: 600},
				{ID: uuid.New(), SectionPrice: 1200, FnbCredit: 0},
			},
			wantBase:  3000,
			wantTotal: 3362,
			wantFnB:   600,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.ComputeAuthoritativePricing(eventID, tc.quotes, nil)
			if got.BaseAmount != tc.wantBase || got.TotalPayable != tc.wantTotal || got.FnBCreditTotal != tc.wantFnB {
				t.Fatalf("got %+v, want base=%d total=%d fnb=%d", got, tc.wantBase, tc.wantTotal, tc.wantFnB)
			}
		})
	}
}

func TestValidateAgeCompliance(t *testing.T) {
	vipSeat := uuid.New()
	genSeat := uuid.New()

	quotes := []db.QuoteSeatsByIDsRow{
		{ID: vipSeat, Label: "A-1", IsAlcoholic: true, MinAge: 21},
		{ID: genSeat, Label: "C-1", IsAlcoholic: false, MinAge: 0},
	}

	err := ValidateAgeCompliance(quotes, []AttendeeInput{
		{SeatID: vipSeat, Age: 19},
		{SeatID: genSeat, Age: 16},
	})
	if !errors.Is(err, ErrUnderageForAlcoholZone) {
		t.Fatalf("expected ErrUnderageForAlcoholZone, got %v", err)
	}

	if err := ValidateAgeCompliance(quotes, []AttendeeInput{
		{SeatID: vipSeat, Age: 23},
		{SeatID: genSeat, Age: 16},
	}); err != nil {
		t.Fatalf("expected nil error for compliant ages, got %v", err)
	}
}

func TestValidateCoupleComposition(t *testing.T) {
	c1 := uuid.New()
	c2 := uuid.New()
	quotes := []db.QuoteSeatsByIDsRow{
		{ID: c1, Label: "B-1", Tier: db.TierTypeCOUPLE},
		{ID: c2, Label: "B-2", Tier: db.TierTypeCOUPLE},
	}

	if err := ValidateCoupleComposition(quotes, []AttendeeInput{
		{SeatID: c1, Gender: "MALE"},
		{SeatID: c2, Gender: "MALE"},
	}); !errors.Is(err, ErrInvalidCoupleComposition) {
		t.Fatalf("expected ErrInvalidCoupleComposition for two males, got %v", err)
	}

	if err := ValidateCoupleComposition(quotes, []AttendeeInput{
		{SeatID: c1, Gender: "MALE"},
		{SeatID: c2, Gender: "FEMALE"},
	}); err != nil {
		t.Fatalf("expected 1 male + 1 female couple pair to pass, got %v", err)
	}
}

func TestAuthFlows_GoogleAndAdminStaffMFA(t *testing.T) {
	cfg := testServiceConfig()
	store := newMockStore()
	tokens := auth.NewTokenManager("access-secret-123", "qr-secret-456", cfg.AccessTokenTTL, cfg.MFATokenTTL)
	passHash := encodeTestArgon2id("VenuStaff!2026")
	totpSecret := "JBSWY3DPEHPK3PXP"

	adminUser := db.User{
		ID:           uuid.New(),
		Name:         "Meera",
		Email:        "meera@venu.live",
		Role:         db.UserRoleADMIN,
		PasswordHash: &passHash,
		TotpSecret:   &totpSecret,
		IsActive:     true,
	}
	scannerUser := db.User{
		ID:           uuid.New(),
		Name:         "Rohan",
		Email:        "rohan@venu.live",
		Role:         db.UserRoleSCANNER,
		PasswordHash: &passHash,
		IsActive:     true,
	}
	patronUser := db.User{
		ID:           uuid.New(),
		Name:         "Arjun",
		Email:        "arjun.mehta@gmail.com",
		Role:         db.UserRoleUSER,
		PasswordHash: &passHash,
		IsActive:     true,
	}

	for _, u := range []db.User{adminUser, scannerUser, patronUser} {
		store.usersByEmail[u.Email] = u
		store.usersByID[u.ID] = u
	}

	verifier := &stubTokenVerifier{
		identity: &auth.GoogleIdentity{
			Sub:           "google-sub-999",
			Email:         "new.patron@gmail.com",
			EmailVerified: true,
			Name:          "Kavya Rao",
		},
	}

	svc := NewTicketingService(cfg, store, tokens, verifier, nil, nil)
	ctx := context.Background()

	t.Run("google login always forces USER role", func(t *testing.T) {
		res, err := svc.LoginWithGoogle(ctx, "valid-google-id-token")
		if err != nil {
			t.Fatalf("google login failed: %v", err)
		}
		if res.User.Role != db.UserRoleUSER {
			t.Fatalf("expected USER role, got %s", res.User.Role)
		}
	})

	t.Run("patron USER rejected from operator login with 403", func(t *testing.T) {
		_, err := svc.LoginAdminOrStaff(ctx, patronUser.Email, "VenuStaff!2026")
		var appErr *httpx.AppError
		if !errors.As(err, &appErr) || appErr.Status != 403 {
			t.Fatalf("expected 403 AppError, got %v", err)
		}
	})

	t.Run("scanner staff skips MFA and receives backend-assigned SCANNER role", func(t *testing.T) {
		out, err := svc.LoginAdminOrStaff(ctx, scannerUser.Email, "VenuStaff!2026")
		if err != nil {
			t.Fatalf("scanner login failed: %v", err)
		}
		if out.RequiresMFA || out.Session == nil {
			t.Fatalf("expected immediate session for SCANNER, got %+v", out)
		}
		if out.Session.User.Role != db.UserRoleSCANNER {
			t.Fatalf("expected SCANNER role, got %s", out.Session.User.Role)
		}
	})

	t.Run("admin requires step-2 TOTP MFA before JWT issuance", func(t *testing.T) {
		out, err := svc.LoginAdminOrStaff(ctx, adminUser.Email, "VenuStaff!2026")
		if err != nil {
			t.Fatalf("admin login step 1 failed: %v", err)
		}
		if !out.RequiresMFA || out.MFAChallengeToken == "" || out.Session != nil {
			t.Fatalf("expected RequiresMFA challenge for ADMIN, got %+v", out)
		}

		if _, err := svc.VerifyAdminMFA(ctx, out.MFAChallengeToken, "000000"); err == nil {
			t.Fatalf("expected invalid TOTP to fail")
		}

		validCode, err := auth.GenerateTOTP(totpSecret, time.Now().UTC())
		if err != nil {
			t.Fatalf("generate totp: %v", err)
		}
		session, err := svc.VerifyAdminMFA(ctx, out.MFAChallengeToken, validCode)
		if err != nil {
			t.Fatalf("admin step-2 MFA verification failed: %v", err)
		}
		if session.User.Role != db.UserRoleADMIN || session.AccessToken == "" {
			t.Fatalf("expected ADMIN session, got %+v", session)
		}

		// Replay protection: reusing the same TOTP code in the same 30s window must fail
		if _, err := svc.VerifyAdminMFA(ctx, out.MFAChallengeToken, validCode); err == nil {
			t.Fatalf("expected TOTP replay attempt to be rejected")
		}
	})

	t.Run("admin TOTP enrollment encrypts at rest and verifies last code (t-1)", func(t *testing.T) {
		enroll, err := svc.EnrollAdminMFA(ctx, adminUser.ID)
		if err != nil {
			t.Fatalf("enroll mfa: %v", err)
		}
		if !strings.HasPrefix(enroll.OtpAuthURI, "otpauth://totp/") {
			t.Fatalf("expected otpauth:// URI, got %s", enroll.OtpAuthURI)
		}
		stored := store.usersByID[adminUser.ID]
		if stored.TotpSecret == nil || *stored.TotpSecret == enroll.Secret {
			t.Fatalf("expected encrypted totp_secret at rest")
		}

		// Last code (t - 1) must be accepted once via Skew: 1, then rejected on replay
		lastCode, err := auth.GenerateTOTP(enroll.Secret, time.Now().UTC().Add(-30*time.Second))
		if err != nil {
			t.Fatalf("generate last code: %v", err)
		}
		out, err := svc.LoginAdminOrStaff(ctx, adminUser.Email, "VenuStaff!2026")
		if err != nil {
			t.Fatalf("login step 1: %v", err)
		}
		if _, err := svc.VerifyAdminMFA(ctx, out.MFAChallengeToken, lastCode); err != nil {
			t.Fatalf("expected previous window (t-1) code to be accepted: %v", err)
		}
		if _, err := svc.VerifyAdminMFA(ctx, out.MFAChallengeToken, lastCode); err == nil {
			t.Fatalf("expected replay of t-1 code to be rejected")
		}
	})
}

func TestRazorpayWebhookAndAsyncMailer(t *testing.T) {
	cfg := testServiceConfig()
	store := newMockStore()
	tokens := auth.NewTokenManager("access-secret-123", "qr-secret-456", cfg.AccessTokenTTL, cfg.MFATokenTTL)
	mailerSpy := &spyMailer{}
	svc := NewTicketingService(cfg, store, tokens, nil, mailerSpy, nil)

	payload, _ := json.Marshal(map[string]any{
		"razorpay_order_id": "order_RZP_101",
		"phone":             "+919820144120",
		"event_title":       "Anoushka Shankar",
	})
	mac := hmac.New(sha256.New, []byte(cfg.RazorpayWebhookSecret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	if _, err := svc.HandleRazorpayWebhook(context.Background(), payload, "bad-sig", "order_RZP_101", "+919820144120", "Anoushka Shankar", "Gate 2", 600); err == nil {
		t.Fatalf("expected webhook signature verification to fail")
	}

	tkt, err := svc.HandleRazorpayWebhook(context.Background(), payload, sig, "order_RZP_101", "+919820144120", "Anoushka Shankar", "Gate 2", 600)
	if err != nil {
		t.Fatalf("expected webhook confirmation to succeed: %v", err)
	}
	if tkt.FoodBalance != 600 || len(mailerSpy.sent) != 1 || mailerSpy.sent[0].TicketCode != tkt.Code {
		t.Fatalf("unexpected ticket/mailer state: tkt=%+v sent=%+v", tkt, mailerSpy.sent)
	}
}

func TestRedeemCounterBalance_SplitAndOwnership(t *testing.T) {
	cfg := testServiceConfig()
	store := newMockStore()
	drinkID := uuid.New()
	ticketID := uuid.New()
	store.menuItems = []db.FnbMenuItem{{ID: drinkID, Name: "Yuzu Tonic", Price: 650, IsAvailable: true}}
	store.tickets[ticketID] = db.Ticket{
		ID:                 ticketID,
		OrderID:            uuid.New(),
		EventID:            uuid.New(),
		UserID:             uuid.New(),
		Code:               "VNU-TEST1",
		FoodBalance:        500,
		InitialFoodBalance: 500,
	}

	svc := NewTicketingService(cfg, store, nil, nil, nil, nil)
	bartenderID := uuid.New()

	_, err := svc.RedeemCounterBalance(context.Background(), ticketID, bartenderID, []FnbCartEntry{{ItemID: drinkID, Quantity: 1}}, false)
	if !errors.Is(err, ErrUpiDeltaUnconfirmed) {
		t.Fatalf("expected ErrUpiDeltaUnconfirmed when order > wallet, got %v", err)
	}

	res, err := svc.RedeemCounterBalance(context.Background(), ticketID, bartenderID, []FnbCartEntry{{ItemID: drinkID, Quantity: 1}}, true)
	if err != nil {
		t.Fatalf("expected split redemption to succeed: %v", err)
	}
	if res.Ticket.FoodBalance != 0 || res.Transaction.WalletDeducted != 500 || res.Transaction.UpiDeltaPaid != 150 {
		t.Fatalf("unexpected settlement result: %+v", res)
	}
}

func TestServicePropagatesDeadlineExceeded(t *testing.T) {
	store := newMockStore()
	store.errOnListEvents = fmt.Errorf("repo_list_active_events: %w", context.DeadlineExceeded)
	svc := NewTicketingService(testServiceConfig(), store, nil, nil, nil, nil)

	_, err := svc.ListActiveEvents(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded propagation, got %v", err)
	}
}

func TestTicketQRHMAC_TamperExpirationAndAtomicSingleScan(t *testing.T) {
	cfg := testServiceConfig()
	cfg.RotatingQREnabled = true
	store := newMockStore()
	tm := auth.NewTokenManager("access-secret-123", "qr-secret-456", cfg.AccessTokenTTL, cfg.MFATokenTTL)
	svc := NewTicketingService(cfg, store, tm, nil, nil, nil)

	ownerID := uuid.New()
	tID := uuid.New()
	oID := uuid.New()
	eID := uuid.New()
	rotSecret, _ := auth.GenerateRandomBase32Secret()
	encRot, _ := auth.EncryptTOTPSecret(rotSecret, cfg.MFAEncryptionKey)
	exp := time.Now().UTC().Add(2 * time.Hour)

	store.tickets[tID] = db.Ticket{
		ID:            tID,
		OrderID:       oID,
		EventID:       eID,
		UserID:        ownerID,
		Code:          "VNU-42091",
		QrTokenDigest: tm.ComputeTicketDigest(tID),
		QrOtpSecret:   &encRot,
		ExpiresAt:     &exp,
	}

	// Re-generate on demand with rotating TOTP
	qrRes, err := svc.GenerateTicketQR(context.Background(), tID, ownerID, true)
	if err != nil {
		t.Fatalf("generate ticket qr: %v", err)
	}
	if !qrRes.Rotating || !strings.Contains(qrRes.QrToken, ".") {
		t.Fatalf("expected signed rotating payload.signature token, got %+v", qrRes)
	}

	// Valid scan passes
	scanRes, err := svc.ValidateScanTicket(context.Background(), qrRes.QrToken)
	if err != nil || !scanRes.Valid || scanRes.Reason != "VALID" {
		t.Fatalf("expected valid scan, got %+v err=%v", scanRes, err)
	}

	// Tampered signature rejected
	if tampered, _ := svc.ValidateScanTicket(context.Background(), qrRes.QrToken+"tamper"); tampered.Valid || tampered.Reason != "INVALID_SIGNATURE" {
		t.Fatalf("expected INVALID_SIGNATURE for tampered QR, got %+v", tampered)
	}

	// Expired QR token rejected
	expiredTok, _ := tm.CreateTicketQRToken(tID, oID, eID, "VNU-42091", time.Now().UTC().Add(-time.Minute), nil)
	if expRes, _ := svc.ValidateScanTicket(context.Background(), expiredTok); expRes.Valid || expRes.Reason != "EXPIRED" {
		t.Fatalf("expected EXPIRED for past-exp QR, got %+v", expRes)
	}

	// Atomic single-scan enforcement: first check-in succeeds, second fails with 409
	scannerID := uuid.New()
	if _, _, err := svc.CheckInTicket(context.Background(), tID, nil, scannerID); err != nil {
		t.Fatalf("first check-in failed: %v", err)
	}
	if _, _, err := svc.CheckInTicket(context.Background(), tID, nil, scannerID); err == nil {
		t.Fatalf("expected second check-in to fail with already checked in conflict")
	}
}
