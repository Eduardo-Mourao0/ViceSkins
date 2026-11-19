package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
)

type InventoryStore interface {
	GetUserByID(ctx context.Context, id int64) (repository.User, error)
	ClaimInventoryRefresh(ctx context.Context, arg repository.ClaimInventoryRefreshParams) (int64, error)
	ListInventoryByOwner(ctx context.Context, ownerID int64) ([]repository.InventoryItem, error)
	SyncInventory(ctx context.Context, ownerID int64, items []repository.UpsertInventoryItemParams) error
}

type InventoryFetcher interface {
	GetInventory(ctx context.Context, steamID string) ([]steam.InventoryItem, error)
}

type InventoryService struct {
	store    InventoryStore
	steam    InventoryFetcher
	cooldown time.Duration
	now      func() time.Time // trocável nos testes
}

func NewInventoryService(store InventoryStore, fetcher InventoryFetcher, cooldown time.Duration) *InventoryService {
	return &InventoryService{store: store, steam: fetcher, cooldown: cooldown, now: time.Now}
}

// List devolve o inventário em cache. Nunca chama a Steam.
func (s *InventoryService) List(ctx context.Context, userID int64) (domain.Inventory, error) {
	u, err := s.getUser(ctx, userID)
	if err != nil {
		return domain.Inventory{}, err
	}
	rows, err := s.store.ListInventoryByOwner(ctx, userID)
	if err != nil {
		return domain.Inventory{}, fmt.Errorf("list inventory: %w", err)
	}

	items := make([]domain.InventoryItem, 0, len(rows))
	for _, r := range rows {
		it, err := toDomainItem(r)
		if err != nil {
			return domain.Inventory{}, err
		}
		items = append(items, it)
	}
	return domain.Inventory{Items: items, SyncedAt: u.InventorySyncedAt}, nil
}

// Refresh busca o inventário na Steam e atualiza o cache, respeitando o cooldown.
func (s *InventoryService) Refresh(ctx context.Context, userID int64) (domain.Inventory, error) {
	u, err := s.getUser(ctx, userID)
	if err != nil {
		return domain.Inventory{}, err
	}

	now := s.now()
	_, err = s.store.ClaimInventoryRefresh(ctx, repository.ClaimInventoryRefreshParams{
		UserID: userID,
		Now:    now,
		Cutoff: now.Add(-s.cooldown),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Inventory{}, s.cooldownError(u, now)
	}
	if err != nil {
		return domain.Inventory{}, fmt.Errorf("claim inventory refresh: %w", err)
	}

	steamItems, err := s.steam.GetInventory(ctx, u.SteamID)
	switch {
	case errors.Is(err, steam.ErrInventoryPrivate):
		return domain.Inventory{}, domain.ErrInventoryPrivate
	case errors.Is(err, steam.ErrRateLimited):
		return domain.Inventory{}, domain.ErrUpstreamRateLimited
	case err != nil:
		return domain.Inventory{}, fmt.Errorf("fetch steam inventory: %w", err)
	}

	params := make([]repository.UpsertInventoryItemParams, 0, len(steamItems))
	for _, it := range steamItems {
		p, err := toUpsertParams(it)
		if err != nil {
			return domain.Inventory{}, err
		}
		params = append(params, p)
	}
	if err := s.store.SyncInventory(ctx, userID, params); err != nil {
		return domain.Inventory{}, fmt.Errorf("sync inventory: %w", err)
	}

	return s.List(ctx, userID)
}

func (s *InventoryService) getUser(ctx context.Context, id int64) (repository.User, error) {
	u, err := s.store.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.User{}, domain.ErrNotFound
	}
	if err != nil {
		return repository.User{}, fmt.Errorf("get user: %w", err)
	}
	return u, nil
}

func (s *InventoryService) cooldownError(u repository.User, now time.Time) error {
	retry := s.cooldown
	if u.InventoryRefreshAt != nil {
		retry = u.InventoryRefreshAt.Add(s.cooldown).Sub(now)
	}
	if retry < time.Second {
		retry = time.Second
	}
	return &domain.CooldownError{RetryAfter: retry}
}

// Formato em que os stickers ficam guardados na coluna JSONB.
type storedSticker struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

func toUpsertParams(it steam.InventoryItem) (repository.UpsertInventoryItemParams, error) {
	stickers := make([]storedSticker, 0, len(it.Stickers))
	for _, s := range it.Stickers {
		stickers = append(stickers, storedSticker(s))
	}
	raw, err := json.Marshal(stickers)
	if err != nil {
		return repository.UpsertInventoryItemParams{}, fmt.Errorf("marshal stickers: %w", err)
	}

	return repository.UpsertInventoryItemParams{
		AssetID:        it.AssetID,
		ClassID:        it.ClassID,
		InstanceID:     it.InstanceID,
		MarketHashName: it.MarketHashName,
		Name:           it.Name,
		ItemType:       it.Type,
		Weapon:         it.Weapon,
		Rarity:         it.Rarity,
		RarityColor:    it.RarityColor,
		Exterior:       it.Exterior,
		FloatValue:     it.Float,
		PaintSeed:      it.PaintSeed,
		IconURL:        it.IconURL,
		Stickers:       raw,
		Tradable:       it.Tradable,
		Marketable:     it.Marketable,
	}, nil
}

func toDomainItem(r repository.InventoryItem) (domain.InventoryItem, error) {
	var stored []storedSticker
	if err := json.Unmarshal(r.Stickers, &stored); err != nil {
		return domain.InventoryItem{}, fmt.Errorf("unmarshal stickers of item %d: %w", r.ID, err)
	}
	stickers := make([]domain.Sticker, 0, len(stored))
	for _, s := range stored {
		stickers = append(stickers, domain.Sticker(s))
	}

	return domain.InventoryItem{
		ID:             r.ID,
		AssetID:        r.AssetID,
		Name:           r.Name,
		MarketHashName: r.MarketHashName,
		Type:           r.ItemType,
		Weapon:         r.Weapon,
		Rarity:         r.Rarity,
		RarityColor:    r.RarityColor,
		Exterior:       r.Exterior,
		Float:          r.FloatValue,
		PaintSeed:      r.PaintSeed,
		ImageURL:       r.IconURL,
		Stickers:       stickers,
		Tradable:       r.Tradable,
		Marketable:     r.Marketable,
	}, nil
}
