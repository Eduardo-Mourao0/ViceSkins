"use client";

import Image from "next/image";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";
import { Badge, ItemCard } from "@/components/item-card";
import { ListingDialog } from "@/components/listing-dialog";
import { MarketFilters } from "@/components/market-filters";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { useMarket, useMe } from "@/lib/queries";
import type { MarketListing } from "@/lib/types";

// Parâmetros da URL repassados para a API (mesmos nomes dos dois lados).
const API_PARAMS = ["q", "minPrice", "maxPrice", "wear", "minFloat", "maxFloat", "sort", "page"] as const;

export function Market() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const { data: me } = useMe();
  const [selected, setSelected] = useState<MarketListing | null>(null);

  const apiQuery = new URLSearchParams();
  for (const k of API_PARAMS) {
    const v = params.get(k);
    if (v) apiQuery.set(k, v);
  }
  apiQuery.set("pageSize", "30");
  const { data, isPending, isFetching, error } = useMarket(apiQuery.toString());

  /** Atualiza a URL; qualquer filtro novo volta para a página 1. */
  const setParams = useCallback(
    (patch: Record<string, string | null>, resetPage = true) => {
      const next = new URLSearchParams(params.toString());
      for (const [k, v] of Object.entries(patch)) {
        if (v === null || v === "") next.delete(k);
        else next.set(k, v);
      }
      if (resetPage) next.delete("page");
      const qs = next.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [params, router, pathname],
  );

  const page = Number(params.get("page") ?? "1") || 1;

  return (
    <div className="grid gap-6 lg:grid-cols-[16rem_1fr]">
      <MarketFilters params={params} setParams={setParams} />

      <section className="flex min-w-0 flex-col gap-4">
        {error ? (
          <Alert>{error.message}</Alert>
        ) : isPending ? (
          <div className="flex justify-center py-24 text-muted">
            <Spinner className="size-6" />
          </div>
        ) : data.listings.length === 0 ? (
          <EmptyState title="Nenhum anúncio encontrado">Tente afrouxar os filtros.</EmptyState>
        ) : (
          <>
            <div
              className={`grid grid-cols-2 gap-3 transition-opacity sm:grid-cols-3 xl:grid-cols-4 ${
                isFetching ? "opacity-60" : ""
              }`}
            >
              {data.listings.map((l) => {
                const mine = me?.id === l.seller.id;
                return (
                  <ItemCard
                    key={l.id}
                    item={l.item}
                    priceCents={l.priceCents}
                    onClick={() => setSelected(l)}
                    highlighted={mine}
                    badge={
                      mine ? (
                        <Badge>Seu</Badge>
                      ) : (
                        <Image
                          src={l.seller.avatarUrl}
                          alt={l.seller.displayName}
                          title={`Vendido por ${l.seller.displayName}`}
                          width={20}
                          height={20}
                          className="rounded-full ring-1 ring-line"
                        />
                      )
                    }
                  />
                );
              })}
            </div>

            <div className="flex items-center justify-center gap-3 pt-2">
              <Button
                variant="ghost"
                disabled={page <= 1}
                onClick={() => setParams({ page: page > 2 ? String(page - 1) : null }, false)}
              >
                ← Anterior
              </Button>
              <span className="text-sm text-muted tabular-nums">Página {page}</span>
              <Button
                variant="ghost"
                disabled={!data.hasMore}
                onClick={() => setParams({ page: String(page + 1) }, false)}
              >
                Próxima →
              </Button>
            </div>
          </>
        )}
      </section>

      {selected && (
        <ListingDialog
          key={selected.id}
          listing={selected}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}
