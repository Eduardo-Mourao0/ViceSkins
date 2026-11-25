"use client";

import { useQueryClient } from "@tanstack/react-query";
import { QRCodeSVG } from "qrcode.react";
import { useEffect, useId, useState } from "react";
import { isApiError } from "@/lib/api";
import { formatPrice, parsePriceToCents } from "@/lib/format";
import { keys, useCreateDeposit, useDepositStatus, useOpenDeposit, useSimulatePixPayment } from "@/lib/queries";
import type { Deposit } from "@/lib/types";
import { useNow } from "@/lib/use-now";
import { Alert, Button, Spinner } from "./ui";

const MIN_CENTS = 100;
const MAX_CENTS = 1_000_000;
const QUICK_AMOUNTS = [5_000, 10_000, 50_000, 100_000]; // R$ 50, 100, 500, 1.000

/**
 * Depósito via Pix: escolher o valor → QR Code e copia e cola → confirmação.
 * Retoma o Pix em aberto se o usuário sair e voltar para a página.
 */
export function PixDeposit() {
  const open = useOpenDeposit();
  // undefined: segue o Pix em aberto (se houver). null: o usuário quer um novo.
  const [chosenId, setChosenId] = useState<number | null | undefined>(undefined);
  const depositId = chosenId === undefined ? (open.data?.id ?? null) : chosenId;
  const { data: deposit } = useDepositStatus(depositId);

  if (open.isPending) {
    return (
      <section className="card flex justify-center p-10 text-muted">
        <Spinner className="size-5" />
      </section>
    );
  }
  if (depositId === null || !deposit) {
    return <AmountForm onCreated={setChosenId} />;
  }
  return <PixCharge deposit={deposit} onNew={() => setChosenId(null)} onCreated={setChosenId} />;
}

function AmountForm({ onCreated }: { onCreated: (id: number) => void }) {
  const id = useId();
  const [value, setValue] = useState("");
  const create = useCreateDeposit();

  const cents = parsePriceToCents(value);
  const error =
    value === ""
      ? null
      : cents === null
        ? "Use um valor como 150,00"
        : cents < MIN_CENTS
          ? `Mínimo de ${formatPrice(MIN_CENTS)}`
          : cents > MAX_CENTS
            ? `Máximo de ${formatPrice(MAX_CENTS)} por depósito`
            : null;

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (cents === null || error) return;
    create.mutate(cents, { onSuccess: (d) => onCreated(d.id) });
  }

  return (
    <form onSubmit={submit} className="card flex flex-col gap-4 p-6">
      <div className="flex items-center gap-3">
        <PixIcon className="size-8 text-cyan" />
        <div>
          <h2 className="font-semibold">Depositar com Pix</h2>
          <p className="text-xs text-muted">O saldo entra assim que o pagamento é confirmado.</p>
        </div>
      </div>

      <div className="grid grid-cols-4 gap-2">
        {QUICK_AMOUNTS.map((a) => (
          <button
            key={a}
            type="button"
            onClick={() => setValue(String(a / 100))}
            className={`rounded-lg border py-2 text-xs transition ${
              cents === a ? "border-cyan bg-cyan/10 text-cyan-soft" : "border-line text-muted hover:border-cyan hover:text-cyan-soft"
            }`}
          >
            {formatPrice(a).replace(",00", "")}
          </button>
        ))}
      </div>

      <div>
        <label htmlFor={id} className="mb-1.5 block text-sm text-muted">
          Valor
        </label>
        <div className="flex items-center rounded-lg border border-line bg-ink-850 focus-within:border-cyan">
          <span className="pl-3 text-sm text-faint">R$</span>
          <input
            id={id}
            inputMode="decimal"
            autoComplete="off"
            placeholder="0,00"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            className="w-full bg-transparent px-2 py-2.5 text-lg tabular-nums outline-none"
          />
        </div>
        {error && <p className="mt-1.5 text-xs text-danger">{error}</p>}
      </div>

      {create.error && <Alert>{createError(create.error)}</Alert>}

      <Button type="submit" loading={create.isPending} disabled={cents === null || !!error}>
        Gerar Pix {cents && !error ? `de ${formatPrice(cents)}` : ""}
      </Button>
    </form>
  );
}

function PixCharge({
  deposit,
  onNew,
  onCreated,
}: {
  deposit: Deposit;
  onNew: () => void;
  onCreated: (id: number) => void;
}) {
  const qc = useQueryClient();
  const now = useNow(1_000);
  const remainingMs = new Date(deposit.expiresAt).getTime() - now;
  // Venceu no relógio do navegador: mostra como expirado sem esperar o job da API.
  const status = deposit.status === "pending" && remainingMs <= 0 ? "expired" : deposit.status;

  // Pago: saldo, extrato e dívida mudaram.
  useEffect(() => {
    if (deposit.status !== "pending") {
      qc.invalidateQueries({ queryKey: keys.me });
      qc.invalidateQueries({ queryKey: keys.wallet });
      qc.invalidateQueries({ queryKey: keys.openDeposit });
    }
  }, [deposit.status, qc]);

  if (status === "paid") {
    return (
      <section className="card relative flex flex-col items-center gap-4 overflow-hidden p-8 text-center">
        <div className="absolute -top-20 size-56 rounded-full bg-success/20 blur-3xl" />
        <span className="relative flex size-16 items-center justify-center rounded-full bg-success/15 text-success shadow-[0_0_32px_-6px_var(--color-success)] ring-1 ring-success/50">
          <svg viewBox="0 0 24 24" className="size-8" fill="none" stroke="currentColor" strokeWidth={2.5} aria-hidden>
            <path d="M5 12.5l4.5 4.5L19 7.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </span>
        <div className="relative">
          <p className="text-lg font-semibold">Depósito confirmado</p>
          <p className="mt-1 text-3xl font-bold text-success tabular-nums">+{formatPrice(deposit.amountCents)}</p>
          <p className="mt-1 text-xs text-muted">Já está na sua carteira.</p>
        </div>
        <Button variant="secondary" onClick={onNew} className="relative">
          Fazer outro depósito
        </Button>
      </section>
    );
  }

  if (status === "expired") {
    return <ExpiredCharge deposit={deposit} onNew={onNew} onCreated={onCreated} />;
  }

  return <PendingCharge deposit={deposit} remainingMs={remainingMs} onNew={onNew} />;
}

function PendingCharge({ deposit, remainingMs, onNew }: { deposit: Deposit; remainingMs: number; onNew: () => void }) {
  const simulate = useSimulatePixPayment();
  const totalMs = new Date(deposit.expiresAt).getTime() - new Date(deposit.createdAt).getTime();
  const progress = Math.max(0, Math.min(1, remainingMs / totalMs));

  return (
    <section className="card relative flex flex-col gap-5 overflow-hidden p-6">
      <div className="absolute -top-16 -right-16 size-48 rounded-full bg-cyan/15 blur-3xl" />

      <div className="relative flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <PixIcon className="size-6 text-cyan" />
          <h2 className="font-semibold">Pague com Pix</h2>
        </div>
        <span className="flex items-center gap-1.5 rounded-full bg-cyan/10 px-2.5 py-1 text-[11px] font-medium text-cyan-soft">
          <span className="relative flex size-2">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-cyan opacity-75" />
            <span className="relative inline-flex size-2 rounded-full bg-cyan" />
          </span>
          Aguardando pagamento
        </span>
      </div>

      <div className="relative text-center">
        <p className="text-xs tracking-wider text-faint uppercase">Valor</p>
        <p className="text-3xl font-bold text-neon-cyan tabular-nums">{formatPrice(deposit.amountCents)}</p>
      </div>

      <div className="relative mx-auto rounded-2xl bg-white p-3 shadow-neon-cyan">
        <QRCodeSVG value={deposit.brCode} size={200} level="M" marginSize={0} bgColor="#ffffff" fgColor="#07040d" />
      </div>

      <div className="relative">
        <div className="mb-1.5 flex justify-between text-xs">
          <span className="text-muted">Expira em</span>
          <span className={`font-mono tabular-nums ${remainingMs < 60_000 ? "text-warning" : "text-text"}`}>
            {formatRemaining(remainingMs)}
          </span>
        </div>
        <div className="h-1 overflow-hidden rounded-full bg-ink-700">
          <div
            className="h-full rounded-full bg-gradient-to-r from-pink to-cyan transition-[width] duration-1000 ease-linear"
            style={{ width: `${progress * 100}%` }}
          />
        </div>
      </div>

      <CopyPaste code={deposit.brCode} />

      <ol className="relative flex flex-col gap-2 text-sm text-muted">
        {["Abra o app do seu banco e entre na área Pix.", "Leia o QR Code ou cole o código copiado.", "Confirme o pagamento: o saldo entra na hora."].map(
          (step, i) => (
            <li key={i} className="flex gap-2.5">
              <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-ink-700 text-[11px] font-semibold text-cyan-soft">
                {i + 1}
              </span>
              {step}
            </li>
          ),
        )}
      </ol>

      {deposit.canSimulate && (
        <div className="relative flex flex-col gap-3 rounded-xl border border-dashed border-warning/50 bg-warning/5 p-4">
          <p className="text-xs text-[#ffe1a3]">
            <strong className="font-semibold">Dinheiro fictício.</strong> Este Pix é de demonstração: nenhum banco aceita o
            código e nenhum valor é cobrado. Use o botão abaixo no lugar do app do banco.
          </p>
          {simulate.error && <Alert>{simulateError(simulate.error)}</Alert>}
          <Button onClick={() => simulate.mutate(deposit.id)} loading={simulate.isPending}>
            Simular pagamento
          </Button>
        </div>
      )}

      <button type="button" onClick={onNew} className="relative self-center text-xs text-faint hover:text-text">
        Escolher outro valor
      </button>
    </section>
  );
}

function ExpiredCharge({
  deposit,
  onNew,
  onCreated,
}: {
  deposit: Deposit;
  onNew: () => void;
  onCreated: (id: number) => void;
}) {
  const create = useCreateDeposit();
  return (
    <section className="card flex flex-col items-center gap-4 p-8 text-center">
      <span className="flex size-14 items-center justify-center rounded-full bg-ink-700 text-faint">
        <svg viewBox="0 0 24 24" className="size-7" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 7v5l3 2" strokeLinecap="round" />
        </svg>
      </span>
      <div>
        <p className="font-semibold">Este Pix expirou</p>
        <p className="mt-1 text-sm text-muted">
          O código de {formatPrice(deposit.amountCents)} não vale mais. Nada foi cobrado.
        </p>
      </div>
      {create.error && <Alert>{createError(create.error)}</Alert>}
      <div className="flex flex-wrap justify-center gap-2">
        <Button onClick={() => create.mutate(deposit.amountCents, { onSuccess: (d) => onCreated(d.id) })} loading={create.isPending}>
          Gerar novo Pix
        </Button>
        <Button variant="ghost" onClick={onNew}>
          Outro valor
        </Button>
      </div>
    </section>
  );
}

function CopyPaste({ code }: { code: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2_000);
    } catch {
      // Sem permissão de clipboard: o usuário ainda pode selecionar o texto.
    }
  }

  return (
    <div className="relative">
      <p className="mb-1.5 text-xs text-muted">Pix copia e cola</p>
      <div className="flex items-center gap-2 rounded-lg border border-line bg-ink-850 p-1.5 pl-3">
        <input
          readOnly
          value={code}
          onFocus={(e) => e.target.select()}
          aria-label="Código Pix copia e cola"
          className="min-w-0 flex-1 bg-transparent font-mono text-xs text-muted outline-none"
        />
        <Button variant={copied ? "secondary" : "primary"} onClick={copy} className="shrink-0 px-3 py-1.5">
          {copied ? "Copiado ✓" : "Copiar"}
        </Button>
      </div>
    </div>
  );
}

/** Losango do Pix (forma simplificada, não é o logo oficial). */
function PixIcon({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden>
      <path d="M12 2.8 21.2 12 12 21.2 2.8 12Z" strokeLinejoin="round" />
      <path d="M8.2 12 12 8.2 15.8 12 12 15.8Z" fill="currentColor" stroke="none" opacity={0.85} />
    </svg>
  );
}

function formatRemaining(ms: number): string {
  const s = Math.max(0, Math.ceil(ms / 1000));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

// Mensagens da API (inglês) → texto para o usuário.
function createError(err: Error): string {
  if (isApiError(err, 409)) return "Você já tem 3 Pix em aberto. Pague um deles ou espere expirar.";
  return err.message;
}

function simulateError(err: Error): string {
  if (isApiError(err, 409)) return "Este Pix expirou. Gere um novo.";
  return err.message;
}
