"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { RequireLogin } from "@/components/require-login";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { formatDate, formatFloat, formatPrice } from "@/lib/format";
import { useCancelListing, useMyListings } from "@/lib/queries";
import type { Listing, ListingStatus } from "@/lib/types";

type Tab = ListingStatus | "all";

const tabs: { id: Tab; label: string }[] = [
  { id: "active", label: "Ativos" },
  { id: "sold", label: "Vendidos" },
  { id: "cancelled", label: "Cancelados" },
  { id: "all", label: "Todos" },
];

const statusStyle: Record<ListingStatus, { label: string; className: string }> = {
  active: { label: "À venda", className: "bg-pink/15 text-pink" },
  sold: { label: "Vendido", className: "bg-success/15 text-success" },
  cancelled: { label: "Cancelado", className: "bg-ink-700 text-muted" },
};

export default function MyListingsPage() {
  return <RequireLogin>{() => <MyListings />}</RequireLogin>;
}

function MyListings() {
  const [tab, setTab] = useState<Tab>("active");
  const { data: listings, isPending, error } = useMyListings(tab, true);
  const cancel = useCancelListing();

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold">Meus anúncios</h1>
        <p className="text-sm text-muted">Itens que você colocou à venda.</p>
      </div>

      <div className="flex w-fit rounded-lg border border-line bg-ink-900 p-1">
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`rounded-md px-3 py-1.5 text-sm transition ${
              tab === t.id ? "bg-ink-700 text-text" : "text-muted hover:text-text"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {cancel.error && <Alert>{cancel.error.message}</Alert>}

      {isPending ? (
        <div className="flex justify-center py-16 text-muted">
          <Spinner className="size-6" />
        </div>
      ) : error ? (
        <Alert>Erro ao carregar anúncios: {error.message}</Alert>
      ) : listings.length === 0 ? (
        <EmptyState
          title={tab === "active" ? "Nenhum item à venda" : "Nada por aqui"}
          action={
            tab === "active" && (
              <Link href="/inventory" className="text-sm text-cyan-soft underline-offset-4 hover:underline">
                Anunciar um item do inventário
              </Link>
            )
          }
        />
      ) : (
        <ul className="card divide-y divide-line/70">
          {listings.map((l) => (
            <ListingRow
              key={l.id}
              listing={l}
              onCancel={() => cancel.mutate(l.id)}
              cancelling={cancel.isPending && cancel.variables === l.id}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function ListingRow({
  listing: l,
  onCancel,
  cancelling,
}: {
  listing: Listing;
  onCancel: () => void;
  cancelling: boolean;
}) {
  const st = statusStyle[l.status];
  return (
    <li className="flex flex-wrap items-center gap-4 p-4">
      <div
        className="flex h-14 w-20 shrink-0 items-center justify-center rounded-lg bg-ink-850"
        style={{ boxShadow: `inset 0 -2px 0 #${l.item?.rarityColor || "6f6290"}` }}
      >
        {l.item && (
          <Image src={l.item.imageUrl} alt="" width={80} height={60} className="h-11 w-auto object-contain" />
        )}
      </div>

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{l.item?.name ?? `Item #${l.itemId}`}</p>
        <p className="text-xs text-muted">
          {[l.item?.exterior, l.item?.float != null && `float ${formatFloat(l.item.float)}`]
            .filter(Boolean)
            .join(" · ")}
        </p>
        <p className="mt-0.5 text-[11px] text-faint">Criado em {formatDate(l.createdAt)}</p>
      </div>

      <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${st.className}`}>{st.label}</span>

      <p className="w-28 text-right font-semibold tabular-nums">{formatPrice(l.priceCents)}</p>

      <div className="w-36 text-right">
        {l.status === "active" && (
          <Button variant="danger" onClick={onCancel} loading={cancelling}>
            Cancelar
          </Button>
        )}
      </div>
    </li>
  );
}
