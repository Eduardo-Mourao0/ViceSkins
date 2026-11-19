package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
)

// fakeStore guarda tudo em memória e imita o comportamento das queries.
type fakeStore struct {
	user   repository.User
	items  []repository.InventoryItem
	synced []repository.UpsertInventoryItemParams
}

func (f *fakeStore) GetUserByID(_ context.Context, id int64) (repository.User, error) {
	if id != f.user.ID {
		return repository.User{}, pgx.ErrNoRows
	}
	return f.user, nil
}

func (f *fakeStore) ClaimInventoryRefresh(_ context.Context, arg repository.ClaimInventoryRefreshParams) (int64, error) {
	last := f.user.InventoryRefreshAt
	if last != nil && !last.Before(arg.Cutoff) {
		return 0, pgx.ErrNoRows
	}
	f.user.InventoryRefreshAt = &arg.Now
	return f.user.ID, nil
}

func (f *fakeStore) ListInventoryByOwner(context.Context, int64) ([]repository.InventoryItem, error) {
	return f.items, nil
}

func (f *fakeStore) SyncInventory(_ context.Context, ownerID int64, items []repository.UpsertInventoryItemParams) error {
	f.synced = items
	f.items = nil
	for i, p := range items {
		f.items = append(f.items, repository.InventoryItem{
			ID: int64(i + 1), OwnerID: ownerID, AssetID: p.AssetID, Name: p.Name,
			FloatValue: p.FloatValue, Stickers: p.Stickers, Tradable: p.Tradable,
		})
	}
	now := time.Now()
	f.user.InventorySyncedAt = &now
	return nil
}

type fakeFetcher struct {
	items []steam.InventoryItem
	err   error
	calls int
}

func (f *fakeFetcher) GetInventory(context.Context, string) ([]steam.InventoryItem, error) {
	f.calls++
	return f.items, f.err
}

func ptr[T any](v T) *T { return &v }

func newInventoryService(store *fakeStore, fetcher *fakeFetcher, now time.Time) *InventoryService {
	s := NewInventoryService(store, fetcher, 5*time.Minute)
	s.now = func() time.Time { return now }
	return s
}

func TestRefreshSyncsAndReturnsItems(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{user: repository.User{ID: 1, SteamID: "765"}}
	fetcher := &fakeFetcher{items: []steam.InventoryItem{{
		AssetID:  "a1",
		Name:     "AK-47 | Redline",
		Float:    ptr(0.15),
		Stickers: []steam.Sticker{{Kind: "sticker", Name: "Falcons", ImageURL: "u"}},
		Tradable: true,
	}}}

	inv, err := newInventoryService(store, fetcher, now).Refresh(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) != 1 || inv.Items[0].AssetID != "a1" || *inv.Items[0].Float != 0.15 {
		t.Fatalf("items = %+v", inv.Items)
	}
	if got := inv.Items[0].Stickers; len(got) != 1 || got[0].Name != "Falcons" {
		t.Fatalf("stickers = %+v", got)
	}
	if inv.SyncedAt == nil {
		t.Fatal("SyncedAt should be set")
	}
	if string(store.synced[0].Stickers) != `[{"kind":"sticker","name":"Falcons","imageUrl":"u"}]` {
		t.Fatalf("stored stickers json = %s", store.synced[0].Stickers)
	}
}

func TestRefreshRespectsCooldown(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{user: repository.User{
		ID: 1, SteamID: "765",
		InventoryRefreshAt: ptr(now.Add(-2 * time.Minute)), // atualizou há 2 min
	}}
	fetcher := &fakeFetcher{}

	_, err := newInventoryService(store, fetcher, now).Refresh(context.Background(), 1)

	var cd *domain.CooldownError
	if !errors.As(err, &cd) {
		t.Fatalf("err = %v, want *CooldownError", err)
	}
	if cd.RetryAfter != 3*time.Minute {
		t.Fatalf("RetryAfter = %s, want 3m", cd.RetryAfter)
	}
	if fetcher.calls != 0 {
		t.Fatal("steam should not be called during cooldown")
	}
}

func TestRefreshAllowedAfterCooldown(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{user: repository.User{
		ID: 1, SteamID: "765",
		InventoryRefreshAt: ptr(now.Add(-6 * time.Minute)),
	}}
	fetcher := &fakeFetcher{}

	if _, err := newInventoryService(store, fetcher, now).Refresh(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != 1 {
		t.Fatalf("steam calls = %d, want 1", fetcher.calls)
	}
}

func TestRefreshMapsSteamErrors(t *testing.T) {
	tests := []struct {
		steamErr error
		want     error
	}{
		{steam.ErrInventoryPrivate, domain.ErrInventoryPrivate},
		{steam.ErrRateLimited, domain.ErrUpstreamRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.want.Error(), func(t *testing.T) {
			store := &fakeStore{user: repository.User{ID: 1, SteamID: "765"}}
			fetcher := &fakeFetcher{err: tt.steamErr}

			_, err := newInventoryService(store, fetcher, time.Now()).Refresh(context.Background(), 1)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if store.synced != nil {
				t.Fatal("cache must not change when steam fails")
			}
		})
	}
}
