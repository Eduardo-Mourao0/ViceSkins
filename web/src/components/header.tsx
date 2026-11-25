"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Suspense } from "react";
import { useLogout, useMe } from "@/lib/queries";
import { formatPrice } from "@/lib/format";
import { Button, SteamLoginButton } from "./ui";

const nav = [
  { href: "/market", label: "Mercado", public: true },
  { href: "/inventory", label: "Inventário", public: false },
  { href: "/my-listings", label: "Meus anúncios", public: false },
  { href: "/transactions", label: "Transações", public: false },
  { href: "/wallet", label: "Carteira", public: false },
];

export function Logo() {
  return (
    <Link href="/" className="flex items-baseline gap-0.5 select-none" aria-label="ViceSkins, início">
      <span className="font-script text-3xl leading-none text-neon-pink">Vice</span>
      <span className="text-lg font-bold tracking-[0.2em] text-neon-cyan">SKINS</span>
    </Link>
  );
}

type NavItem = (typeof nav)[number];
type NavVariant = "desktop" | "mobile";

function NavList({ items, pathname, variant }: { items: NavItem[]; pathname: string; variant: NavVariant }) {
  return items.map((n) => {
    const active = pathname.startsWith(n.href);
    const look = active ? "bg-ink-700 text-text" : variant === "desktop" ? "text-muted hover:text-text" : "text-muted";
    return (
      <Link key={n.href} href={n.href} className={`rounded-md px-3 py-1.5 text-sm transition ${look}`}>
        {n.label}
      </Link>
    );
  });
}

// usePathname só é conhecido na requisição em rotas dinâmicas (/transactions/[id]).
// Isolado aqui dentro de um Suspense, o resto do header continua pré-renderizado;
// o fallback mostra os mesmos links, só sem o destaque do item ativo.
function ActiveNavList(props: { items: NavItem[]; variant: NavVariant }) {
  return <NavList {...props} pathname={usePathname()} />;
}

function Nav({ items, variant }: { items: NavItem[]; variant: NavVariant }) {
  return (
    <Suspense fallback={<NavList items={items} variant={variant} pathname="" />}>
      <ActiveNavList items={items} variant={variant} />
    </Suspense>
  );
}

export function Header() {
  const { data: me, isPending } = useMe();
  const items = [
    ...nav.filter((n) => n.public || me),
    ...(me?.role === "admin" ? [{ href: "/admin/disputes", label: "Disputas", public: false }] : []),
  ];
  const logout = useLogout();

  return (
    <header className="sticky top-0 z-30 border-b border-line/70 bg-ink-950/80 backdrop-blur">
      {/* Linha neon no topo, do rosa ao ciano. */}
      <div className="h-px bg-gradient-to-r from-pink via-pink/40 to-cyan" />
      <div className="mx-auto flex h-16 max-w-7xl items-center gap-6 px-4 sm:px-6">
        <Logo />

        <nav className="hidden gap-1 sm:flex">
          <Nav items={items} variant="desktop" />
        </nav>

        <div className="ml-auto flex items-center gap-3">
          {isPending ? (
            <div className="h-9 w-40 animate-pulse rounded-lg bg-ink-800" />
          ) : me ? (
            <>
              <Link
                href="/wallet"
                title="Ver carteira"
                className="hidden rounded-md px-2 py-1 text-right transition hover:bg-ink-800 sm:block"
              >
                <p className="text-[11px] uppercase tracking-wider text-faint">Saldo</p>
                <p className="text-sm font-semibold text-cyan-soft tabular-nums">
                  {formatPrice(me.balanceCents)}
                </p>
              </Link>
              <Link
                href="/profile"
                title={me.tradeUrl ? "Perfil" : "Cadastre sua trade URL"}
                className="relative flex items-center gap-2 rounded-full border border-line bg-ink-850 py-1 pr-3 pl-1 transition hover:border-pink/60"
              >
                <Image
                  src={me.avatarUrl}
                  alt=""
                  width={28}
                  height={28}
                  className="rounded-full ring-1 ring-pink/60"
                />
                <span className="max-w-32 truncate text-sm">{me.displayName}</span>
                {!me.tradeUrl && (
                  <span className="absolute -top-0.5 -right-0.5 size-2.5 rounded-full bg-warning ring-2 ring-ink-950">
                    <span className="sr-only">Trade URL não cadastrada</span>
                  </span>
                )}
              </Link>
              <Button variant="ghost" onClick={() => logout.mutate()} loading={logout.isPending}>
                Sair
              </Button>
            </>
          ) : (
            <SteamLoginButton />
          )}
        </div>
      </div>

      <nav className="flex gap-1 overflow-x-auto border-t border-line/60 px-4 py-2 sm:hidden">
        <Nav items={items} variant="mobile" />
      </nav>
    </header>
  );
}
