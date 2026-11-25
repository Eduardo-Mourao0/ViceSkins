"use client";

import type { ReactNode } from "react";
import { useMe } from "@/lib/queries";
import type { User } from "@/lib/types";
import { EmptyState, Spinner, SteamLoginButton } from "./ui";

/** Renderiza children só com usuário logado; senão mostra o convite para login. */
export function RequireLogin({ children }: { children: (me: User) => ReactNode }) {
  const { data: me, isPending, isError } = useMe();

  if (isPending) {
    return (
      <div className="flex justify-center py-24 text-muted">
        <Spinner className="size-6" />
      </div>
    );
  }
  if (isError) {
    return <EmptyState title="Não foi possível falar com a API">Tente recarregar a página.</EmptyState>;
  }
  if (!me) {
    return (
      <EmptyState
        title="Entre para continuar"
        action={
          <div className="flex flex-wrap justify-center gap-2">
            <SteamLoginButton />
          </div>
        }
      >
        O login é feito pela própria Steam. Nunca vemos sua senha.
      </EmptyState>
    );
  }
  return <>{children(me)}</>;
}
