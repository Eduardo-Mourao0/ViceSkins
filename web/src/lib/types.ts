// Espelho dos DTOs da API Go (internal/handler).

export type User = {
  id: number;
  steamId: string; // string: SteamID64 passa do limite de inteiro seguro do JS
  displayName: string;
  avatarUrl: string;
  balanceCents: number;
  tradeUrl: string | null; // obrigatória para comprar e anunciar
  hasSteamApiKey: boolean; // a key em si nunca vem da API
  steamApiKeyLast4: string | null;
  role: "user" | "admin";
  debtCents: number; // deve à plataforma: não compra até pagar
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

export type TxStatus = "pending" | "sent" | "on_hold" | "completed" | "cancelled" | "failed";

export type Transaction = {
  id: number;
  listingId: number;
  itemId: number;
  buyerId: number;
  sellerId: number;
  role: "buyer" | "seller";
  priceCents: number;
  feeCents: number;
  sellerReceivesCents: number;
  status: TxStatus;
  tradeOfferId: string | null;
  holdUntil: string | null;
  failureReason: string | null;
  deadlineAt: string | null; // prazo da etapa atual (pending/sent)
  buyerTradeUrl: string | null; // só para o vendedor, enquanto precisa enviar a oferta
  lateDeliveryAt: string | null; // item chegou ao comprador depois do estorno: em análise
  createdAt: string;
  updatedAt: string;
};

export type TransactionDetail = Transaction & {
  item: InventoryItem;
  buyer: PublicUser;
  seller: PublicUser;
};

export type TxPage = {
  transactions: TransactionDetail[];
  nextCursor: number | null;
};

export type TxSummary = {
  purchases: number;
  spentCents: number;
  sales: number;
  earnedCents: number;
  pendingEarningsCents: number;
  feesPaidCents: number;
};

export type TxRoleFilter = "all" | "buyer" | "seller";
export type TxStatusFilter = "all" | "open" | "closed";

export type WalletEntryKind =
  | "deposit"
  | "purchase"
  | "sale"
  | "refund"
  | "fee"
  | "dispute_charge"
  | "dispute_collect"
  | "dispute_payout"
  | "dispute_refund"
  | "debt_payment"
  | "debt_recovery";

export type WalletEntry = {
  id: number;
  amountCents: number; // > 0 crédito, < 0 débito
  balanceAfterCents: number;
  kind: WalletEntryKind;
  transactionId: number | null;
  description: string;
  createdAt: string;
};

export type Wallet = {
  balanceCents: number;
  entries: WalletEntry[];
  lockedCents: number; // vendas em proteção de troca: ainda fora do saldo
  lockedSales: number;
  nextReleaseAt: string | null;
};

/** Dados públicos de um usuário (vendedor, comprador). */
export type PublicUser = {
  id: number;
  displayName: string;
  avatarUrl: string;
};

export type Seller = PublicUser;

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

export type DisputeStatus = "holding" | "settled" | "reversed" | "dismissed";

/** Entrega tardia: o item chegou ao comprador depois do estorno. */
export type Dispute = {
  id: number;
  status: DisputeStatus;
  detectedAt: string;
  refundedAt: string;
  evidence: "fingerprint" | "offer";
  tradedAt: string | null;
  chargedCents: number;
  coveredCents: number;
  holdUntil: string;
  resolvedAt: string | null;
  resolvedBy: number | null;
  note: string | null;
  transactionId: number;
  priceCents: number;
  sellerReceivesCents: number;
  failureReason: string | null;
  itemName: string;
  itemImageUrl: string;
  buyerName: string;
  buyerDebtCents: number;
  sellerName: string;
};

export type DepositStatus = "pending" | "paid" | "expired";

/** Depósito via Pix: o saldo só entra quando a cobrança é paga. */
export type Deposit = {
  id: number;
  amountCents: number;
  status: DepositStatus;
  brCode: string; // Pix copia e cola
  expiresAt: string;
  paidAt: string | null;
  createdAt: string;
  canSimulate: boolean; // só no ambiente com Pix simulado
};
