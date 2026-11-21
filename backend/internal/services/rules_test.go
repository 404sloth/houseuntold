package services

import (
	"testing"

	"github.com/venu-live/venu-api/internal/db"
)

func TestCalculateCheckoutPricing_First99Rule(t *testing.T) {
	femaleTier := db.TicketCategory{
		Name:       "STAG_FEMALE",
		BasePrice:  999,
		FnbCredit:  999,
		EntryCount: 1,
	}

	p0 := CalculateCheckoutPricing(femaleTier, 0, 99)
	if p0.TotalAmount != 0 || p0.FnbCredit != 999 || !p0.First99Applied || !p0.FreeShotPerk {
		t.Fatalf("expected free first-99 pricing at slot 0, got %+v", p0)
	}

	p98 := CalculateCheckoutPricing(femaleTier, 98, 99)
	if p98.TotalAmount != 0 || !p98.First99Applied {
		t.Fatalf("expected free pricing at 99th slot (count 98), got %+v", p98)
	}

	p99 := CalculateCheckoutPricing(femaleTier, 99, 99)
	if p99.TotalAmount != 999 || p99.FnbCredit != 999 || p99.First99Applied || !p99.FreeShotPerk {
		t.Fatalf("expected paid INR 999 pricing at 100th slot (count 99), got %+v", p99)
	}
}

func TestValidateCategoryGuests_CoupleStrictRule(t *testing.T) {
	coupleTier := db.TicketCategory{
		Name:           "COUPLE",
		BasePrice:      3299,
		FnbCredit:      3000,
		EntryCount:     2,
		IsGenderStrict: true,
	}

	valid := []GuestInput{
		{Name: "Aarav", Age: 25, Gender: db.GenderTypeMALE},
		{Name: "Meera", Age: 24, Gender: db.GenderTypeFEMALE},
	}
	if err := ValidateCategoryGuests(coupleTier, valid, 21); err != nil {
		t.Fatalf("expected valid 1M+1F couple to pass, got %v", err)
	}

	twoMales := []GuestInput{
		{Name: "Aarav", Age: 25, Gender: db.GenderTypeMALE},
		{Name: "Rohan", Age: 26, Gender: db.GenderTypeMALE},
	}
	if err := ValidateCategoryGuests(coupleTier, twoMales, 21); err == nil {
		t.Fatalf("expected 2M couple to be rejected")
	}

	underage := []GuestInput{
		{Name: "Aarav", Age: 19, Gender: db.GenderTypeMALE},
		{Name: "Meera", Age: 22, Gender: db.GenderTypeFEMALE},
	}
	if err := ValidateCategoryGuests(coupleTier, underage, 21); err == nil {
		t.Fatalf("expected underage guest to be rejected")
	}
}

func TestCalculateBarSettlement(t *testing.T) {
	s1 := CalculateBarSettlement(800, 1999)
	if s1.WalletDeducted != 800 || s1.DeltaToCollect != 0 {
		t.Fatalf("expected full wallet cover, got %+v", s1)
	}

	s2 := CalculateBarSettlement(1500, 999)
	if s2.WalletDeducted != 999 || s2.DeltaToCollect != 501 {
		t.Fatalf("expected 999 wallet + 501 counter delta, got %+v", s2)
	}

	s3 := CalculateBarSettlement(600, 0)
	if s3.WalletDeducted != 0 || s3.DeltaToCollect != 600 {
		t.Fatalf("expected full counter delta when wallet is empty, got %+v", s3)
	}
}
