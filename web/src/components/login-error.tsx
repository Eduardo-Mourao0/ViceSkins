"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";
import { SteamLoginButton } from "./ui";

// Códigos que a API manda na URL quando o login com a Steam falha.
const messages: Record<string, string> = {
  expirou: "O login demorou demais ou foi aberto em outra aba. Tente de novo.",
  recusado: "A Steam não confirmou o login. Tente de novo.",
  indisponivel: "A Steam não respondeu. Tente de novo em alguns minutos.",
};

/** Aviso de login que falhou (?login=erro&motivo=...). */
export function LoginError() {
  // useSearchParams só existe na requisição: o Suspense deixa o resto da
  // página ser pré-renderizado sem ele.
  return (
    <Suspense fallback={null}>
      <LoginErrorBanner />
    </Suspense>
  );
}

function LoginErrorBanner() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  if (params.get("login") !== "erro") return null;

  // Código desconhecido cai na mensagem genérica: o texto nunca vem da URL.
  const message = messages[params.get("motivo") ?? ""] ?? "Tente de novo.";

  function dismiss() {
    // Tira o aviso da URL, para ele não voltar ao recarregar a página.
    const rest = new URLSearchParams(params);
    rest.delete("login");
    rest.delete("motivo");
    const query = rest.toString();
    router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
  }

  return (
    <div className="mx-auto w-full max-w-7xl px-4 pt-6 sm:px-6">
      <div
        role="alert"
        className="flex flex-wrap items-center gap-4 rounded-xl border border-warning/40 bg-warning/10 px-4 py-3"
      >
        <svg viewBox="0 0 24 24" className="size-5 shrink-0 text-warning" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden>
          <path d="M12 3 2.5 20h19L12 3Z" strokeLinejoin="round" />
          <path d="M12 10v4m0 3h.01" strokeLinecap="round" />
        </svg>
        <div className="min-w-0 flex-1 text-sm">
          <p className="font-semibold text-[#ffe1a3]">Não foi possível entrar com a Steam.</p>
          <p className="text-muted">{message}</p>
        </div>
        <SteamLoginButton />
        <button
          type="button"
          onClick={dismiss}
          aria-label="Fechar aviso"
          className="rounded-md p-1.5 text-muted transition hover:bg-ink-700 hover:text-text"
        >
          <svg viewBox="0 0 24 24" className="size-4" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden>
            <path d="M6 6l12 12M18 6 6 18" strokeLinecap="round" />
          </svg>
        </button>
      </div>
    </div>
  );
}
