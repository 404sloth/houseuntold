package services

import (
	"fmt"
	"strings"

	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
)

// GuestInput represents an attendee provided during guest checkout.
type GuestInput struct {
	Name             string        `json:"name"`
	Age              int32         `json:"age"`
	Gender           db.GenderType `json:"gender"`
	IsPrimaryContact bool          `json:"is_primary_contact"`
}

// CheckoutPricing holds the authoritative server-calculated checkout totals.
type CheckoutPricing struct {
	TotalAmount    int32
	FnbCredit      int32
	First99Applied bool
	FreeShotPerk   bool
}

// BarSettlement holds the zero-trust wallet deduction vs counter delta note calculation.
type BarSettlement struct {
	TotalBill      int32
	WalletDeducted int32
	DeltaToCollect int32
}

// ValidateCategoryGuests enforces entry counts, minimum age, and strict gender rules (Couple 1M+1F, Stag Male/Female).
func ValidateCategoryGuests(cat db.TicketCategory, guests []GuestInput, minAge int32) error {
	if len(guests) == 0 {
		return httpx.NewBadRequest("at least one guest is required", nil)
	}
	if int32(len(guests)) != cat.EntryCount {
		return httpx.NewBadRequest(
			fmt.Sprintf("this pass requires exactly %d guest(s), got %d", cat.EntryCount, len(guests)),
			nil,
		)
	}

	var maleCount, femaleCount int
	for i, g := range guests {
		trimmedName := strings.TrimSpace(g.Name)
		if trimmedName == "" || len(trimmedName) > 255 {
			return httpx.NewBadRequest(fmt.Sprintf("guest %d name is required (max 255 chars)", i+1), nil)
		}
		if g.Age < minAge || g.Age > 120 {
			return httpx.NewBadRequest(
				fmt.Sprintf("guest %d (%s) must be at least %d years old", i+1, strings.TrimSpace(g.Name), minAge),
				nil,
			)
		}
		switch g.Gender {
		case db.GenderTypeMALE:
			maleCount++
		case db.GenderTypeFEMALE:
			femaleCount++
		case db.GenderTypeOTHER:
		default:
			return httpx.NewBadRequest(fmt.Sprintf("guest %d has invalid gender", i+1), nil)
		}
	}

	upperName := strings.ToUpper(strings.TrimSpace(cat.Name))
	if cat.IsGenderStrict || upperName == "COUPLE" {
		if cat.EntryCount != 2 || maleCount != 1 || femaleCount != 1 {
			return httpx.NewBadRequest("couple entry strictly requires 1 MALE and 1 FEMALE guest", nil)
		}
	}

	switch upperName {
	case "STAG_FEMALE":
		if femaleCount != int(cat.EntryCount) {
			return httpx.NewBadRequest("stag female pass requires FEMALE guest gender", nil)
		}
	case "STAG_MALE":
		if maleCount != int(cat.EntryCount) {
			return httpx.NewBadRequest("stag male pass requires MALE guest gender", nil)
		}
	}

	return nil
}

// CalculateCheckoutPricing applies GA tier pricing and the First 99 Female rule server-side.
func CalculateCheckoutPricing(cat db.TicketCategory, confirmedFemaleCount, first99Limit int64) CheckoutPricing {
	upperName := strings.ToUpper(strings.TrimSpace(cat.Name))
	if upperName == "STAG_FEMALE" {
		if confirmedFemaleCount < first99Limit {
			return CheckoutPricing{
				TotalAmount:    0,
				FnbCredit:      cat.FnbCredit,
				First99Applied: true,
				FreeShotPerk:   true,
			}
		}
		return CheckoutPricing{
			TotalAmount:    cat.BasePrice,
			FnbCredit:      cat.FnbCredit,
			First99Applied: false,
			FreeShotPerk:   true,
		}
	}

	return CheckoutPricing{
		TotalAmount:    cat.BasePrice,
		FnbCredit:      cat.FnbCredit,
		First99Applied: false,
		FreeShotPerk:   false,
	}
}

// CalculateBarSettlement splits a server-calculated bar bill across F&B wallet balance and counter delta reference.
func CalculateBarSettlement(totalBill, walletBalance int32) BarSettlement {
	if totalBill <= 0 {
		return BarSettlement{}
	}
	if walletBalance <= 0 {
		return BarSettlement{
			TotalBill:      totalBill,
			WalletDeducted: 0,
			DeltaToCollect: totalBill,
		}
	}
	if walletBalance >= totalBill {
		return BarSettlement{
			TotalBill:      totalBill,
			WalletDeducted: totalBill,
			DeltaToCollect: 0,
		}
	}
	return BarSettlement{
		TotalBill:      totalBill,
		WalletDeducted: walletBalance,
		DeltaToCollect: totalBill - walletBalance,
	}
}

// PrimaryGuestName selects the guest marked IsPrimaryContact, falling back to the first guest or "Guest".
func PrimaryGuestName(guests []db.TicketGuest) string {
	for _, g := range guests {
		if g.IsPrimaryContact && strings.TrimSpace(g.Name) != "" {
			return strings.TrimSpace(g.Name)
		}
	}
	if len(guests) > 0 && strings.TrimSpace(guests[0].Name) != "" {
		return strings.TrimSpace(guests[0].Name)
	}
	return "Guest"
}
