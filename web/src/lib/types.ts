// Espelho dos DTOs da API Go (internal/handler).

export type User = {
  id: number;
  steamId: string; // string: SteamID64 passa do limite de inteiro seguro do JS
  displayName: string;
  avatarUrl: string;
  balanceCents: number;
};

export type Sticker = {
  kind: string; // sticker, patch, charm...
  name: string;
  imageUrl: string;
};

export type InventoryItem = {
  id: number;
  assetId: string;
  name: string;
  marketHashName: string;
  type: string;
  weapon: string;
  rarity: string;
  rarityColor: string;
  exterior: string;
  float: number | null;
  paintSeed: number | null;
  imageUrl: string;
  stickers: Sticker[];
  tradable: boolean;
  marketable: boolean;
  listing?: { id: number; priceCents: number };
};

export type Inventory = {
  items: InventoryItem[];
  syncedAt: string | null;
};

export type ListingStatus = "active" | "sold" | "cancelled";

export type Seller = {
  id: number;
  displayName: string;
  avatarUrl: string;
};

export type MarketListing = Listing & { item: InventoryItem; seller: Seller };

export type MarketPage = {
  listings: MarketListing[];
  page: number;
  pageSize: number;
  hasMore: boolean;
};

export type MarketSort = "newest" | "price_asc" | "price_desc" | "float_asc";

export type Listing = {
  id: number;
  itemId: number;
  priceCents: number;
  status: ListingStatus;
  createdAt: string;
  updatedAt: string;
  item?: InventoryItem;
};
