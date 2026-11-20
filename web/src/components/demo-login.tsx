"use client";

import Image from "next/image";
import { useEffect, useRef, useState } from "react";
import { formatPrice } from "@/lib/format";
import { useDemoLogin, useDemoUsers } from "@/lib/queries";
import { Spinner } from "./ui";

/**
 * Login sem Steam nas contas da seed de demonstração.
 * Não renderiza nada se a API não expõe o login demo (DEV_LOGIN desligado).
 */
export function DemoLoginButton({ align = "right" }: { align?: "left" | "right" }) {
  const { data: users } = useDemoUsers();
  const login = useDemoLogin();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Fecha ao clicar fora.
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  if (!users?.length) return null;

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="inline-flex items-center gap-2 rounded-lg border border-cyan/50 px-4 py-2 text-sm text-cyan-soft transition hover:bg-cyan/10"
      >
        Entrar como demo
      </button>

      {open && (
        <div
          className={`absolute top-full z-40 mt-2 w-64 rounded-xl border border-line bg-ink-900 p-1.5 shadow-2xl ${
            align === "right" ? "right-0" : "left-0"
          }`}
        >
          <p className="px-2.5 py-1.5 text-[11px] text-faint">Contas de demonstração, sem Steam</p>
          {users.map((u) => (
            <button
              key={u.id}
              type="button"
              disabled={login.isPending}
              onClick={() => login.mutate(u.id, { onSuccess: () => setOpen(false) })}
              className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left transition hover:bg-ink-700 disabled:opacity-60"
            >
              <Image src={u.avatarUrl} alt="" width={28} height={28} className="rounded-full ring-1 ring-line" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm">{u.displayName}</span>
                <span className="block text-[11px] text-muted tabular-nums">{formatPrice(u.balanceCents)}</span>
              </span>
              {login.isPending && login.variables === u.id && <Spinner />}
            </button>
          ))}
          {login.error && <p className="px-2.5 py-1.5 text-xs text-danger">{login.error.message}</p>}
        </div>
      )}
    </div>
  );
}
