"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useId, useRef, useState } from "react";
import type { InventoryItem } from "@/lib/types";
import { formatFloat, formatPrice, parsePriceToCents } from "@/lib/format";
import { useCancelListing, useCreateListing, useMe } from "@/lib/queries";
import { Alert, Button } from "./ui";
import { FloatBar } from "./float-bar";
import { TradeURLNotice } from "./trade-url-notice";

const MIN_CENTS = 10;
const MAX_CENTS = 100_000_000;

/** Modal do item: anuncia (se livre) ou cancela o anúncio (se já à venda). */
export function ItemDialog({ item, onClose }: { item: InventoryItem; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const priceId = useId();
  const [price, setPrice] = useState("");
  const create = useCreateListing();
  const cancel = useCancelListing();
  const { data: me } = useMe();
  const missingTradeUrl = !me?.tradeUrl;

  // <dialog> nativo: trava o foco dentro dele e fecha com Esc.
  useEffect(() => {
    ref.current?.showModal();
  }, []);

  const cents = parsePriceToCents(price);
  const priceError =
    price === ""
      ? null
      : cents === null
        ? "Use um valor como 15,50"
        : cents < MIN_CENTS
          ? `Mínimo de ${formatPrice(MIN_CENTS)}`
          : cents > MAX_CENTS
            ? `Máximo de ${formatPrice(MAX_CENTS)}`
            : null;

  const listing = item.listing;
  const mutation = listing ? cancel : create;

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (listing) {
      cancel.mutate(listing.id, { onSuccess: onClose });
    } else if (cents !== null && !priceError) {
      create.mutate({ itemId: item.id, priceCents: cents }, { onSuccess: onClose });
    }
  }

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => e.target === ref.current && onClose()} // clique fora fecha
      className="m-auto w-[min(92vw,28rem)] rounded-2xl border border-line bg-ink-900 p-0 text-text shadow-2xl backdrop:bg-ink-950/80 backdrop:backdrop-blur-sm"
    >
      <form onSubmit={submit} className="flex flex-col gap-5 p-6">
        <div className="flex items-start gap-4">
          <div
            className="flex size-24 shrink-0 items-center justify-center rounded-xl bg-ink-800"
            style={{ boxShadow: `inset 0 -2px 0 #${item.rarityColor || "6f6290"}` }}
          >
            <Image src={item.imageUrl} alt="" width={96} height={72} className="h-16 w-auto object-contain" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-xs" style={{ color: `#${item.rarityColor}` }}>
              {item.rarity}
            </p>
            <h2 className="font-semibold leading-snug">{item.name}</h2>
            <p className="text-xs text-muted">{item.exterior || item.type}</p>
          </div>
        </div>

        {item.float !== null && (
          <div className="rounded-lg bg-ink-850 p-3">
            <div className="mb-2 flex justify-between text-xs text-muted">
              <span>Float</span>
              <span className="font-mono tabular-nums text-text">{formatFloat(item.float)}</span>
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
                <span className="max-w-40 truncate">{s.name}</span>
              </li>
            ))}
          </ul>
        )}

        {listing ? (
          <div className="rounded-lg border border-pink/40 bg-pink/5 p-4 text-sm">
            Anunciado por{" "}
            <span className="font-semibold text-pink tabular-nums">{formatPrice(listing.priceCents)}</span>
          </div>
        ) : (
          <div>
            <label htmlFor={priceId} className="mb-1.5 block text-sm text-muted">
              Preço de venda
            </label>
            <div className="flex items-center rounded-lg border border-line bg-ink-850 focus-within:border-cyan">
              <span className="pl-3 text-sm text-faint">R$</span>
              <input
                id={priceId}
                inputMode="decimal"
                autoComplete="off"
                autoFocus
                placeholder="0,00"
                value={price}
                onChange={(e) => setPrice(e.target.value)}
                className="w-full bg-transparent px-2 py-2.5 text-lg tabular-nums outline-none"
              />
            </div>
            {priceError && <p className="mt-1.5 text-xs text-danger">{priceError}</p>}
          </div>
        )}

        {!listing && missingTradeUrl && <TradeURLNotice action="anunciar" />}
        {mutation.error && (
          <Alert>
            {mutation.error.message.includes("without float") ? (
              <>
                Itens sem float (caixas, stickers...) só podem ser anunciados com a sua API key da Steam cadastrada.{" "}
                <Link href="/profile" className="font-semibold underline underline-offset-2">
                  Cadastrar
                </Link>
              </>
            ) : (
              mutation.error.message
            )}
          </Alert>
        )}

        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Fechar
          </Button>
          {listing ? (
            <Button type="submit" variant="danger" loading={cancel.isPending}>
              Cancelar anúncio
            </Button>
          ) : (
            <Button
              type="submit"
              loading={create.isPending}
              disabled={cents === null || !!priceError || missingTradeUrl}
            >
              Anunciar
            </Button>
          )}
        </div>
      </form>
    </dialog>
  );
}
