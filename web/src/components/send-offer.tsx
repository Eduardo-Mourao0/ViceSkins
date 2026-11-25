"use client";

import Image from "next/image";
import { useInventory, useMarkSent } from "@/lib/queries";
import type { TransactionDetail } from "@/lib/types";
import { Alert, Button, SteamIcon } from "./ui";

/**
 * O que o vendedor faz para entregar: abrir a troca com o comprador na Steam
 * e adicionar exatamente este item. A Steam não deixa pré-selecionar o item
 * pela URL, então o painel mostra como reconhecê-lo (float e pattern), que é
 * o que importa quando o vendedor tem mais de uma cópia da mesma skin: sem API
 * key, a entrega é confirmada por essa impressão digital, e a cópia errada
 * não conta como entrega.
 */
export function SendOfferPanel({ tx, tradeUrl }: { tx: TransactionDetail; tradeUrl: string }) {
  const markSent = useMarkSent();
  const { data: inv } = useInventory(true);
  const { item } = tx;
  const copies = inv?.items.filter((i) => i.marketHashName === item.marketHashName).length ?? 0;

  return (
    <div className="flex flex-col gap-4 rounded-xl border border-cyan/40 bg-cyan/5 p-4 text-sm">
      <div>
        <p className="font-semibold">Envie o item para o comprador</p>
        <p className="mt-0.5 text-muted">A troca é feita na Steam. Leva um minuto.</p>
      </div>

      <a
        href={tradeUrl}
        target="_blank"
        rel="noreferrer"
        className="inline-flex items-center justify-center gap-2 rounded-lg bg-cyan px-4 py-2.5 font-semibold text-ink-950 shadow-neon-cyan transition hover:bg-cyan-soft"
      >
        <SteamIcon />
        Abrir troca na Steam
        <span aria-hidden>↗</span>
      </a>

      <div className="rounded-lg border border-line bg-ink-900/60 p-3">
        <p className="mb-2 text-xs tracking-wider text-faint uppercase">Adicione este item</p>
        <div className="flex items-center gap-3">
          <Image src={item.imageUrl} alt="" width={72} height={54} className="h-12 w-auto shrink-0 object-contain" />
          <div className="min-w-0">
            <p className="truncate font-medium">{item.name}</p>
            <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 text-xs">
              {item.float !== null && (
                <>
                  <dt className="text-muted">Float</dt>
                  <dd className="font-mono text-text tabular-nums">{item.float}</dd>
                </>
              )}
              {item.paintSeed !== null && (
                <>
                  <dt className="text-muted">Pattern</dt>
                  <dd className="font-mono text-text">{item.paintSeed}</dd>
                </>
              )}
            </dl>
          </div>
        </div>
        {copies > 1 && (
          <div className="mt-3">
            <Alert tone="warning">
              Você tem {copies} cópias deste item. Na Steam, passe o mouse sobre cada uma e confira o float e o pattern:
              só esta cópia conta como entrega.
            </Alert>
          </div>
        )}
      </div>

      <ol className="flex flex-col gap-1.5 text-muted">
        {[
          "Abra a troca e adicione só este item, do seu inventário.",
          "Não peça nada em troca: o pagamento é feito pelo site.",
          "Envie a oferta e confirme no app da Steam, no celular.",
          "Volte aqui e clique em “Já enviei a oferta”.",
        ].map((step, i) => (
          <li key={i} className="flex gap-2.5">
            <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-ink-700 text-[11px] font-semibold text-cyan-soft">
              {i + 1}
            </span>
            {step}
          </li>
        ))}
      </ol>

      <p className="text-xs text-faint">
        Depois de avisar, o prazo passa a ser do comprador. A entrega é confirmada pelo inventário dele (ou pela sua API
        key, se cadastrada).
      </p>
      {markSent.error && <Alert>{markSent.error.message}</Alert>}
      <Button variant="secondary" onClick={() => markSent.mutate(tx.id)} loading={markSent.isPending}>
        Já enviei a oferta
      </Button>
    </div>
  );
}
