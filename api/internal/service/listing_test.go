package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

// fakeListingStore imita o banco em memória, inclusive o índice único
// "uma listing ativa por item".
type fakeListingStore struct {
	items    map[int64]repository.InventoryItem
	listings map[int64]repository.Listing
	nextID   int64

	noTradeURL bool // vendedor sem trade URL cadastrada
	hasAPIKey  bool // vendedor com API key da Steam cadastrada
}

func newFakeListingStore(items ...repository.InventoryItem) *fakeListingStore {
	f := &fakeListingStore{items: map[int64]repository.InventoryItem{}, listings: map[int64]repository.Listing{}}
	for _, it := range items {
		f.items[it.ID] = it
	}
	return f
}

func (f *fakeListingStore) GetUserByID(_ context.Context, id int64) (repository.User, error) {
	u := repository.User{ID: id}
	if !f.noTradeURL {
		u.TradeURL = ptr("https://steamcommunity.com/tradeoffer/new/?partner=1&token=abcdefgh")
	}
	if f.hasAPIKey {
		u.SteamAPIKeyEnc = []byte("sealed")
	}
	return u, nil
}

func (f *fakeListingStore) GetInventoryItem(_ context.Context, id int64) (repository.InventoryItem, error) {
	it, ok := f.items[id]
	if !ok {
		return repository.InventoryItem{}, pgx.ErrNoRows
	}
	return it, nil
}

func (f *fakeListingStore) CreateListing(_ context.Context, arg repository.CreateListingParams) (repository.Listing, error) {
	for _, l := range f.listings {
		if l.ItemID == arg.ItemID && l.Status == "active" {
			return repository.Listing{}, &pgconn.PgError{Code: "23505", ConstraintName: constraintOneActivePerItem}
		}
	}
	f.nextID++
	l := repository.Listing{ID: f.nextID, ItemID: arg.ItemID, SellerID: arg.SellerID, PriceCents: arg.PriceCents, Status: "active", CreatedAt: time.Now()}
	f.listings[l.ID] = l
	return l, nil
}

func (f *fakeListingStore) GetListing(_ context.Context, id int64) (repository.Listing, error) {
	l, ok := f.listings[id]
	if !ok {
		return repository.Listing{}, pgx.ErrNoRows
	}
	return l, nil
}

func (f *fakeListingStore) CancelListing(_ context.Context, arg repository.CancelListingParams) (repository.Listing, error) {
	l, ok := f.listings[arg.ID]
	if !ok || l.SellerID != arg.SellerID || l.Status != "active" {
		return repository.Listing{}, pgx.ErrNoRows
	}
	l.Status = "cancelled"
	f.listings[l.ID] = l
	return l, nil
}

func (f *fakeListingStore) ListListingsBySeller(context.Context, repository.ListListingsBySellerParams) ([]repository.ListListingsBySellerRow, error) {
	return nil, nil
}

const (
	seller = int64(1)
	other  = int64(2)
)

func tradableItem(id, owner int64) repository.InventoryItem {
	return repository.InventoryItem{ID: id, OwnerID: owner, Tradable: true, Marketable: true, Stickers: []byte("[]")}
}

func TestCreateListing(t *testing.T) {
	removed := tradableItem(3, seller)
	removed.RemovedAt = ptr(time.Now())
	notTradable := tradableItem(4, seller)
	notTradable.Tradable = false

	tests := []struct {
		name   string
		itemID int64
		price  int64
		want   error
	}{
		{"ok", 1, 1500, nil},
		{"price too low", 1, 5, domain.ErrInvalidInput},
		{"price too high", 1, domain.MaxListingPriceCents + 1, domain.ErrInvalidInput},
		{"item does not exist", 99, 1500, domain.ErrNotFound},
		{"item of another user", 2, 1500, domain.ErrNotFound},
		{"item removed from inventory", 3, 1500, domain.ErrNotFound},
		{"item not tradable", 4, 1500, domain.ErrItemNotTradable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeListingStore(tradableItem(1, seller), tradableItem(2, other), removed, notTradable)
			l, err := NewListingService(store).Create(context.Background(), seller, tt.itemID, tt.price)

			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("err = %v, want %v", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if l.Status != domain.ListingActive || l.PriceCents != tt.price || l.SellerID != seller {
				t.Fatalf("listing = %+v", l)
			}
		})
	}
}

func TestCreateListingTwiceForSameItem(t *testing.T) {
	svc := NewListingService(newFakeListingStore(tradableItem(1, seller)))
	if _, err := svc.Create(context.Background(), seller, 1, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), seller, 1, 2000); !errors.Is(err, domain.ErrItemAlreadyListed) {
		t.Fatalf("err = %v, want ErrItemAlreadyListed", err)
	}
}

func TestRelistAfterCancel(t *testing.T) {
	svc := NewListingService(newFakeListingStore(tradableItem(1, seller)))
	l, _ := svc.Create(context.Background(), seller, 1, 1000)
	if _, err := svc.Cancel(context.Background(), seller, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), seller, 1, 900); err != nil {
		t.Fatalf("relist after cancel should work, got %v", err)
	}
}

func TestCancelListing(t *testing.T) {
	tests := []struct {
		name  string
		setup func(svc *ListingService) int64 // devolve o id da listing a cancelar
		by    int64
		want  error
	}{
		{
			name: "ok",
			setup: func(svc *ListingService) int64 {
				l, _ := svc.Create(context.Background(), seller, 1, 1000)
				return l.ID
			},
			by: seller,
		},
		{
			name:  "not found",
			setup: func(*ListingService) int64 { return 42 },
			by:    seller,
			want:  domain.ErrNotFound,
		},
		{
			name: "someone else's listing",
			setup: func(svc *ListingService) int64 {
				l, _ := svc.Create(context.Background(), seller, 1, 1000)
				return l.ID
			},
			by:   other,
			want: domain.ErrForbidden,
		},
		{
			name: "already cancelled",
			setup: func(svc *ListingService) int64 {
				l, _ := svc.Create(context.Background(), seller, 1, 1000)
				svc.Cancel(context.Background(), seller, l.ID)
				return l.ID
			},
			by:   seller,
			want: domain.ErrListingNotActive,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewListingService(newFakeListingStore(tradableItem(1, seller)))
			id := tt.setup(svc)

			l, err := svc.Cancel(context.Background(), tt.by, id)

			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("err = %v, want %v", err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if l.Status != domain.ListingCancelled {
				t.Fatalf("status = %s, want cancelled", l.Status)
			}
		})
	}
}

func TestCreateListingRequiresTradeURL(t *testing.T) {
	store := newFakeListingStore(tradableItem(1, seller))
	store.noTradeURL = true
	if _, err := NewListingService(store).Create(context.Background(), seller, 1, 1500); !errors.Is(err, domain.ErrTradeURLRequired) {
		t.Fatalf("err = %v, want ErrTradeURLRequired", err)
	}
	if len(store.listings) != 0 {
		t.Fatal("no listing must be created")
	}
}

func TestFloatlessItemsNeedAPIKeyWithSteamVerification(t *testing.T) {
	withFloat := tradableItem(1, seller)
	withFloat.FloatValue = ptr(0.25)
	caseItem := tradableItem(2, seller) // sem float: caixa, sticker...

	t.Run("simulated: no extra rule", func(t *testing.T) {
		if _, err := NewListingService(newFakeListingStore(caseItem)).Create(context.Background(), seller, 2, 1500); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("steam: item with float is fine without key", func(t *testing.T) {
		svc := NewListingService(newFakeListingStore(withFloat), RequireAPIKeyForFloatless())
		if _, err := svc.Create(context.Background(), seller, 1, 1500); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("steam: item without float needs key", func(t *testing.T) {
		svc := NewListingService(newFakeListingStore(caseItem), RequireAPIKeyForFloatless())
		if _, err := svc.Create(context.Background(), seller, 2, 1500); !errors.Is(err, domain.ErrAPIKeyRequired) {
			t.Fatalf("err = %v, want ErrAPIKeyRequired", err)
		}
	})
	t.Run("steam: item without float with key", func(t *testing.T) {
		store := newFakeListingStore(caseItem)
		store.hasAPIKey = true
		if _, err := NewListingService(store, RequireAPIKeyForFloatless()).Create(context.Background(), seller, 2, 1500); err != nil {
			t.Fatal(err)
		}
	})
}
