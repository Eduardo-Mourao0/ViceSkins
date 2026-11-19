"use client";

import Link from "next/link";
import { useMe } from "@/lib/queries";
import { SteamLoginButton } from "@/components/ui";

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

export default function Home() {
  const { data: me } = useMe();

  return (
    <div className="flex flex-col gap-16 py-8 sm:py-16">
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
          {me ? (
            <>
              <Link
                href="/inventory"
                className="rounded-lg bg-pink px-5 py-2.5 text-sm font-semibold text-ink-950 shadow-[0_0_24px_-6px_var(--color-pink)] transition hover:bg-pink-soft"
              >
                Ver meu inventário
              </Link>
              <Link
                href="/my-listings"
                className="rounded-lg border border-cyan/50 px-5 py-2.5 text-sm text-cyan-soft transition hover:bg-cyan/10"
              >
                Meus anúncios
              </Link>
            </>
          ) : (
            <SteamLoginButton className="px-5 py-2.5" />
          )}
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-3">
        {features.map((f, i) => (
          <div key={f.title} className="card p-5">
            <div
              className={`mb-3 h-0.5 w-10 rounded-full ${i % 2 === 0 ? "bg-pink" : "bg-cyan"}`}
            />
            <h2 className="mb-1 font-semibold">{f.title}</h2>
            <p className="text-sm text-muted">{f.text}</p>
          </div>
        ))}
      </section>
    </div>
  );
}
