"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { RequireLogin } from "@/components/require-login";
import { Alert, Button, EmptyState, Spinner } from "@/components/ui";
import { formatDate, formatPrice } from "@/lib/format";
import { useDisputes, useDismissDispute } from "@/lib/queries";
import type { Dispute, DisputeStatus } from "@/lib/types";

const filters: { value: DisputeStatus | "all"; label: string }[] = [
  { value: "holding", label: "Em andamento" },
  { value: "settled", label: "Pagas" },
  { value: "reversed", label: "Revertidas" },
  { value: "dismissed", label: "Arquivadas" },
  { value: "all", label: "Todas" },
];

const statusLabel: Record<DisputeStatus, { text: string; className: string }> = {
  holding: { text: "Em proteção", className: "bg-pink/15 text-pink" },
  settled: { text: "Vendedor pago", className: "bg-success/15 text-success" },
  reversed: { text: "Troca revertida", className: "bg-ink-700 text-muted" },
  dismissed: { text: "Arquivada", className: "bg-warning/15 text-warning" },
};

export default function DisputesPage() {
  return (
    <RequireLogin>
      {(me) =>
        me.role === "admin" ? (
          <Disputes />
        ) : (
          <EmptyState title="Acesso restrito">Esta página é só para administradores.</EmptyState>
        )
      }
    </RequireLogin>
  );
}

function Disputes() {
  const [status, setStatus] = useState<DisputeStatus | "all">("holding");
  const { data, isPending, error } = useDisputes(status);

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold">Disputas</h1>
        <p className="mt-1 max-w-3xl text-sm text-muted">
          Entregas tardias: o item chegou ao comprador depois de a compra ter sido estornada. O comprador já foi cobrado de
          novo, e o vendedor recebe da plataforma no fim da proteção de troca. Arquive só se a detecção estiver errada:
          isso desfaz a cobrança e devolve o item ao vendedor.
        </p>
      </div>

      <div className="flex flex-wrap gap-1.5">
        {filters.map((f) => (
          <button
            key={f.value}
            type="button"
            onClick={() => setStatus(f.value)}
            className={`rounded-md px-3 py-1.5 text-sm transition ${
              status === f.value ? "bg-ink-700 text-text" : "text-muted hover:text-text"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      {isPending ? (
        <div className="flex justify-center py-16 text-muted">
          <Spinner className="size-6" />
        </div>
      ) : error ? (
        <Alert>{error.message}</Alert>
      ) : data.length === 0 ? (
        <EmptyState title="Nenhuma disputa aqui" />
      ) : (
        <ul className="flex flex-col gap-4">
          {data.map((d) => (
            <DisputeCard key={d.id} d={d} />
          ))}
        </ul>
      )}
    </div>
  );
}

function DisputeCard({ d }: { d: Dispute }) {
  const s = statusLabel[d.status];
  return (
    <li className="card flex flex-col gap-4 p-5">
      <div className="flex flex-wrap items-start gap-4">
        <Image src={d.itemImageUrl} alt="" width={96} height={72} className="h-16 w-auto rounded bg-ink-850 object-contain" />
        <div className="min-w-0 flex-1">
          <p className="text-xs text-faint">
            Disputa #{d.id} ·{" "}
            <Link href={`/transactions/${d.transactionId}`} className="text-cyan-soft hover:underline">
              transação #{d.transactionId}
            </Link>
          </p>
          <p className="truncate font-semibold">{d.itemName}</p>
          <p className="text-sm text-muted">
            {d.sellerName} → {d.buyerName} · {formatPrice(d.priceCents)}
          </p>
        </div>
        <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${s.className}`}>{s.text}</span>
      </div>

      <dl className="grid gap-x-6 gap-y-1.5 text-sm sm:grid-cols-2">
        <Row label="Estornada em" value={formatDate(d.refundedAt)} />
        <Row label="Item visto no comprador em" value={formatDate(d.detectedAt)} />
        <Row
          label="Evidência"
          value={d.evidence === "offer" ? "oferta aceita (API key do vendedor)" : "impressão digital no inventário"}
        />
        <Row label="Troca na Steam" value={d.tradedAt ? formatDate(d.tradedAt) : "horário não informado"} />
        <Row label="Cobrado do saldo" value={formatPrice(d.chargedCents)} />
        <Row
          label="Coberto pela plataforma"
          value={d.coveredCents > 0 ? `${formatPrice(d.coveredCents)} (dívida atual: ${formatPrice(d.buyerDebtCents)})` : "—"}
        />
        <Row label="Vendedor recebe" value={formatPrice(d.sellerReceivesCents)} />
        <Row label={d.status === "holding" ? "Libera em" : "Resolvida em"} value={formatDate(d.resolvedAt ?? d.holdUntil)} />
        {d.failureReason && <Row label="Motivo do estorno" value={d.failureReason} />}
        {d.note && <Row label="Nota do admin" value={d.note} />}
      </dl>

      {d.status === "holding" && <DismissForm id={d.id} />}
    </li>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line/50 py-1">
      <dt className="text-muted">{label}</dt>
      <dd className="text-right">{value}</dd>
    </div>
  );
}

function DismissForm({ id }: { id: number }) {
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const dismiss = useDismissDispute();

  if (!open) {
    return (
      <Button variant="danger" className="self-start" onClick={() => setOpen(true)}>
        Arquivar disputa…
      </Button>
    );
  }
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        dismiss.mutate({ id, note: note.trim() });
      }}
      className="flex flex-col gap-3 rounded-lg border border-danger/40 bg-danger/5 p-4"
    >
      <p className="text-sm">
        Arquivar desfaz a cobrança do comprador (inclusive o que ele já pagou da dívida) e devolve o item ao vendedor. Use
        só se a detecção estiver errada.
      </p>
      <textarea
        required
        rows={2}
        placeholder="Por que a detecção está errada? (obrigatório)"
        value={note}
        onChange={(e) => setNote(e.target.value)}
        className="rounded-lg border border-line bg-ink-850 px-3 py-2 text-sm outline-none focus:border-cyan"
      />
      {dismiss.error && <Alert>{dismiss.error.message}</Alert>}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={dismiss.isPending}>
          Voltar
        </Button>
        <Button type="submit" variant="danger" loading={dismiss.isPending} disabled={!note.trim()}>
          Arquivar
        </Button>
      </div>
    </form>
  );
}
