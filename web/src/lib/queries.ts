"use client";

import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api, isApiError } from "./api";
import type {
  Inventory,
  Listing,
  ListingStatus,
  MarketListing,
  MarketPage,
  Transaction,
  TransactionDetail,
  TxPage,
  TxRoleFilter,
  TxStatus,
  TxStatusFilter,
  TxSummary,
  User,
  Wallet,
  WalletEntry,
} from "./types";

export const keys = {
  me: ["me"] as const,
  inventory: ["inventory"] as const,
  myListings: (status: ListingStatus | "all") => ["my-listings", status] as const,
  market: (query: string) => ["market", query] as const,
  listing: (id: number) => ["listing", id] as const,
  wallet: ["wallet"] as const,
  transaction: (id: number) => ["transaction", id] as const,
};

const FINAL: TxStatus[] = ["completed", "cancelled", "failed"];

/** Acompanha uma transação; consulta a cada 3s enquanto ela ainda pode mudar de status. */
export function useTransaction(id: number | null) {
  return useQuery({
    queryKey: keys.transaction(id ?? 0),
    queryFn: () => api<TransactionDetail>(`/transactions/${id}`),
    enabled: id !== null,
    refetchInterval: (q) => (q.state.data && FINAL.includes(q.state.data.status) ? false : 3_000),
  });
}

/** Tudo que muda quando dinheiro ou itens mudam de mãos. */
function useInvalidateMoneyViews() {
  const qc = useQueryClient();
  return () => {
    for (const key of [
      keys.me,
      keys.wallet,
      keys.inventory,
      ["market"],
      ["listing"],
      ["my-listings"],
      ["transactions"],
      ["tx-summary"],
    ]) {
      qc.invalidateQueries({ queryKey: key });
    }
  };
}

export function useBuy() {
  const invalidate = useInvalidateMoneyViews();
  return useMutation({
    // A resposta é a transação "crua"; o detalhe (com item e partes) vem do useTransaction.
    mutationFn: (listingId: number) => api<Transaction>(`/listings/${listingId}/buy`, { method: "POST" }),
    onSuccess: invalidate,
  });
}

export function useCancelTransaction() {
  const qc = useQueryClient();
  const invalidate = useInvalidateMoneyViews();
  return useMutation({
    mutationFn: (txId: number) => api<TransactionDetail>(`/transactions/${txId}/cancel`, { method: "POST" }),
    onSuccess: (tx) => {
      qc.setQueryData(keys.transaction(tx.id), tx);
      invalidate();
    },
  });
}

export function useWallet(enabled: boolean) {
  return useQuery({
    queryKey: keys.wallet,
    queryFn: () => api<Wallet>("/me/wallet?limit=100"),
    enabled,
  });
}

export function useDeposit() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (amountCents: number) =>
      api<WalletEntry>("/wallet/deposits", { method: "POST", body: JSON.stringify({ amountCents }) }),
    onSuccess: (entry) => {
      // Atualiza o saldo do header na hora, sem esperar outra requisição.
      qc.setQueryData<User | null>(keys.me, (me) => me && { ...me, balanceCents: entry.balanceAfterCents });
      qc.invalidateQueries({ queryKey: keys.wallet });
    },
  });
}

/** Mercado público. query é a query string já pronta (ex.: "q=ak&sort=price_asc"). */
export function useMarket(query: string) {
  return useQuery({
    queryKey: keys.market(query),
    queryFn: () => api<MarketPage>(`/listings${query ? `?${query}` : ""}`),
    // Ao trocar filtro/página, mantém os resultados anteriores na tela até chegar o novo.
    placeholderData: keepPreviousData,
  });
}

export function useListing(id: number) {
  return useQuery({
    queryKey: keys.listing(id),
    queryFn: () => api<MarketListing>(`/listings/${id}`),
  });
}

/** Usuário logado, ou null se não houver sessão. */
export function useMe() {
  return useQuery({
    queryKey: keys.me,
    queryFn: async () => {
      try {
        return await api<User>("/me");
      } catch (e) {
        if (isApiError(e, 401)) return null;
        throw e;
      }
    },
    staleTime: 60_000,
  });
}

/** Contas da seed de demonstração. Lista vazia quando o login demo está desligado (404). */
export function useDemoUsers() {
  return useQuery({
    queryKey: ["demo-users"],
    queryFn: async () => {
      try {
        return (await api<{ users: User[] }>("/auth/dev/users")).users;
      } catch (e) {
        if (isApiError(e, 404)) return [];
        throw e;
      }
    },
    staleTime: Infinity,
    retry: false,
  });
}

export function useDemoLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (userId: number) =>
      api<User>("/auth/dev/login", { method: "POST", body: JSON.stringify({ userId }) }),
    onSuccess: async (user) => {
      // Trocou de usuário: nada do cache anterior vale mais.
      await qc.resetQueries();
      qc.setQueryData(keys.me, user);
    },
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<void>("/auth/logout", { method: "POST" }),
    onSuccess: () => {
      qc.setQueryData(keys.me, null);
      qc.removeQueries({ queryKey: keys.inventory });
      qc.removeQueries({ queryKey: ["my-listings"] });
    },
  });
}

export function useInventory(enabled: boolean) {
  return useQuery({
    queryKey: keys.inventory,
    queryFn: () => api<Inventory>("/me/inventory"),
    enabled,
  });
}

export function useRefreshInventory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<Inventory>("/me/inventory/refresh", { method: "POST" }),
    onSuccess: (inv) => {
      qc.setQueryData(keys.inventory, inv);
      qc.invalidateQueries({ queryKey: ["my-listings"] });
    },
  });
}

export function useMyListings(status: ListingStatus | "all", enabled: boolean) {
  return useQuery({
    queryKey: keys.myListings(status),
    queryFn: () =>
      api<{ listings: Listing[] }>(
        status === "all" ? "/me/listings" : `/me/listings?status=${status}`,
      ).then((r) => r.listings),
    enabled,
  });
}

function useInvalidateListingViews() {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: keys.inventory });
    qc.invalidateQueries({ queryKey: ["my-listings"] });
    qc.invalidateQueries({ queryKey: ["market"] });
    qc.invalidateQueries({ queryKey: ["listing"] });
  };
}

export function useCreateListing() {
  const invalidate = useInvalidateListingViews();
  return useMutation({
    mutationFn: (input: { itemId: number; priceCents: number }) =>
      api<Listing>("/listings", { method: "POST", body: JSON.stringify(input) }),
    onSuccess: invalidate,
  });
}

export function useCancelListing() {
  const invalidate = useInvalidateListingViews();
  return useMutation({
    mutationFn: (listingId: number) =>
      api<Listing>(`/listings/${listingId}/cancel`, { method: "POST" }),
    onSuccess: invalidate,
  });
}

/** Histórico paginado por cursor ("carregar mais"). */
export function useTransactions(role: TxRoleFilter, status: TxStatusFilter) {
  return useInfiniteQuery({
    queryKey: ["transactions", role, status],
    queryFn: ({ pageParam }) => {
      const qs = new URLSearchParams({ limit: "20" });
      if (role !== "all") qs.set("role", role);
      if (status !== "all") qs.set("status", status);
      if (pageParam) qs.set("before", String(pageParam));
      return api<TxPage>(`/me/transactions?${qs}`);
    },
    initialPageParam: 0,
    getNextPageParam: (last) => last.nextCursor ?? undefined,
  });
}

export function useTxSummary() {
  return useQuery({
    queryKey: ["tx-summary"],
    queryFn: () => api<TxSummary>("/me/transactions/summary"),
  });
}

export function useSetTradeURL() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (tradeUrl: string) =>
      api<User>("/me/trade-url", { method: "PUT", body: JSON.stringify({ tradeUrl }) }),
    onSuccess: (me) => qc.setQueryData(keys.me, me),
  });
}

/** Vendedor avisa que enviou a oferta (sem API key a plataforma não a vê). */
export function useMarkSent() {
  const qc = useQueryClient();
  const invalidate = useInvalidateMoneyViews();
  return useMutation({
    mutationFn: (txId: number) => api<TransactionDetail>(`/transactions/${txId}/sent`, { method: "POST" }),
    onSuccess: (tx) => {
      qc.setQueryData(keys.transaction(tx.id), tx);
      invalidate();
    },
  });
}

export function useSetSteamAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (apiKey: string) =>
      api<User>("/me/steam-api-key", { method: "PUT", body: JSON.stringify({ apiKey }) }),
    onSuccess: (me) => qc.setQueryData(keys.me, me),
  });
}

export function useDeleteSteamAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<User>("/me/steam-api-key", { method: "DELETE" }),
    onSuccess: (me) => qc.setQueryData(keys.me, me),
  });
}
