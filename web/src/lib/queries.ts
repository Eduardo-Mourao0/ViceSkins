"use client";

import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, isApiError } from "./api";
import type { Inventory, Listing, ListingStatus, MarketListing, MarketPage, User } from "./types";

export const keys = {
  me: ["me"] as const,
  inventory: ["inventory"] as const,
  myListings: (status: ListingStatus | "all") => ["my-listings", status] as const,
  market: (query: string) => ["market", query] as const,
  listing: (id: number) => ["listing", id] as const,
};

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
