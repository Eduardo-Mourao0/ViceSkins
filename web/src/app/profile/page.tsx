"use client";

import Image from "next/image";
import { useId, useState } from "react";
import { RequireLogin } from "@/components/require-login";
import { Alert, Button } from "@/components/ui";
import { useDeleteSteamAPIKey, useSetSteamAPIKey, useSetTradeURL } from "@/lib/queries";
import type { User } from "@/lib/types";

// Página da Steam onde o usuário vê (e pode gerar de novo) a própria trade URL.
const STEAM_TRADE_URL_PAGE = "https://steamcommunity.com/my/tradeoffers/privacy#trade_offer_access_url";

export default function ProfilePage() {
  return <RequireLogin>{(me) => <Profile me={me} />}</RequireLogin>;
}

function Profile({ me }: { me: User }) {
  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-6">
      <section className="card flex items-center gap-4 p-6">
        <Image src={me.avatarUrl} alt="" width={56} height={56} className="rounded-full ring-2 ring-pink/60" />
        <div className="min-w-0">
          <h1 className="truncate text-xl font-bold">{me.displayName}</h1>
          <p className="font-mono text-xs text-faint">{me.steamId}</p>
        </div>
      </section>
      <TradeURLForm current={me.tradeUrl} />
      {!me.steamId.startsWith("demo-") && <SteamAPIKeyForm me={me} />}
    </div>
  );
}

function TradeURLForm({ current }: { current: string | null }) {
  const id = useId();
  const [value, setValue] = useState(current ?? "");
  const save = useSetTradeURL();
  const changed = value.trim() !== (current ?? "");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!changed) return;
    // A validação de verdade (formato e se a URL é da sua conta) é da API.
    save.mutate(value.trim(), { onSuccess: (me) => setValue(me.tradeUrl ?? "") });
  }

  return (
    <form onSubmit={submit} className="card flex flex-col gap-4 p-6">
      <div>
        <h2 className="font-semibold">Trade URL da Steam</h2>
        <p className="mt-1 text-sm text-muted">
          Obrigatória para comprar e anunciar: é o endereço para onde as ofertas de troca são enviadas. Ela precisa ser
          da mesma conta Steam com que você entrou.
        </p>
      </div>

      {!current && <Alert tone="warning">Você ainda não cadastrou sua trade URL.</Alert>}

      <div>
        <label htmlFor={id} className="mb-1.5 block text-sm text-muted">
          Trade URL
        </label>
        <input
          id={id}
          type="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://steamcommunity.com/tradeoffer/new/?partner=...&token=..."
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            save.reset();
          }}
          className="w-full rounded-lg border border-line bg-ink-850 px-3 py-2.5 font-mono text-sm outline-none focus:border-cyan"
        />
        <a
          href={STEAM_TRADE_URL_PAGE}
          target="_blank"
          rel="noreferrer"
          className="mt-1.5 inline-block text-xs text-cyan-soft hover:underline"
        >
          Onde encontro a minha? ↗
        </a>
      </div>

      {save.error && <Alert>{tradeURLError(save.error.message)}</Alert>}
      {save.isSuccess && <Alert tone="info">Trade URL salva.</Alert>}

      <Button type="submit" className="self-end" loading={save.isPending} disabled={!changed || !value.trim()}>
        Salvar
      </Button>
    </form>
  );
}

// Mensagens da API (inglês) → texto para o usuário.
function tradeURLError(message: string): string {
  if (message.includes("another steam account")) return "Essa trade URL é de outra conta Steam.";
  if (message.includes("token")) return "O token da trade URL é inválido. Copie a URL inteira da Steam.";
  if (message.includes("partner")) return "O partner da trade URL é inválido. Copie a URL inteira da Steam.";
  if (message.includes("trade url")) return "Isso não parece uma trade URL da Steam.";
  return message;
}

const STEAM_API_KEY_PAGE = "https://steamcommunity.com/dev/apikey";

/**
 * API key opcional: deixa a plataforma ver as ofertas de troca do vendedor.
 * Confirmação mais rápida, horário exato da troca e cancelamento automático
 * da oferta quando a compra é cancelada. É obrigatória para anunciar itens
 * sem float (caixas, stickers), que não dá para reconhecer pelo inventário.
 */
function SteamAPIKeyForm({ me }: { me: User }) {
  const id = useId();
  const [value, setValue] = useState("");
  const save = useSetSteamAPIKey();
  const remove = useDeleteSteamAPIKey();

  function submit(e: React.FormEvent) {
    e.preventDefault();
    save.mutate(value.trim(), { onSuccess: () => setValue("") });
  }

  return (
    <form onSubmit={submit} className="card flex flex-col gap-4 p-6">
      <div>
        <h2 className="font-semibold">
          API key da Steam <span className="text-xs font-normal text-faint">(opcional, para vendedores)</span>
        </h2>
        <p className="mt-1 text-sm text-muted">
          Sem ela, as vendas são confirmadas pelo seu inventário público, o que é mais lento, e você avisa quando
          enviar a oferta. Com ela, a plataforma acompanha as suas ofertas de troca e as cancela sozinha se a compra for
          cancelada. É obrigatória para anunciar itens sem float, como caixas e stickers.
        </p>
      </div>

      <Alert tone="warning">
        A API key dá acesso às suas ofertas de troca. Ela é guardada criptografada e usada só para acompanhar as vendas
        feitas aqui. Nunca a informe em sites em que você não confia e, se suspeitar de vazamento, revogue-a na Steam.
      </Alert>

      {me.hasSteamApiKey ? (
        <div className="flex items-center justify-between gap-3 rounded-lg border border-line bg-ink-850 px-4 py-3">
          <p className="text-sm">
            Cadastrada <span className="font-mono text-muted">••••{me.steamApiKeyLast4}</span>
          </p>
          <Button type="button" variant="danger" loading={remove.isPending} onClick={() => remove.mutate()}>
            Remover
          </Button>
        </div>
      ) : (
        <div>
          <label htmlFor={id} className="mb-1.5 block text-sm text-muted">
            API key
          </label>
          <input
            id={id}
            type="password"
            autoComplete="off"
            spellCheck={false}
            placeholder="32 caracteres"
            value={value}
            onChange={(e) => {
              setValue(e.target.value);
              save.reset();
            }}
            className="w-full rounded-lg border border-line bg-ink-850 px-3 py-2.5 font-mono text-sm outline-none focus:border-cyan"
          />
          <a
            href={STEAM_API_KEY_PAGE}
            target="_blank"
            rel="noreferrer"
            className="mt-1.5 inline-block text-xs text-cyan-soft hover:underline"
          >
            Gerar a minha na Steam ↗
          </a>
        </div>
      )}

      {(save.error || remove.error) && <Alert>{apiKeyError((save.error ?? remove.error)!.message)}</Alert>}

      {!me.hasSteamApiKey && (
        <Button type="submit" className="self-end" loading={save.isPending} disabled={!value.trim()}>
          Salvar
        </Button>
      )}
    </form>
  );
}

function apiKeyError(message: string): string {
  if (message.includes("32 hexadecimal")) return "A API key tem 32 caracteres (números e letras de A a F).";
  if (message.includes("rejected")) return "A Steam recusou essa API key. Confira se copiou a key inteira.";
  if (message.includes("not enabled")) return "O cadastro de API key não está habilitado neste servidor.";
  return message;
}
