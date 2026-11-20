"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { isApiError } from "@/lib/api";
import { formatDate, formatFloat, formatPrice } from "@/lib/format";
import { useBuy, useCancelTransaction, useMe, useTransaction } from "@/lib/queries";
import type { MarketListing } from "@/lib/types";
import { FloatBar } from "./float-bar";
import { TradeURLNotice } from "./trade-url-notice";
import { TxProgress, TxStatusBadge, txStatusInfo } from "./tx-status";
import { Alert, Button, SteamLoginButton } from "./ui";

/** Detalhe de um anúncio do mercado, com o fluxo de compra. */
export function ListingDialog({ listing, onClose }: { listing: MarketListing; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    ref.current?.showModal();
  }, []);

  const { data: me } = useMe();
  const isMine = me?.id === listing.seller.id;
  const { item, seller } = listing;
  const color = `#${item.rarityColor || "6f6290"}`;

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => e.target === ref.current && onClose()}
      className="m-auto w-[min(92vw,34rem)] rounded-2xl border border-line bg-ink-900 p-0 text-text shadow-2xl backdrop:bg-ink-950/80 backdrop:backdrop-blur-sm"
    >
      <div className="relative flex h-56 items-center justify-center overflow-hidden bg-ink-850">
        <div className="absolute h-32 w-2/3 rounded-full opacity-30 blur-3xl" style={{ backgroundColor: color }} />
        <Image
          src={item.imageUrl}
          alt={item.name}
          width={360}
          height={270}
          className="relative h-44 w-auto object-contain drop-shadow-[0_10px_16px_rgba(0,0,0,0.6)]"
        />
        <div className="absolute inset-x-0 bottom-0 h-0.5" style={{ backgroundColor: color }} />
      </div>

      <div className="flex flex-col gap-5 p-6">
        <div>
          <p className="text-xs" style={{ color }}>
            {item.rarity}
            {item.exterior && <span className="text-muted"> · {item.exterior}</span>}
          </p>
          <h2 className="text-lg font-semibold leading-snug">{item.name}</h2>
        </div>

        {item.float !== null && (
          <div className="rounded-lg bg-ink-850 p-3">
            <div className="mb-2 flex justify-between text-xs text-muted">
              <span>Float</span>
              <span className="font-mono text-text tabular-nums">{formatFloat(item.float)}</span>
            </div>
            <FloatBar value={item.float} compact />
            {item.paintSeed !== null && (
              <p className="mt-2 text-xs text-muted">
                Pattern <span className="font-mono text-text">{item.paintSeed}</span>
              </p>
            )}
          </div>
        )}

        {item.stickers.length > 0 && (
          <ul className="flex flex-wrap gap-2">
            {item.stickers.map((s, i) => (
              <li key={i} className="flex items-center gap-1.5 rounded-md bg-ink-850 py-1 pr-2 pl-1 text-xs">
                <Image src={s.imageUrl} alt="" width={28} height={21} className="h-5 w-auto" />
                <span className="max-w-44 truncate">{s.name}</span>
              </li>
            ))}
          </ul>
        )}

        <div className="flex items-center gap-3 text-sm">
          <Image src={seller.avatarUrl} alt="" width={32} height={32} className="rounded-full ring-1 ring-line" />
          <div>
            <p className="text-xs text-faint">Vendido por</p>
            <p>{isMine ? "Você" : seller.displayName}</p>
          </div>
          <p className="ml-auto text-xs text-faint">Anunciado em {formatDate(listing.createdAt)}</p>
        </div>

        <div className="border-t border-line pt-5">
          {!me ? (
            <div className="flex items-center justify-between gap-4">
              <Price cents={listing.priceCents} />
              <SteamLoginButton />
            </div>
          ) : isMine ? (
            <div className="flex items-center justify-between gap-4">
              <Price cents={listing.priceCents} />
              <Button disabled>Seu anúncio</Button>
            </div>
          ) : (
            <BuyFlow listing={listing} balanceCents={me.balanceCents} hasTradeUrl={!!me.tradeUrl} onClose={onClose} />
          )}
        </div>
      </div>
    </dialog>
  );
}

function Price({ cents }: { cents: number }) {
  return <p className="text-2xl font-bold text-neon-pink tabular-nums">{formatPrice(cents)}</p>;
}

/** Comprar → confirmar → acompanhar a transação ao vivo. */
function BuyFlow({
  listing,
  balanceCents,
  hasTradeUrl,
  onClose,
}: {
  listing: MarketListing;
  balanceCents: number;
  hasTradeUrl: boolean;
  onClose: () => void;
}) {
  const [confirming, setConfirming] = useState(false);
  const buy = useBuy();
  const cancel = useCancelTransaction();
  const { data: tx } = useTransaction(buy.data?.id ?? null);

  // Depois da compra: linha do tempo da transação.
  if (tx) {
    const info = txStatusInfo[tx.status];
    const canCancel = tx.status === "pending" || tx.status === "sent";
    return (
      <div className="flex flex-col gap-4">
        <div className="flex items-center justify-between">
          <p className="font-semibold">Compra #{tx.id}</p>
          <TxStatusBadge status={tx.status} />
        </div>
        <TxProgress status={tx.status} />
        <p className="text-sm text-muted">{tx.failureReason ? `${info.hint} (${tx.failureReason})` : info.hint}</p>
        {cancel.error && <Alert>{cancel.error.message}</Alert>}
        <div className="flex items-center justify-end gap-2">
          <Link href={`/transactions/${tx.id}`} className="mr-auto text-sm text-cyan-soft hover:underline">
            Ver detalhes
          </Link>
          {canCancel && (
            <Button variant="danger" onClick={() => cancel.mutate(tx.id)} loading={cancel.isPending}>
              Cancelar compra
            </Button>
          )}
          <Button variant="ghost" onClick={onClose}>
            Fechar
          </Button>
        </div>
      </div>
    );
  }

  const insufficient = balanceCents < listing.priceCents;
  const after = balanceCents - listing.priceCents;
  const errorMessage =
    buy.error &&
    (isApiError(buy.error, 409)
      ? "Este item acabou de ser vendido para outra pessoa."
      : isApiError(buy.error, 422) && buy.error.message === "insufficient balance"
        ? "Saldo insuficiente."
        : buy.error.message);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <Price cents={listing.priceCents} />
        {!confirming && (
          <Button onClick={() => setConfirming(true)} disabled={insufficient || !hasTradeUrl}>
            Comprar
          </Button>
        )}
      </div>

      {!hasTradeUrl && <TradeURLNotice action="comprar" />}

      {hasTradeUrl && insufficient && (
        <Alert tone="warning">
          Seu saldo é {formatPrice(balanceCents)}.{" "}
          <Link href="/wallet" className="underline underline-offset-2">
            Depositar
          </Link>
        </Alert>
      )}

      {confirming && (
        <div className="flex flex-col gap-3 rounded-lg border border-pink/40 bg-pink/5 p-4">
          <dl className="grid grid-cols-2 gap-y-1 text-sm">
            <dt className="text-muted">Saldo atual</dt>
            <dd className="text-right tabular-nums">{formatPrice(balanceCents)}</dd>
            <dt className="text-muted">Preço</dt>
            <dd className="text-right tabular-nums">− {formatPrice(listing.priceCents)}</dd>
            <dt className="font-semibold">Saldo após a compra</dt>
            <dd className="text-right font-semibold tabular-nums">{formatPrice(after)}</dd>
          </dl>
          {errorMessage && <Alert>{errorMessage}</Alert>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setConfirming(false)} disabled={buy.isPending}>
              Voltar
            </Button>
            <Button onClick={() => buy.mutate(listing.id)} loading={buy.isPending} disabled={buy.isError}>
              Confirmar compra
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
