"use client";

import Image from "next/image";
import { useEffect, useRef } from "react";
import { formatDate, formatFloat, formatPrice } from "@/lib/format";
import type { MarketListing } from "@/lib/types";
import { FloatBar } from "./float-bar";
import { Button } from "./ui";

/** Detalhe de um anúncio do mercado. A compra entra na etapa 6. */
export function ListingDialog({
  listing,
  isMine,
  onClose,
}: {
  listing: MarketListing;
  isMine: boolean;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    ref.current?.showModal();
  }, []);

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

        <div className="flex items-center justify-between gap-4 border-t border-line pt-5">
          <p className="text-2xl font-bold text-neon-pink tabular-nums">{formatPrice(listing.priceCents)}</p>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onClose}>
              Fechar
            </Button>
            <Button disabled title={isMine ? "Este anúncio é seu" : "Compra em breve"}>
              {isMine ? "Seu anúncio" : "Comprar"}
            </Button>
          </div>
        </div>
      </div>
    </dialog>
  );
}
