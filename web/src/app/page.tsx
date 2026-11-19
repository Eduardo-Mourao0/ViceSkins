"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ItemCard } from "@/components/item-card";
import { SteamLoginButton } from "@/components/ui";
import { useMarket, useMe } from "@/lib/queries";

const features = [
  {
    title: "Login pela Steam",
    text: "Você entra pela própria Steam. Nunca vemos sua senha.",
  },
  {
    title: "Inventário com float",
    text: "Float, pattern e stickers de cada skin, direto do seu inventário.",
  },
  {
    title: "Compra segura",
    text: "Cada compra é atômica: o mesmo item nunca é vendido duas vezes.",
  },
];

const ctaPrimary =
  "rounded-lg bg-pink px-5 py-2.5 text-sm font-semibold text-ink-950 shadow-[0_0_24px_-6px_var(--color-pink)] transition hover:bg-pink-soft";
const ctaSecondary =
  "rounded-lg border border-cyan/50 px-5 py-2.5 text-sm text-cyan-soft transition hover:bg-cyan/10";

export default function Home() {
  const router = useRouter();
  const { data: me } = useMe();
  const { data: latest } = useMarket("pageSize=8");

  return (
    <div className="flex flex-col gap-16 py-8 sm:py-12">
      <section className="flex flex-col items-start gap-6">
        <p className="rounded-full border border-cyan/40 bg-cyan/5 px-3 py-1 text-xs tracking-wider text-cyan-soft uppercase">
          Marketplace de skins de CS2
        </p>
        <h1 className="max-w-3xl text-4xl leading-tight font-bold sm:text-6xl">
          Compre e venda skins{" "}
          <span className="font-script text-5xl font-normal text-neon-pink sm:text-7xl">sob neon</span>
        </h1>
        <p className="max-w-xl text-muted">
          Importe seu inventário, anuncie pelo preço que quiser e compre de outros jogadores com saldo na
          plataforma.
        </p>
        <div className="flex flex-wrap gap-3">
          <Link href="/market" className={ctaPrimary}>
            Explorar o mercado
          </Link>
          {me ? (
            <Link href="/inventory" className={ctaSecondary}>
              Vender do meu inventário
            </Link>
          ) : (
            <SteamLoginButton variant="outline" className="px-5 py-2.5" />
          )}
        </div>
      </section>

      {latest && latest.listings.length > 0 && (
        <section className="flex flex-col gap-4">
          <div className="flex items-end justify-between">
            <h2 className="text-lg font-semibold">Anunciados recentemente</h2>
            <Link href="/market" className="text-sm text-cyan-soft hover:underline">
              Ver todos →
            </Link>
          </div>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {latest.listings.map((l) => (
              <ItemCard key={l.id} item={l.item} priceCents={l.priceCents} onClick={() => router.push("/market")} />
            ))}
          </div>
        </section>
      )}

      <section className="grid gap-4 sm:grid-cols-3">
        {features.map((f, i) => (
          <div key={f.title} className="card p-5">
            <div className={`mb-3 h-0.5 w-10 rounded-full ${i % 2 === 0 ? "bg-pink" : "bg-cyan"}`} />
            <h2 className="mb-1 font-semibold">{f.title}</h2>
            <p className="text-sm text-muted">{f.text}</p>
          </div>
        ))}
      </section>
    </div>
  );
}
