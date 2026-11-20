"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { RequireLogin } from "@/components/require-login";
import { TxStatusBadge } from "@/components/tx-status";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { formatDate, formatPrice } from "@/lib/format";
import { useTransactions, useTxSummary } from "@/lib/queries";
import type { TransactionDetail, TxRoleFilter, TxStatusFilter } from "@/lib/types";

const roles: { id: TxRoleFilter; label: string }[] = [
  { id: "all", label: "Todas" },
  { id: "buyer", label: "Compras" },
  { id: "seller", label: "Vendas" },
];

const statuses: { id: TxStatusFilter; label: string }[] = [
  { id: "all", label: "Qualquer status" },
  { id: "open", label: "Em andamento" },
  { id: "closed", label: "Finalizadas" },
];

export default function TransactionsPage() {
  return <RequireLogin>{() => <Transactions />}</RequireLogin>;
}

function Transactions() {
  const [role, setRole] = useState<TxRoleFilter>("all");
  const [status, setStatus] = useState<TxStatusFilter>("all");
  const { data, isPending, error, fetchNextPage, hasNextPage, isFetchingNextPage } = useTransactions(role, status);
  const txs = data?.pages.flatMap((p) => p.transactions) ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold">Transações</h1>
        <p className="text-sm text-muted">Suas compras e vendas.</p>
      </div>

      <Summary />

      <div className="flex flex-wrap items-center gap-3">
        <Segmented options={roles} value={role} onChange={setRole} />
        <Segmented options={statuses} value={status} onChange={setStatus} />
      </div>

      {isPending ? (
        <div className="flex justify-center py-16 text-muted">
          <Spinner className="size-6" />
        </div>
      ) : error ? (
        <Alert>Erro ao carregar transações: {error.message}</Alert>
      ) : txs.length === 0 ? (
        <EmptyState
          title="Nenhuma transação"
          action={
            <Link href="/market" className="text-sm text-cyan-soft underline-offset-4 hover:underline">
              Explorar o mercado
            </Link>
          }
        />
      ) : (
        <>
          <ul className="card divide-y divide-line/70">
            {txs.map((tx) => (
              <TxRow key={tx.id} tx={tx} />
            ))}
          </ul>
          {hasNextPage && (
            <Button variant="ghost" className="self-center" onClick={() => fetchNextPage()} loading={isFetchingNextPage}>
              Carregar mais
            </Button>
          )}
        </>
      )}
    </div>
  );
}

function Summary() {
  const { data: s } = useTxSummary();
  const cards = [
    { label: "Comprado", value: s && formatPrice(s.spentCents), sub: s && `${s.purchases} compra(s)` },
    { label: "Recebido em vendas", value: s && formatPrice(s.earnedCents), sub: s && `${s.sales} venda(s) concluída(s)` },
    { label: "A receber", value: s && formatPrice(s.pendingEarningsCents), sub: "vendas em andamento" },
    { label: "Taxas pagas", value: s && formatPrice(s.feesPaidCents), sub: "2% sobre vendas concluídas" },
  ];
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      {cards.map((c) => (
        <div key={c.label} className="card p-4">
          <p className="text-xs text-faint">{c.label}</p>
          <p className="mt-1 text-lg font-semibold tabular-nums">
            {c.value ?? <span className="inline-block h-6 w-24 animate-pulse rounded bg-ink-700" />}
          </p>
          <p className="text-[11px] text-muted">{c.sub}</p>
        </div>
      ))}
    </div>
  );
}

function TxRow({ tx }: { tx: TransactionDetail }) {
  const buying = tx.role === "buyer";
  const other = buying ? tx.seller : tx.buyer;
  // Comprador vê o que pagou; vendedor vê o que recebe (já sem a taxa).
  const amount = buying ? tx.priceCents : tx.sellerReceivesCents;
  const refunded = tx.status === "cancelled" || tx.status === "failed";

  return (
    <li>
      <Link href={`/transactions/${tx.id}`} className="flex flex-wrap items-center gap-4 p-4 transition hover:bg-ink-800/60">
        <div
          className="flex h-14 w-20 shrink-0 items-center justify-center rounded-lg bg-ink-850"
          style={{ boxShadow: `inset 0 -2px 0 #${tx.item.rarityColor || "6f6290"}` }}
        >
          <Image src={tx.item.imageUrl} alt="" width={80} height={60} className="h-11 w-auto object-contain" />
        </div>

        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{tx.item.name}</p>
          <p className="flex items-center gap-1.5 text-xs text-muted">
            <span className={buying ? "text-cyan-soft" : "text-pink-soft"}>{buying ? "Compra" : "Venda"}</span>
            <span>·</span>
            <span>{buying ? "de" : "para"}</span>
            <Image src={other.avatarUrl} alt="" width={14} height={14} className="rounded-full" />
            <span className="truncate">{other.displayName}</span>
          </p>
          <p className="mt-0.5 text-[11px] text-faint">
            #{tx.id} · {formatDate(tx.createdAt)}
          </p>
        </div>

        <TxStatusBadge status={tx.status} />

        <p
          className={`w-28 text-right font-semibold tabular-nums ${
            refunded ? "text-faint line-through" : buying ? "text-pink" : "text-success"
          }`}
        >
          {buying ? "−" : "+"}
          {formatPrice(amount)}
        </p>
      </Link>
    </li>
  );
}

function Segmented<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { id: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <div className="flex rounded-lg border border-line bg-ink-900 p-1">
      {options.map((o) => (
        <button
          key={o.id}
          onClick={() => onChange(o.id)}
          aria-pressed={value === o.id}
          className={`rounded-md px-3 py-1.5 text-sm transition ${
            value === o.id ? "bg-ink-700 text-text" : "text-muted hover:text-text"
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
