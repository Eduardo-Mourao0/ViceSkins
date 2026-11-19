"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useLogout, useMe } from "@/lib/queries";
import { formatPrice } from "@/lib/format";
import { Button, SteamLoginButton } from "./ui";

const nav = [
  { href: "/market", label: "Mercado", public: true },
  { href: "/inventory", label: "Inventário", public: false },
  { href: "/my-listings", label: "Meus anúncios", public: false },
];

export function Logo() {
  return (
    <Link href="/" className="flex items-baseline gap-0.5 select-none" aria-label="ViceSkins, início">
      <span className="font-script text-3xl leading-none text-neon-pink">Vice</span>
      <span className="text-lg font-bold tracking-[0.2em] text-neon-cyan">SKINS</span>
    </Link>
  );
}

export function Header() {
  const pathname = usePathname();
  const { data: me, isPending } = useMe();
  const logout = useLogout();

  return (
    <header className="sticky top-0 z-30 border-b border-line/70 bg-ink-950/80 backdrop-blur">
      {/* Linha neon no topo, do rosa ao ciano. */}
      <div className="h-px bg-gradient-to-r from-pink via-pink/40 to-cyan" />
      <div className="mx-auto flex h-16 max-w-7xl items-center gap-6 px-4 sm:px-6">
        <Logo />

        <nav className="hidden gap-1 sm:flex">
          {nav.filter((n) => n.public || me).map((n) => {
            const active = pathname.startsWith(n.href);
            return (
              <Link
                key={n.href}
                href={n.href}
                className={`rounded-md px-3 py-1.5 text-sm transition ${
                  active ? "bg-ink-700 text-text" : "text-muted hover:text-text"
                }`}
              >
                {n.label}
              </Link>
            );
          })}
        </nav>

        <div className="ml-auto flex items-center gap-3">
          {isPending ? (
            <div className="h-9 w-40 animate-pulse rounded-lg bg-ink-800" />
          ) : me ? (
            <>
              <div className="hidden text-right sm:block">
                <p className="text-[11px] uppercase tracking-wider text-faint">Saldo</p>
                <p className="text-sm font-semibold text-cyan-soft tabular-nums">
                  {formatPrice(me.balanceCents)}
                </p>
              </div>
              <div className="flex items-center gap-2 rounded-full border border-line bg-ink-850 py-1 pr-3 pl-1">
                <Image
                  src={me.avatarUrl}
                  alt=""
                  width={28}
                  height={28}
                  className="rounded-full ring-1 ring-pink/60"
                />
                <span className="max-w-32 truncate text-sm">{me.displayName}</span>
              </div>
              <Button variant="ghost" onClick={() => logout.mutate()} loading={logout.isPending}>
                Sair
              </Button>
            </>
          ) : (
            <SteamLoginButton />
          )}
        </div>
      </div>

      <nav className="flex gap-1 border-t border-line/60 px-4 py-2 sm:hidden">
        {nav.filter((n) => n.public || me).map((n) => (
          <Link
            key={n.href}
            href={n.href}
            className={`rounded-md px-3 py-1.5 text-sm ${
              pathname.startsWith(n.href) ? "bg-ink-700 text-text" : "text-muted"
            }`}
          >
            {n.label}
          </Link>
        ))}
      </nav>
    </header>
  );
}
