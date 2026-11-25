"use client";

import Link from "next/link";
import { RequireLogin } from "@/components/require-login";
import { PixDeposit } from "@/components/pix-deposit";
import { Alert, EmptyState, Spinner } from "@/components/ui";
import { formatDate, formatPrice } from "@/lib/format";
import { useMe, useWallet } from "@/lib/queries";
import type { WalletEntryKind } from "@/lib/types";

const kindLabel: Record<WalletEntryKind, string> = {
  deposit: "Depósito",
  purchase: "Compra",
  sale: "Venda",
  refund: "Estorno",
  fee: "Taxa da plataforma",
  dispute_charge: "Cobrança de entrega tardia",
  dispute_collect: "Cobrança de entrega tardia",
  dispute_payout: "Pagamento de entrega tardia",
  dispute_refund: "Cobrança desfeita",
  debt_payment: "Pagamento de dívida",
  debt_recovery: "Recuperação de dívida",
};

export default function WalletPage() {
  return <RequireLogin>{() => <WalletView />}</RequireLogin>;
}

function WalletView() {
  const { data: wallet, isPending, error } = useWallet(true);
  const { data: me } = useMe();

  return (
    <div className="grid gap-6 lg:grid-cols-[22rem_1fr]">
      <div className="flex flex-col gap-6">
        <section className="card relative overflow-hidden p-6">
          <div className="absolute -top-16 -right-16 size-48 rounded-full bg-cyan/20 blur-3xl" />
          <p className="text-xs tracking-wider text-faint uppercase">Saldo disponível</p>
          <p className="mt-1 text-4xl font-bold text-neon-cyan tabular-nums">
            {wallet ? formatPrice(wallet.balanceCents) : "—"}
          </p>
          {wallet && wallet.lockedSales > 0 && (
            <div className="relative mt-5 border-t border-line pt-4">
              <p className="text-xs tracking-wider text-faint uppercase">Bloqueado</p>
              <p className="mt-0.5 text-xl font-semibold text-pink-soft tabular-nums">
                {formatPrice(wallet.lockedCents)}
              </p>
              <p className="mt-1 text-xs text-muted">
                {wallet.lockedSales === 1 ? "1 venda" : `${wallet.lockedSales} vendas`} em proteção de troca. O valor
                entra no saldo quando a Steam não permitir mais reverter a troca
                {wallet.nextReleaseAt && <> (próxima liberação em {formatDate(wallet.nextReleaseAt)})</>}.
              </p>
            </div>
          )}
        </section>
        {me && me.debtCents > 0 && (
          <Alert tone="warning">
            Você deve {formatPrice(me.debtCents)} à plataforma: um item chegou para você depois de a compra ter sido
            estornada, e o saldo não cobria a nova cobrança. Os próximos depósitos quitam a dívida primeiro, e as compras
            ficam bloqueadas até lá.
          </Alert>
        )}
        <PixDeposit />
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
                      {e.transactionId && (
                        <>
                          {" · "}
                          <Link href={`/transactions/${e.transactionId}`} className="text-cyan-soft hover:underline">
                            transação #{e.transactionId}
                          </Link>
                        </>
                      )}
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
