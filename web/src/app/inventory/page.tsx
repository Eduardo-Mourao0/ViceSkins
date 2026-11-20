"use client";

import { useEffect, useMemo, useState } from "react";
import { RequireLogin } from "@/components/require-login";
import { Badge, ItemCard } from "@/components/item-card";
import { ItemDialog } from "@/components/item-dialog";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { isApiError } from "@/lib/api";
import { timeAgo } from "@/lib/format";
import { useInventory, useRefreshInventory } from "@/lib/queries";
import type { InventoryItem } from "@/lib/types";

type Filter = "all" | "tradable" | "listed";

const filters: { id: Filter; label: string }[] = [
  { id: "all", label: "Todos" },
  { id: "tradable", label: "Negociáveis" },
  { id: "listed", label: "À venda" },
];

export default function InventoryPage() {
  return <RequireLogin>{(me) => <Inventory demo={me.steamId.startsWith("demo-")} />}</RequireLogin>;
}

function Inventory({ demo }: { demo: boolean }) {
  const { data: inv, isPending, error } = useInventory(true);
  const refresh = useRefreshInventory();
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<InventoryItem | null>(null);
  const [retryAt, setRetryAt] = useState<number | null>(null);
  const secondsLeft = useCountdown(retryAt);

  function doRefresh() {
    refresh.mutate(undefined, {
      onError: (e) => {
        if (isApiError(e, 429) && e.retryAfterSeconds) {
          setRetryAt(Date.now() + e.retryAfterSeconds * 1000);
        }
      },
      // Depois de um refresh bem-sucedido o backend aplica 5 min de cooldown.
      onSuccess: () => setRetryAt(Date.now() + 300_000),
    });
  }

  const items = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (inv?.items ?? []).filter((it) => {
      if (filter === "tradable" && !(it.tradable && it.marketable)) return false;
      if (filter === "listed" && !it.listing) return false;
      return !q || it.name.toLowerCase().includes(q);
    });
  }, [inv, filter, search]);

  const counts = useMemo(() => {
    const all = inv?.items ?? [];
    return {
      all: all.length,
      tradable: all.filter((i) => i.tradable && i.marketable).length,
      listed: all.filter((i) => i.listing).length,
    };
  }, [inv]);

  if (isPending) {
    return (
      <div className="flex justify-center py-24 text-muted">
        <Spinner className="size-6" />
      </div>
    );
  }
  if (error) return <Alert>Erro ao carregar o inventário: {error.message}</Alert>;

  const cooling = secondsLeft > 0;
  const refreshButton = demo ? (
    <span
      title="Contas demo não existem na Steam; os itens vieram da seed."
      className="rounded-lg border border-line px-3 py-2 text-xs text-muted"
    >
      Conta demo · sem sincronização
    </span>
  ) : (
    <Button
      variant="secondary"
      onClick={doRefresh}
      loading={refresh.isPending}
      disabled={cooling}
      title={cooling ? "A Steam limita consultas ao inventário" : undefined}
    >
      {cooling ? `Sincronizar em ${formatCountdown(secondsLeft)}` : "Sincronizar com a Steam"}
    </Button>
  );

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Inventário</h1>
          <p className="text-sm text-muted">
            {inv.syncedAt ? `Sincronizado ${timeAgo(inv.syncedAt)}` : "Ainda não sincronizado"}
            {" · "}
            {counts.all} itens
          </p>
        </div>
        {refreshButton}
      </div>

      <RefreshError error={refresh.error} />

      {!inv.syncedAt && inv.items.length === 0 ? (
        <EmptyState title="Importe seu inventário" action={refreshButton}>
          Buscamos os itens de CS2 do seu inventário na Steam. Ele precisa estar público.
        </EmptyState>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex rounded-lg border border-line bg-ink-900 p-1">
              {filters.map((f) => (
                <button
                  key={f.id}
                  onClick={() => setFilter(f.id)}
                  className={`rounded-md px-3 py-1.5 text-sm transition ${
                    filter === f.id ? "bg-ink-700 text-text" : "text-muted hover:text-text"
                  }`}
                >
                  {f.label} <span className="text-faint tabular-nums">{counts[f.id]}</span>
                </button>
              ))}
            </div>
            <input
              type="search"
              placeholder="Buscar por nome"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full rounded-lg border border-line bg-ink-900 px-3 py-2 text-sm outline-none placeholder:text-faint focus:border-cyan sm:w-64"
            />
          </div>

          {items.length === 0 ? (
            <EmptyState title="Nenhum item aqui">Tente outro filtro ou busca.</EmptyState>
          ) : (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
              {items.map((it) => {
                const sellable = it.tradable && it.marketable;
                return (
                  <ItemCard
                    key={it.id}
                    item={it}
                    onClick={it.listing || sellable ? () => setSelected(it) : undefined}
                    priceCents={it.listing?.priceCents}
                    badge={it.listing && <Badge>À venda</Badge>}
                    highlighted={!!it.listing}
                    dimmed={!sellable && !it.listing}
                    hint={
                      sellable ? (
                        <span className="text-cyan-soft opacity-0 transition group-hover:opacity-100">
                          Anunciar →
                        </span>
                      ) : (
                        <span className="text-faint">Não negociável</span>
                      )
                    }
                  />
                );
              })}
            </div>
          )}
        </>
      )}

      {selected && (
        <ItemDialog
          // key: troca de item recria o modal com estado limpo
          key={selected.id}
          item={selected}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

function RefreshError({ error }: { error: Error | null }) {
  if (!error || isApiError(error, 429)) return null;
  if (isApiError(error, 422)) {
    return (
      <Alert tone="warning">
        Seu inventário na Steam está privado. Em <b>Perfil → Editar perfil → Configurações de privacidade</b>,
        deixe <b>Inventário</b> como <b>Público</b> e tente de novo.
      </Alert>
    );
  }
  if (isApiError(error, 503)) {
    return <Alert tone="warning">A Steam está limitando as consultas agora. Tente daqui a alguns minutos.</Alert>;
  }
  return <Alert>Não foi possível sincronizar: {error.message}</Alert>;
}

function useCountdown(target: number | null): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!target) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [target]);
  return target ? Math.max(0, Math.ceil((target - now) / 1000)) : 0;
}

function formatCountdown(s: number): string {
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}
