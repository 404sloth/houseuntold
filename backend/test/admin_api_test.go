package test

import (
	"net/http"
	"testing"
	"time"

	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/services"
)

// Covers Routes 18–25:
//   18. POST   /api/admin/staff
//   19. PATCH  /api/admin/staff/{id}/password
//   20. DELETE /api/admin/staff/{id}
//   21. GET    /api/admin/bookings
//   22. POST   /api/admin/bookings/{id}/verify
//   23. POST   /api/admin/events
//   24. POST   /api/admin/events/{id}/tiers
//   25. GET    /api/admin/stats/{event_id}
func TestAdminStaffLifecycleAndInstantCacheRevocation(t *testing.T) {
	env := newTestEnv(t)
	adminToken, _ := env.loginAs("admin_user", "AdminPass!123")

	// 18. POST /api/admin/staff -> 201 creates a new SCANNER
	rec := env.doRequest(http.MethodPost, "/api/admin/staff", services.CreateStaffRequest{
		Username: "gate_shift_2",
		Password: "ShiftPassword!1",
		Role:     db.StaffRoleSCANNER,
	}, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusCreated)
	var created services.StaffAccountView
	decodeBody(t, rec, &created)
	if created.Role != db.StaffRoleSCANNER || !created.IsActive {
		t.Fatalf("unexpected created staff: %+v", created)
	}

	// Log in as new staff (populates StaffRoleCache) and verify token works
	staffToken, _ := env.loginAs("gate_shift_2", "ShiftPassword!1")
	if _, hit := env.roleCache.Get(created.ID); !hit {
		t.Fatal("expected new staff to be cached on login")
	}
	assertStatus(t, env.doRequest(http.MethodPost, "/api/gate/validate", map[string]string{
		"qr_token": "not-a-real-qr",
	}, bearerHeader(staffToken)), http.StatusUnauthorized) // reached GateService (not 403/401 auth)

	// 19. PATCH /api/admin/staff/{id}/password -> 200 rotates password + evicts cache
	rec = env.doRequest(http.MethodPatch, "/api/admin/staff/"+created.ID.String()+"/password", map[string]string{
		"password": "RotatedPassword!2",
	}, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusOK)
	if _, hit := env.roleCache.Get(created.ID); hit {
		t.Fatal("expected StaffRoleCache eviction on password update")
	}
	// Old password fails; new password works
	assertStatus(t, env.doRequest(http.MethodPost, "/api/auth/login", map[string]string{
		"username": "gate_shift_2",
		"password": "ShiftPassword!1",
	}, nil), http.StatusUnauthorized)
	staffToken, _ = env.loginAs("gate_shift_2", "RotatedPassword!2")

	// 20a. Admin cannot self-deactivate -> 400
	assertStatus(t, env.doRequest(http.MethodDelete, "/api/admin/staff/"+env.adminID.String(), nil, bearerHeader(adminToken)), http.StatusBadRequest)

	// 20b. DELETE /api/admin/staff/{id} -> 200 deactivates + evicts cache -> existing JWT immediately rejected with 401
	rec = env.doRequest(http.MethodDelete, "/api/admin/staff/"+created.ID.String(), nil, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusOK)

	assertStatus(t, env.doRequest(http.MethodPost, "/api/gate/validate", map[string]string{
		"qr_token": "anything",
	}, bearerHeader(staffToken)), http.StatusUnauthorized)
}

func TestAdminUPIVerificationEventsTiersAndStats(t *testing.T) {
	env := newTestEnv(t)
	adminToken, _ := env.loginAs("admin_user", "AdminPass!123")
	barToken, _ := env.loginAs("bar_user", "BarPass!123")

	// 23. POST /api/admin/events -> invalid time window 400, valid 201
	now := time.Now().UTC()
	assertStatus(t, env.doRequest(http.MethodPost, "/api/admin/events", services.CreateEventRequest{
		Title:    "Bad Window",
		StartsAt: now.Add(2 * time.Hour),
		EndsAt:   now.Add(1 * time.Hour),
		Location: "Club",
		Status:   db.EventStatusONSALE,
	}, bearerHeader(adminToken)), http.StatusBadRequest)

	rec := env.doRequest(http.MethodPost, "/api/admin/events", services.CreateEventRequest{
		Title:    "Sunday Rooftop",
		StartsAt: now.Add(-30 * time.Minute),
		EndsAt:   now.Add(5 * time.Hour),
		Location: "Skyline Deck, Bengaluru",
		Status:   db.EventStatusONSALE,
	}, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusCreated)
	var newEv db.Event
	decodeBody(t, rec, &newEv)

	// 24. POST /api/admin/events/{id}/tiers -> bad entry_count 400, valid COUPLE 201
	assertStatus(t, env.doRequest(http.MethodPost, "/api/admin/events/"+newEv.ID.String()+"/tiers", services.CreateTicketTierRequest{
		Name:       "GROUP",
		BasePrice:  10000,
		FnbCredit:  8000,
		EntryCount: 5,
	}, bearerHeader(adminToken)), http.StatusBadRequest)

	rec = env.doRequest(http.MethodPost, "/api/admin/events/"+newEv.ID.String()+"/tiers", services.CreateTicketTierRequest{
		Name:       "COUPLE",
		Category:   "VIP",
		BasePrice:  4000,
		FnbCredit:  2500,
		EntryCount: 2,
	}, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusCreated)
	var tier db.TicketCategory
	decodeBody(t, rec, &tier)
	if !tier.IsGenderStrict {
		t.Fatal("COUPLE tier must automatically enforce is_gender_strict=true")
	}

	// Guest initiates paid booking WITHOUT UTR first
	rec = env.doRequest(http.MethodPost, "/api/checkout/init", services.InitCheckoutRequest{
		EventID:      newEv.ID,
		CategoryID:   tier.ID,
		ContactPhone: "+919876540000",
		ContactEmail: "couple@example.com",
		Guests: []services.GuestInput{
			{Name: "Rohan Das", Age: 27, Gender: db.GenderTypeMALE},
			{Name: "Diya Kapoor", Age: 25, Gender: db.GenderTypeFEMALE, IsPrimaryContact: true},
		},
	}, nil)
	assertStatus(t, rec, http.StatusCreated)
	var booking services.InitCheckoutResponse
	decodeBody(t, rec, &booking)

	// 22a. Admin cannot APPROVE paid booking before patron submits UPI UTR -> 400
	assertStatus(t, env.doRequest(http.MethodPost, "/api/admin/bookings/"+booking.BookingID.String()+"/verify", services.VerifyBookingRequest{
		Action: "APPROVE",
	}, bearerHeader(adminToken)), http.StatusBadRequest)

	// Patron submits UTR
	assertStatus(t, env.doRequest(http.MethodPost, "/api/checkout/"+booking.BookingID.String()+"/upi-reference", map[string]string{
		"upi_transaction_id": "UTR9988776655",
	}, nil), http.StatusOK)

	// 21. GET /api/admin/bookings -> Admin sees both UTR and exact expected_amount
	rec = env.doRequest(http.MethodGet, "/api/admin/bookings?status=PENDING&event_id="+newEv.ID.String(), nil, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusOK)
	var queue struct {
		Bookings []services.BookingVerificationItem `json:"bookings"`
		Count    int                                `json:"count"`
	}
	decodeBody(t, rec, &queue)
	if queue.Count != 1 || queue.Bookings[0].UPITransactionID != "UTR9988776655" || queue.Bookings[0].ExpectedAmount != 4000 {
		t.Fatalf("unexpected verification queue: %+v", queue)
	}

	// 22b. Admin APPROVES booking -> CONFIRMED + WhatsApp PNG pass queued with primary guest "Diya Kapoor"
	rec = env.doRequest(http.MethodPost, "/api/admin/bookings/"+booking.BookingID.String()+"/verify", services.VerifyBookingRequest{
		Action: "APPROVE",
	}, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusOK)
	var verified services.VerifyBookingResponse
	decodeBody(t, rec, &verified)
	if verified.Status != db.BookingStatusCONFIRMED || !verified.WhatsAppQueued {
		t.Fatalf("expected CONFIRMED + whatsapp_queued=true, got %+v", verified)
	}

	msgs := env.notifier.Messages()
	if len(msgs) != 1 || msgs[0].PrimaryGuest != "Diya Kapoor" {
		t.Fatalf("expected WhatsApp confirmation addressed to primary guest Diya Kapoor, got %+v", msgs)
	}

	// 22c. Double-verifying an already CONFIRMED booking -> 409
	assertStatus(t, env.doRequest(http.MethodPost, "/api/admin/bookings/"+booking.BookingID.String()+"/verify", services.VerifyBookingRequest{
		Action: "REJECT",
	}, bearerHeader(adminToken)), http.StatusConflict)

	// 22d. Explicit REJECT on a second pending booking -> FAILED + no extra WhatsApp queued
	recRejInit := env.doRequest(http.MethodPost, "/api/checkout/init", services.InitCheckoutRequest{
		EventID:          newEv.ID,
		CategoryID:       tier.ID,
		ContactPhone:     "+919876540009",
		ContactEmail:     "reject@example.com",
		UPITransactionID: "BADUTR123456",
		Guests: []services.GuestInput{
			{Name: "Guest A", Age: 25, Gender: db.GenderTypeMALE, IsPrimaryContact: true},
			{Name: "Guest B", Age: 25, Gender: db.GenderTypeFEMALE},
		},
	}, nil)
	assertStatus(t, recRejInit, http.StatusCreated)
	var rejBooking services.InitCheckoutResponse
	decodeBody(t, recRejInit, &rejBooking)

	recRej := env.doRequest(http.MethodPost, "/api/admin/bookings/"+rejBooking.BookingID.String()+"/verify", services.VerifyBookingRequest{
		Action:          "REJECT",
		RejectionReason: "UTR not found in bank statement",
	}, bearerHeader(adminToken))
	assertStatus(t, recRej, http.StatusOK)
	var rejResp services.VerifyBookingResponse
	decodeBody(t, recRej, &rejResp)
	if rejResp.Status != db.BookingStatusFAILED || rejResp.WhatsAppQueued {
		t.Fatalf("expected FAILED booking with whatsapp_queued=false, got %+v", rejResp)
	}

	// Redeem 1 drink against the newly approved couple ticket so stats show bar activity
	barCode := queue.Bookings[0].BarCode
	assertStatus(t, env.doRequest(http.MethodPost, "/api/counter/redeem", services.RedeemItemsRequest{
		BarCode: barCode,
		Items:   []services.RedeemLineItemInput{{MenuItemID: env.drinkID, Quantity: 2}}, // 1500 INR from 2500 credit
	}, bearerHeader(barToken)), http.StatusOK)

	// 25. GET /api/admin/stats/{event_id} -> aggregates revenue, F&B, and gender breakdown
	rec = env.doRequest(http.MethodGet, "/api/admin/stats/"+newEv.ID.String(), nil, bearerHeader(adminToken))
	assertStatus(t, rec, http.StatusOK)
	var stats services.EventStatsView
	decodeBody(t, rec, &stats)
	if stats.ConfirmedBookings != 1 || stats.TicketRevenue != 4000 || stats.TotalFnbAllocated != 2500 || stats.TotalFnbRedeemed != 1500 || stats.TotalFnbRemaining != 1000 || stats.MaleGuests != 1 || stats.FemaleGuests != 1 {
		t.Fatalf("unexpected event stats: %+v", stats)
	}
}
