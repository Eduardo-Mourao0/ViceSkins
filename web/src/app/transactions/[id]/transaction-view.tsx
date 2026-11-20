"use client";

import Image from "next/image";
import Link from "next/link";
import { FloatBar } from "@/components/float-bar";
import { RequireLogin } from "@/components/require-login";
import { TxProgress, TxStatusBadge, txStatusInfo } from "@/components/tx-status";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { isApiError } from "@/lib/api";
import { formatDate, formatFloat, formatPrice, timeAgo } from "@/lib/format";
import { useCancelTransaction, useTransaction } from "@/lib/queries";
import type { TransactionDetail } from "@/lib/types";

export function TransactionView({ id }: { id: number }) {
  return <RequireLogin>{() => <Detail id={id} />}</RequireLogin>;
}

function Detail({ id }: { id: number }) {
  const valid = Number.isInteger(id) && id > 0;
  const { data: tx, isPending, error } = useTransaction(valid ? id : null);

  if (!valid || isApiError(error, 404)) {
    return (
      <EmptyState
        title="Transação não encontrada"
        action={
          <Link href="/transactions" className="text-sm text-cyan-soft hover:underline">
            Voltar para Transações
          </Link>
        }
      />
    );
  }
  if (error) return <Alert>{error.message}</Alert>;
  if (isPending) {
    return (
      <div className="flex justify-center py-24 text-muted">
        <Spinner className="size-6" />
      </div>
    );
  }
  return <TxDetail tx={tx} />;
}

function TxDetail({ tx }: { tx: TransactionDetail }) {
  const cancel = useCancelTransaction();
  const buying = tx.role === "buyer";
  const other = buying ? tx.seller : tx.buyer;
  const info = txStatusInfo[tx.status];
  const color = `#${tx.item.rarityColor || "6f6290"}`;
  const canCancel = buying && (tx.status === "pending" || tx.status === "sent");

  // Textos de status do ponto de vista do vendedor (os padrões são do comprador).
  const hint =
    !buying && tx.status === "sent"
      ? "Oferta de troca enviada ao comprador. Aguardando ele aceitar."
      : !buying && tx.status === "on_hold"
        ? "Troca aceita. Você recebe quando a proteção de troca terminar."
        : !buying && (tx.status === "cancelled" || tx.status === "failed")
          ? "A venda não se concretizou. O item continua com você."
          : info.hint;

  return (
    <div className="flex flex-col gap-6">
      <Link href="/transactions" className="w-fit text-sm text-muted hover:text-text">
        ← Transações
      </Link>

      <div className="grid gap-6 lg:grid-cols-[1fr_22rem]">
        <section className="card flex flex-col gap-6 p-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-xs text-faint">
                {buying ? "Compra" : "Venda"} #{tx.id} · {formatDate(tx.createdAt)}
              </p>
              <h1 className="text-xl font-bold">{tx.item.name}</h1>
            </div>
            <TxStatusBadge status={tx.status} />
          </div>

          <div className="relative flex h-48 items-center justify-center overflow-hidden rounded-xl bg-ink-850">
            <div className="absolute h-28 w-2/3 rounded-full opacity-30 blur-3xl" style={{ backgroundColor: color }} />
            <Image
              src={tx.item.imageUrl}
              alt={tx.item.name}
              width={320}
              height={240}
              className="relative h-40 w-auto object-contain drop-shadow-[0_10px_16px_rgba(0,0,0,0.6)]"
            />
            <div className="absolute inset-x-0 bottom-0 h-0.5" style={{ backgroundColor: color }} />
          </div>

          {tx.item.float !== null && (
            <div>
              <div className="mb-2 flex justify-between text-xs text-muted">
                <span>{tx.item.exterior}</span>
                <span className="font-mono text-text tabular-nums">{formatFloat(tx.item.float)}</span>
              </div>
              <FloatBar value={tx.item.float} compact />
            </div>
          )}

          <div className="flex flex-col gap-3">
            <TxProgress status={tx.status} />
            <p className="text-sm text-muted">
              {hint}
              {tx.failureReason && <span className="text-faint"> ({tx.failureReason})</span>}
            </p>
            {tx.status === "on_hold" && tx.holdUntil && (
              <p className="text-xs text-pink-soft">Proteção termina {timeAgo(tx.holdUntil)}.</p>
            )}
          </div>

          {cancel.error && <Alert>{cancel.error.message}</Alert>}
          {canCancel && (
            <Button variant="danger" className="self-start" onClick={() => cancel.mutate(tx.id)} loading={cancel.isPending}>
              Cancelar compra
            </Button>
          )}
        </section>

        <aside className="card flex h-fit flex-col gap-5 p-6">
          <div className="flex items-center gap-3">
            <Image src={other.avatarUrl} alt="" width={36} height={36} className="rounded-full ring-1 ring-line" />
            <div>
              <p className="text-xs text-faint">{buying ? "Vendedor" : "Comprador"}</p>
              <p className="text-sm">{other.displayName}</p>
            </div>
          </div>

          <dl className="grid grid-cols-[1fr_auto] gap-y-2 text-sm">
            <dt className="text-muted">Preço</dt>
            <dd className="text-right tabular-nums">{formatPrice(tx.priceCents)}</dd>
            {!buying && (
              <>
                <dt className="text-muted">Taxa da plataforma</dt>
                <dd className="text-right tabular-nums">− {formatPrice(tx.feeCents)}</dd>
              </>
            )}
            <dt className="border-t border-line pt-2 font-semibold">{buying ? "Você pagou" : "Você recebe"}</dt>
            <dd
              className={`border-t border-line pt-2 text-right font-semibold tabular-nums ${
                buying ? "text-pink" : "text-success"
              }`}
            >
              {formatPrice(buying ? tx.priceCents : tx.sellerReceivesCents)}
            </dd>
          </dl>

          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-xs">
            {tx.tradeOfferId && (
              <>
                <dt className="text-faint">Oferta</dt>
                <dd className="truncate text-right font-mono text-muted">{tx.tradeOfferId}</dd>
              </>
            )}
            <dt className="text-faint">Atualizada</dt>
            <dd className="text-right text-muted">{formatDate(tx.updatedAt)}</dd>
          </dl>

          <Link href="/wallet" className="text-xs text-cyan-soft hover:underline">
            Ver movimentações na carteira →
          </Link>
        </aside>
      </div>
    </div>
  );
}
