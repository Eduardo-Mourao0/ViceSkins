"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";
import { FloatBar } from "@/components/float-bar";
import { RequireLogin } from "@/components/require-login";
import { TxProgress, TxStatusBadge, txStatusInfo } from "@/components/tx-status";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { isApiError } from "@/lib/api";
import { formatDate, formatFloat, formatPrice, timeAgo } from "@/lib/format";
import { useCancelTransaction, useMarkSent, useTransaction } from "@/lib/queries";
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
  const markSent = useMarkSent();
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
              <p className="text-xs text-pink-soft">
                Proteção termina {timeAgo(tx.holdUntil)} ({formatDate(tx.holdUntil)}).
                {!buying && " O valor fica bloqueado até lá."}
              </p>
            )}
            {tx.deadlineAt && <Deadline tx={tx} buying={buying} deadlineAt={tx.deadlineAt} />}
          </div>

          {/* P2P: o vendedor envia a oferta para a trade URL do comprador. */}
          {tx.buyerTradeUrl && (
            <div className="rounded-lg border border-cyan/40 bg-cyan/5 p-4 text-sm">
              <p className="font-semibold">Envie o item para o comprador</p>
              <p className="mt-1 text-muted">
                Crie a oferta de troca na Steam por esta trade URL, só com este item e sem pedir nada em troca.
              </p>
              <a
                href={tx.buyerTradeUrl}
                target="_blank"
                rel="noreferrer"
                className="mt-2 block truncate font-mono text-xs text-cyan-soft hover:underline"
              >
                {tx.buyerTradeUrl}
              </a>
              <p className="mt-3 text-xs text-muted">
                Depois de enviar, avise aqui: o prazo passa a ser do comprador. A entrega é confirmada pelo inventário dele
                (ou pela sua API key, se cadastrada).
              </p>
              {markSent.error && (
                <div className="mt-2">
                  <Alert>{markSent.error.message}</Alert>
                </div>
              )}
              <Button className="mt-3" onClick={() => markSent.mutate(tx.id)} loading={markSent.isPending}>
                Já enviei a oferta
              </Button>
            </div>
          )}

          {tx.lateDeliveryAt && (
            <Alert tone="warning">
              Entrega tardia: o item chegou ao comprador depois de a compra ter sido estornada (
              {formatDate(tx.lateDeliveryAt)}).{" "}
              {buying
                ? "O valor foi cobrado de novo do seu saldo (o que faltou ficou como dívida)."
                : "Você recebe o valor da venda no fim da proteção de troca, garantido pela plataforma."}
            </Alert>
          )}

          {cancel.error && <Alert>{cancelError(cancel.error)}</Alert>}
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

/** Prazo da etapa atual: perdeu, a compra é cancelada e o comprador estornado. */
function Deadline({ tx, buying, deadlineAt }: { tx: TransactionDetail; buying: boolean; deadlineAt: string }) {
  const sellerTurn = tx.status === "pending";
  const myTurn = sellerTurn !== buying; // pending: vez do vendedor; sent: vez do comprador
  const who = myTurn ? "Você tem" : sellerTurn ? "O vendedor tem" : "O comprador tem";
  const what = sellerTurn ? "enviar a oferta de troca" : "aceitar a oferta de troca";
  const late = new Date(deadlineAt).getTime() <= useNow();

  return (
    <p className={`text-xs ${myTurn ? "text-warning" : "text-faint"}`}>
      {late
        ? "O prazo terminou. A compra será cancelada e o valor devolvido ao comprador."
        : `${who} até ${formatDate(deadlineAt)} (${timeAgo(deadlineAt)}) para ${what}. Depois disso, a compra é cancelada e o valor volta para o comprador.`}
    </p>
  );
}

/** Hora atual em estado, atualizada a cada 30s: o render continua puro. */
function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(t);
  }, [intervalMs]);
  return now;
}

// Mensagens da API (inglês) → texto para o usuário.
function cancelError(err: Error): string {
  if (isApiError(err, 409) && err.message.includes("already sent")) {
    return "O vendedor já enviou a oferta de troca. Recuse-a na Steam: a compra é encerrada no fim do prazo e o valor volta para você.";
  }
  if (isApiError(err, 503)) {
    return "Não foi possível confirmar na Steam que o item ainda está com o vendedor. Tente de novo em alguns minutos.";
  }
  if (isApiError(err, 409) && err.message.includes("already delivered")) {
    return "O item já foi entregue: a compra não pode mais ser cancelada.";
  }
  return err.message;
}
