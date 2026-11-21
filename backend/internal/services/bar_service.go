package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/venu-live/venu-api/internal/auth"
	"github.com/venu-live/venu-api/internal/db"
	"github.com/venu-live/venu-api/internal/httpx"
	"github.com/venu-live/venu-api/internal/repositories"
)

type BarService struct {
	repo   *repositories.TicketingRepository
	logger *slog.Logger
}

func NewBarService(repo *repositories.TicketingRepository, logger *slog.Logger) *BarService {
	if logger == nil {
		logger = slog.Default()
	}
	return &BarService{repo: repo, logger: logger}
}

func (s *BarService) GetMenu(ctx context.Context) ([]db.MenuItem, error) {
	items, err := s.repo.ListActiveMenuItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("svc_bar_get_menu: %w", err)
	}
	return items, nil
}

type CreateMenuItemRequest struct {
	Name      string          `json:"name"`
	Category  db.MenuCategory `json:"category"`
	UnitPrice int32           `json:"unit_price"`
}

func (s *BarService) CreateMenuItem(ctx context.Context, req CreateMenuItemRequest) (db.MenuItem, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 200 {
		return db.MenuItem{}, httpx.NewBadRequest("menu item name is required (max 200 chars)", nil)
	}
	if req.Category != db.MenuCategoryDRINK && req.Category != db.MenuCategoryFOOD {
		return db.MenuItem{}, httpx.NewBadRequest("category must be DRINK or FOOD", nil)
	}
	if req.UnitPrice <= 0 || req.UnitPrice > 1000000 {
		return db.MenuItem{}, httpx.NewBadRequest("unit_price must be between 1 and 1000000 INR", nil)
	}

	item, err := s.repo.CreateMenuItem(ctx, db.CreateMenuItemParams{
		Name:      name,
		Category:  req.Category,
		UnitPrice: req.UnitPrice,
	})
	if err != nil {
		return db.MenuItem{}, fmt.Errorf("svc_bar_create_menu_item: %w", err)
	}
	return item, nil
}

// DeleteMenuItem soft-deletes a menu item so past bar_transactions remain 100% auditable.
func (s *BarService) DeleteMenuItem(ctx context.Context, itemID uuid.UUID) (db.MenuItem, error) {
	item, err := s.repo.DeactivateMenuItem(ctx, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.MenuItem{}, httpx.NewNotFound("active menu item not found")
		}
		return db.MenuItem{}, fmt.Errorf("svc_bar_delete_menu_item: %w", err)
	}
	return item, nil
}

type BarLineItemHistoryView struct {
	MenuItemID      uuid.UUID `json:"menu_item_id"`
	MenuItemName    string    `json:"menu_item_name"`
	Quantity        int32     `json:"quantity"`
	UnitPriceAtSale int32     `json:"unit_price_at_sale"`
	LineTotal       int32     `json:"line_total"`
}

type BarTransactionHistoryView struct {
	TransactionID     uuid.UUID                `json:"transaction_id"`
	BartenderID       uuid.UUID                `json:"bartender_id"`
	BartenderUsername string                   `json:"bartender_username"`
	TotalBill         int32                    `json:"total_bill"`
	WalletDeducted    int32                    `json:"wallet_deducted"`
	DeltaToCollect    int32                    `json:"delta_to_collect"`
	BalanceBefore     int32                    `json:"balance_before"`
	BalanceAfter      int32                    `json:"balance_after"`
	CreatedAt         time.Time                `json:"created_at"`
	Items             []BarLineItemHistoryView `json:"items"`
}

type BarTicketSearchView struct {
	TicketID           uuid.UUID                   `json:"ticket_id"`
	EventID            uuid.UUID                   `json:"event_id"`
	EventTitle         string                      `json:"event_title"`
	EventEndsAt        time.Time                   `json:"event_ends_at"`
	BarCode            string                      `json:"bar_code"`
	CategoryName       string                      `json:"category_name"`
	CategoryTier       string                      `json:"category_tier,omitempty"`
	EntryCount         int32                       `json:"entry_count"`
	FoodBalance        int32                       `json:"food_balance"`
	InitialFoodBalance int32                       `json:"initial_food_balance"`
	WalletExpired      bool                        `json:"wallet_expired"`
	IsCheckedIn        bool                        `json:"is_checked_in"`
	Guests             []GuestView                 `json:"guests"`
	RecentTransactions []BarTransactionHistoryView `json:"recent_transactions"`
}

func (s *BarService) SearchTicketByBarCode(ctx context.Context, barCode string) (BarTicketSearchView, error) {
	code := strings.ToUpper(strings.TrimSpace(barCode))
	if !auth.IsValidBarCode(code) {
		return BarTicketSearchView{}, httpx.NewBadRequest("bar_code must be a 5-character 1D barcode (1 letter + 4 digits, e.g. K4829)", nil)
	}

	row, err := s.repo.GetTicketDetailsByBarCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BarTicketSearchView{}, httpx.NewNotFound("ticket not found for bar_code")
		}
		return BarTicketSearchView{}, fmt.Errorf("svc_bar_search: %w", err)
	}
	if row.BookingStatus != db.BookingStatusCONFIRMED {
		return BarTicketSearchView{}, httpx.NewConflict("ticket booking is not confirmed", nil)
	}

	guests, err := s.repo.ListGuestsByTicketID(ctx, row.ID)
	if err != nil {
		return BarTicketSearchView{}, fmt.Errorf("svc_bar_search_guests: %w", err)
	}

	history, err := s.loadTicketHistory(ctx, row.ID, 15)
	if err != nil {
		return BarTicketSearchView{}, err
	}

	guestViews := make([]GuestView, 0, len(guests))
	for _, g := range guests {
		guestViews = append(guestViews, GuestView{
			ID:               g.ID,
			Name:             g.Name,
			Age:              g.Age,
			Gender:           g.Gender,
			IsPrimaryContact: g.IsPrimaryContact,
			FreeShotRedeemed: g.FreeShotRedeemed,
		})
	}

	view := BarTicketSearchView{
		TicketID:           row.ID,
		EventID:            row.EventID,
		EventTitle:         row.EventTitle,
		EventEndsAt:        row.EventEndsAt,
		BarCode:            row.BarCode,
		CategoryName:       row.CategoryName,
		EntryCount:         row.EntryCount,
		FoodBalance:        row.FoodBalance,
		InitialFoodBalance: row.InitialFoodBalance,
		WalletExpired:      time.Now().UTC().After(row.EventEndsAt),
		IsCheckedIn:        row.IsCheckedIn,
		Guests:             guestViews,
		RecentTransactions: history,
	}
	if row.CategoryTier.Valid {
		view.CategoryTier = row.CategoryTier.String
	}
	return view, nil
}

func (s *BarService) GetTicketBarHistory(ctx context.Context, barCode string, limit int32) ([]BarTransactionHistoryView, error) {
	code := strings.ToUpper(strings.TrimSpace(barCode))
	if !auth.IsValidBarCode(code) {
		return nil, httpx.NewBadRequest("bar_code must be a 5-character 1D barcode (1 letter + 4 digits, e.g. K4829)", nil)
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	row, err := s.repo.GetTicketDetailsByBarCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.NewNotFound("ticket not found for bar_code")
		}
		return nil, fmt.Errorf("svc_bar_history_lookup: %w", err)
	}
	return s.loadTicketHistory(ctx, row.ID, limit)
}

func (s *BarService) loadTicketHistory(ctx context.Context, ticketID uuid.UUID, limit int32) ([]BarTransactionHistoryView, error) {
	txRows, err := s.repo.ListBarTransactionsByTicketID(ctx, ticketID, limit)
	if err != nil {
		return nil, fmt.Errorf("svc_bar_list_txs: %w", err)
	}
	if len(txRows) == 0 {
		return []BarTransactionHistoryView{}, nil
	}

	txIDs := make([]uuid.UUID, len(txRows))
	for i, r := range txRows {
		txIDs[i] = r.ID
	}

	lineRows, err := s.repo.ListLineItemsByTransactionIDs(ctx, txIDs)
	if err != nil {
		return nil, fmt.Errorf("svc_bar_list_lines: %w", err)
	}

	linesByTx := make(map[uuid.UUID][]BarLineItemHistoryView, len(txRows))
	for _, l := range lineRows {
		linesByTx[l.TransactionID] = append(linesByTx[l.TransactionID], BarLineItemHistoryView{
			MenuItemID:      l.MenuItemID,
			MenuItemName:    l.MenuItemName,
			Quantity:        l.Quantity,
			UnitPriceAtSale: l.UnitPriceAtSale,
			LineTotal:       l.UnitPriceAtSale * l.Quantity,
		})
	}

	out := make([]BarTransactionHistoryView, 0, len(txRows))
	for _, t := range txRows {
		items := linesByTx[t.ID]
		if items == nil {
			items = []BarLineItemHistoryView{}
		}
		out = append(out, BarTransactionHistoryView{
			TransactionID:     t.ID,
			BartenderID:       t.BartenderID,
			BartenderUsername: t.BartenderUsername,
			TotalBill:         t.TotalBill,
			WalletDeducted:    t.WalletDeducted,
			DeltaToCollect:    t.UpiDeltaPaid,
			BalanceBefore:     t.BalanceBefore,
			BalanceAfter:      t.BalanceAfter,
			CreatedAt:         t.CreatedAt,
			Items:             items,
		})
	}
	return out, nil
}

type RedeemLineItemInput struct {
	MenuItemID uuid.UUID `json:"menu_item_id"`
	Quantity   int32     `json:"quantity"`
}

type RedeemItemsRequest struct {
	BarCode               string                `json:"bar_code"`
	Items                 []RedeemLineItemInput `json:"items"`
	IdempotencyKey        string                `json:"idempotency_key,omitempty"`
	RedeemFreeShotGuestID *uuid.UUID            `json:"redeem_free_shot_guest_id,omitempty"`
}

type RedeemItemsResponse struct {
	TransactionID    uuid.UUID `json:"transaction_id"`
	TicketID         uuid.UUID `json:"ticket_id"`
	BarCode          string    `json:"bar_code"`
	TotalBill        int32     `json:"total_bill"`
	WalletDeducted   int32     `json:"wallet_deducted"`
	DeltaToCollect   int32     `json:"delta_to_collect"` // Noted for bartender counter collection reference
	BalanceBefore    int32     `json:"balance_before"`
	RemainingBalance int32     `json:"remaining_balance"`
	FreeShotRedeemed bool      `json:"free_shot_redeemed"`
	IdempotentReplay bool      `json:"idempotent_replay,omitempty"`
}

// RedeemItems is the highest-traffic hot-path endpoint:
// 1. Enforces Idempotency-Key so retries on club Wi-Fi never double-deduct.
// 2. Locks ticket row via B-tree index (bar_code = $1 FOR UPDATE OF t) to prevent double-spend across concurrent bartenders.
// 3. Computes authoritative prices strictly from PostgreSQL menu_items.
// 4. Deducts up to available ticket balance and records any excess delta_to_collect for bartender counter reference.
// 5. Persists balance_before, balance_after, and every line item for 100% audit traceability.
func (s *BarService) RedeemItems(ctx context.Context, bartenderID uuid.UUID, headerIdemKey string, req RedeemItemsRequest) (RedeemItemsResponse, error) {
	code := strings.ToUpper(strings.TrimSpace(req.BarCode))
	if !auth.IsValidBarCode(code) {
		return RedeemItemsResponse{}, httpx.NewBadRequest("bar_code must be a 5-character 1D barcode (1 letter + 4 digits, e.g. K4829)", nil)
	}
	if len(req.Items) == 0 && req.RedeemFreeShotGuestID == nil {
		return RedeemItemsResponse{}, httpx.NewBadRequest("at least one menu item or free shot redemption is required", nil)
	}
	if len(req.Items) > 50 {
		return RedeemItemsResponse{}, httpx.NewBadRequest("cannot redeem more than 50 distinct line items per transaction", nil)
	}

	idemKey := strings.TrimSpace(headerIdemKey)
	if idemKey == "" {
		idemKey = strings.TrimSpace(req.IdempotencyKey)
	}
	if len(idemKey) > 100 {
		return RedeemItemsResponse{}, httpx.NewBadRequest("idempotency_key must be at most 100 characters", nil)
	}

	qtyByItem := make(map[uuid.UUID]int32, len(req.Items))
	itemIDs := make([]uuid.UUID, 0, len(req.Items))
	for _, line := range req.Items {
		if line.MenuItemID == uuid.Nil {
			return RedeemItemsResponse{}, httpx.NewBadRequest("invalid menu_item_id", nil)
		}
		if line.Quantity <= 0 || line.Quantity > 50 {
			return RedeemItemsResponse{}, httpx.NewBadRequest("quantity must be between 1 and 50", nil)
		}
		if _, exists := qtyByItem[line.MenuItemID]; !exists {
			itemIDs = append(itemIDs, line.MenuItemID)
		}
		qtyByItem[line.MenuItemID] += line.Quantity
	}

	// Fetch authoritative unit prices strictly from PostgreSQL before acquiring row lock (keeps lock duration minimal).
	var menuByID map[uuid.UUID]db.MenuItem
	var totalBill int32
	if len(itemIDs) > 0 {
		menuItems, err := s.repo.GetActiveMenuItemsByIDs(ctx, itemIDs)
		if err != nil {
			return RedeemItemsResponse{}, fmt.Errorf("svc_bar_fetch_menu_items: %w", err)
		}
		if len(menuItems) != len(itemIDs) {
			return RedeemItemsResponse{}, httpx.NewBadRequest("one or more menu items are unavailable or deleted", nil)
		}
		menuByID = make(map[uuid.UUID]db.MenuItem, len(menuItems))
		for _, m := range menuItems {
			menuByID[m.ID] = m
			totalBill += m.UnitPrice * qtyByItem[m.ID]
		}
	}

	var resp RedeemItemsResponse

	err := s.repo.WithTx(ctx, func(txCtx context.Context, q db.Querier) error {
		// 1. Idempotency check prevents duplicate deductions on bartender retry/double-tap.
		if idemKey != "" {
			existing, err := q.GetBarTransactionByIdempotencyKey(txCtx, pgtype.Text{String: idemKey, Valid: true})
			if err == nil {
				resp = RedeemItemsResponse{
					TransactionID:    existing.ID,
					TicketID:         existing.TicketID,
					BarCode:          code,
					TotalBill:        existing.TotalBill,
					WalletDeducted:   existing.WalletDeducted,
					DeltaToCollect:   existing.UpiDeltaPaid,
					BalanceBefore:    existing.BalanceBefore,
					RemainingBalance: existing.BalanceAfter,
					IdempotentReplay: true,
				}
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("tx_bar_check_idempotency: %w", err)
			}
		}

		// 2. Row-level lock on the ticket prevents concurrent double-spend across simultaneous bartenders.
		ticket, err := q.GetTicketForUpdateByBarCode(txCtx, code)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NewNotFound("ticket not found for bar_code")
			}
			return fmt.Errorf("tx_bar_lock_ticket: %w", err)
		}

		if ticket.BookingStatus != db.BookingStatusCONFIRMED {
			return httpx.NewConflict("ticket booking has not been verified by admin", nil)
		}

		// Multi-use 1D bartender barcode expires automatically once the event ends.
		if time.Now().UTC().After(ticket.EventEndsAt) {
			return httpx.NewConflict("bartender barcode and F&B wallet expired after event end time", nil)
		}

		freeShotRedeemed := false
		if req.RedeemFreeShotGuestID != nil {
			if _, err := q.RedeemGuestFreeShotAtomic(txCtx, db.RedeemGuestFreeShotAtomicParams{
				ID:       *req.RedeemFreeShotGuestID,
				TicketID: ticket.ID,
			}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return httpx.NewConflict("free tequila shot is not eligible or has already been redeemed", nil)
				}
				return fmt.Errorf("tx_bar_free_shot: %w", err)
			}
			freeShotRedeemed = true
		}

		balanceBefore := ticket.FoodBalance
		settlement := CalculateBarSettlement(totalBill, balanceBefore)
		remainingBalance := balanceBefore

		if settlement.WalletDeducted > 0 {
			updated, err := q.DeductTicketFoodBalanceAtomic(txCtx, db.DeductTicketFoodBalanceAtomicParams{
				ID:          ticket.ID,
				FoodBalance: settlement.WalletDeducted,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return httpx.NewConflict("concurrent wallet update detected; please retry", nil)
				}
				return fmt.Errorf("tx_bar_deduct_wallet: %w", err)
			}
			remainingBalance = updated.FoodBalance
		}

		var txID uuid.UUID
		if totalBill > 0 {
			idemText := pgtype.Text{Valid: false}
			if idemKey != "" {
				idemText = pgtype.Text{String: idemKey, Valid: true}
			}

			barTx, err := q.CreateBarTransaction(txCtx, db.CreateBarTransactionParams{
				TicketID:       ticket.ID,
				BartenderID:    bartenderID,
				IdempotencyKey: idemText,
				TotalBill:      settlement.TotalBill,
				WalletDeducted: settlement.WalletDeducted,
				UpiDeltaPaid:   settlement.DeltaToCollect,
				BalanceBefore:  balanceBefore,
				BalanceAfter:   remainingBalance,
			})
			if err != nil {
				return fmt.Errorf("tx_bar_create_transaction: %w", err)
			}
			txID = barTx.ID

			for _, itemID := range itemIDs {
				m := menuByID[itemID]
				if _, err := q.CreateTransactionLineItem(txCtx, db.CreateTransactionLineItemParams{
					TransactionID:   barTx.ID,
					MenuItemID:      m.ID,
					Quantity:        qtyByItem[itemID],
					UnitPriceAtSale: m.UnitPrice,
				}); err != nil {
					return fmt.Errorf("tx_bar_create_line_item: %w", err)
				}
			}
		}

		resp = RedeemItemsResponse{
			TransactionID:    txID,
			TicketID:         ticket.ID,
			BarCode:          ticket.BarCode,
			TotalBill:        settlement.TotalBill,
			WalletDeducted:   settlement.WalletDeducted,
			DeltaToCollect:   settlement.DeltaToCollect,
			BalanceBefore:    balanceBefore,
			RemainingBalance: remainingBalance,
			FreeShotRedeemed: freeShotRedeemed,
		}
		return nil
	})
	if err != nil {
		// Handle concurrent double-tap with identical Idempotency-Key hitting unique index 23505.
		if idemKey != "" && isIdempotencyKeyViolation(err) {
			existing, lookupErr := s.repo.GetBarTransactionByIdempotencyKey(ctx, idemKey)
			if lookupErr == nil {
				return RedeemItemsResponse{
					TransactionID:    existing.ID,
					TicketID:         existing.TicketID,
					BarCode:          code,
					TotalBill:        existing.TotalBill,
					WalletDeducted:   existing.WalletDeducted,
					DeltaToCollect:   existing.UpiDeltaPaid,
					BalanceBefore:    existing.BalanceBefore,
					RemainingBalance: existing.BalanceAfter,
					IdempotentReplay: true,
				}, nil
			}
		}
		return RedeemItemsResponse{}, err
	}

	return resp, nil
}

func isIdempotencyKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return strings.Contains(pgErr.ConstraintName, "idempotency")
	}
	return false
}
