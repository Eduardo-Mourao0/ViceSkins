"use client";

import { useId, useState } from "react";
import { RequireLogin } from "@/components/require-login";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { formatDate, formatPrice, parsePriceToCents } from "@/lib/format";
import { useDeposit, useWallet } from "@/lib/queries";
import type { WalletEntryKind } from "@/lib/types";

const MIN_CENTS = 100;
const MAX_CENTS = 1_000_000;
const QUICK_AMOUNTS = [5_000, 10_000, 50_000, 100_000]; // R$ 50, 100, 500, 1.000

const kindLabel: Record<WalletEntryKind, string> = {
  deposit: "Depósito",
  purchase: "Compra",
  sale: "Venda",
  refund: "Estorno",
};

export default function WalletPage() {
  return <RequireLogin>{() => <WalletView />}</RequireLogin>;
}

function WalletView() {
  const { data: wallet, isPending, error } = useWallet(true);

  return (
    <div className="grid gap-6 lg:grid-cols-[22rem_1fr]">
      <div className="flex flex-col gap-6">
        <section className="card relative overflow-hidden p-6">
          <div className="absolute -top-16 -right-16 size-48 rounded-full bg-cyan/20 blur-3xl" />
          <p className="text-xs tracking-wider text-faint uppercase">Saldo disponível</p>
          <p className="mt-1 text-4xl font-bold text-neon-cyan tabular-nums">
            {wallet ? formatPrice(wallet.balanceCents) : "—"}
          </p>
        </section>
        <DepositForm />
      </div>

      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">Extrato</h2>
        {isPending ? (
          <div className="flex justify-center py-16 text-muted">
            <Spinner className="size-6" />
          </div>
        ) : error ? (
          <Alert>Erro ao carregar o extrato: {error.message}</Alert>
        ) : wallet.entries.length === 0 ? (
          <EmptyState title="Nenhuma movimentação ainda">Faça um depósito para começar a comprar.</EmptyState>
        ) : (
          <ul className="card divide-y divide-line/70">
            {wallet.entries.map((e) => {
              const credit = e.amountCents > 0;
              return (
                <li key={e.id} className="flex items-center gap-4 px-4 py-3">
                  <span
                    className={`flex size-9 shrink-0 items-center justify-center rounded-full text-sm font-bold ${
                      credit ? "bg-success/15 text-success" : "bg-pink/15 text-pink"
                    }`}
                    aria-hidden
                  >
                    {credit ? "+" : "−"}
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium">{kindLabel[e.kind] ?? e.kind}</p>
                    <p className="truncate text-xs text-muted">
                      {e.description} · {formatDate(e.createdAt)}
                    </p>
                  </div>
                  <div className="text-right">
                    <p className={`font-semibold tabular-nums ${credit ? "text-success" : "text-pink"}`}>
                      {credit ? "+" : "−"}
                      {formatPrice(Math.abs(e.amountCents))}
                    </p>
                    <p className="text-[11px] text-faint tabular-nums">
                      saldo {formatPrice(e.balanceAfterCents)}
                    </p>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}

function DepositForm() {
  const id = useId();
  const [value, setValue] = useState("");
  const deposit = useDeposit();

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
    deposit.mutate(cents, { onSuccess: () => setValue("") });
  }

  return (
    <form onSubmit={submit} className="card flex flex-col gap-4 p-6">
      <div>
        <h2 className="font-semibold">Depositar</h2>
        <p className="text-xs text-muted">Ambiente de demonstração: o depósito é simulado, nenhum valor é cobrado.</p>
      </div>

      <div className="grid grid-cols-4 gap-2">
        {QUICK_AMOUNTS.map((a) => (
          <button
            key={a}
            type="button"
            onClick={() => setValue(String(a / 100))}
            className="rounded-lg border border-line py-2 text-xs text-muted transition hover:border-cyan hover:text-cyan-soft"
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

      {deposit.error && <Alert>{deposit.error.message}</Alert>}
      {deposit.isSuccess && !value && (
        <Alert tone="info">Depósito de {formatPrice(deposit.data.amountCents)} feito.</Alert>
      )}

      <Button type="submit" loading={deposit.isPending} disabled={cents === null || !!error}>
        Depositar {cents && !error ? formatPrice(cents) : ""}
      </Button>
    </form>
  );
}
