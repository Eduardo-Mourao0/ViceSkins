import Image from "next/image";
import type { ReactNode } from "react";
import type { InventoryItem } from "@/lib/types";
import { formatPrice, wearShort } from "@/lib/format";
import { FloatBar } from "./float-bar";

/** Separa "AK-47 | Redline" em arma e nome da skin. */
export function splitName(name: string): { weapon: string; skin: string } {
  const [weapon, ...rest] = name.split(" | ");
  return rest.length ? { weapon, skin: rest.join(" | ") } : { weapon: "", skin: name };
}

type Props = {
  item: InventoryItem;
  onClick?: () => void;
  /** Preço exibido no rodapé (anúncio). */
  priceCents?: number;
  /** Selo no canto inferior direito (ex.: "À venda", vendedor). */
  badge?: ReactNode;
  /** Texto no lugar do preço quando não há preço (ex.: "Anunciar →"). */
  hint?: ReactNode;
  highlighted?: boolean;
  dimmed?: boolean;
};

export function ItemCard({ item, onClick, priceCents, badge, hint, highlighted, dimmed }: Props) {
  const { weapon, skin } = splitName(item.name);
  const color = item.rarityColor ? `#${item.rarityColor}` : "#6f6290";
  const wear = wearShort(item.exterior);
  const stattrak = item.marketHashName.startsWith("StatTrak™");

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!onClick}
      style={{ "--rarity": color } as React.CSSProperties}
      className={`group card relative flex flex-col overflow-hidden text-left transition enabled:hover:-translate-y-0.5 enabled:hover:border-[var(--rarity)] disabled:cursor-default ${
        highlighted ? "border-pink/70 shadow-neon-pink" : ""
      } ${dimmed ? "opacity-60" : ""}`}
    >
      {/* Faixa de raridade no topo */}
      <div className="h-0.5 w-full" style={{ backgroundColor: color }} />

      <div className="relative flex aspect-[4/3] items-center justify-center px-4 pt-3">
        {/* Brilho da cor da raridade atrás da arma */}
        <div
          className="absolute inset-x-6 top-1/2 h-1/2 -translate-y-1/2 rounded-full opacity-25 blur-2xl transition group-enabled:group-hover:opacity-45"
          style={{ backgroundColor: color }}
        />
        <Image
          src={item.imageUrl}
          alt={item.name}
          width={256}
          height={192}
          className="relative h-full w-auto object-contain drop-shadow-[0_6px_10px_rgba(0,0,0,0.6)]"
        />

        <div className="absolute top-2 left-2 flex gap-1">
          {wear && (
            <span className="rounded bg-ink-950/80 px-1.5 py-0.5 text-[10px] font-semibold text-muted">
              {wear}
            </span>
          )}
          {stattrak && (
            <span className="rounded bg-[#cf6a32]/20 px-1.5 py-0.5 text-[10px] font-semibold text-[#ff9a5c]">
              ST
            </span>
          )}
        </div>

        {item.stickers.length > 0 && (
          <div className="absolute right-2 bottom-1 flex -space-x-1.5">
            {item.stickers.slice(0, 5).map((s, i) => (
              <Image
                key={i}
                src={s.imageUrl}
                alt={s.name}
                title={s.name}
                width={22}
                height={16}
                className="h-4 w-auto"
              />
            ))}
          </div>
        )}
      </div>

      <div className="flex flex-1 flex-col gap-2 p-3 pt-2">
        <div className="min-w-0">
          {weapon && <p className="truncate text-[11px] text-muted">{weapon}</p>}
          <p className="truncate text-sm font-medium">{skin}</p>
        </div>

        {item.float !== null && <FloatBar value={item.float} />}

        <div className="mt-auto flex min-h-6 items-center justify-between gap-2 pt-1">
          {priceCents !== undefined ? (
            <span className="text-sm font-semibold text-pink tabular-nums">{formatPrice(priceCents)}</span>
          ) : (
            <span className="text-[11px]">{hint}</span>
          )}
          {badge}
        </div>
      </div>
    </button>
  );
}

export function Badge({ children, tone = "pink" }: { children: ReactNode; tone?: "pink" | "cyan" }) {
  const tones = { pink: "bg-pink/15 text-pink", cyan: "bg-cyan/15 text-cyan-soft" };
  return (
    <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold tracking-wide uppercase ${tones[tone]}`}>
      {children}
    </span>
  );
}
