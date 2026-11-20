import type { TxStatus } from "@/lib/types";

export const txStatusInfo: Record<TxStatus, { label: string; hint: string; className: string }> = {
  pending: {
    label: "Processando",
    hint: "Pagamento confirmado. Aguardando o vendedor enviar a oferta de troca.",
    className: "bg-warning/15 text-warning",
  },
  sent: {
    label: "Oferta enviada",
    hint: "Aceite a oferta de troca na Steam para receber o item.",
    className: "bg-cyan/15 text-cyan-soft",
  },
  on_hold: {
    label: "Em proteção",
    hint: "Item entregue. A Steam mantém a troca protegida antes de liberar o pagamento ao vendedor.",
    className: "bg-pink/15 text-pink",
  },
  completed: { label: "Concluída", hint: "Transação finalizada.", className: "bg-success/15 text-success" },
  cancelled: {
    label: "Cancelada",
    hint: "O valor voltou para o seu saldo.",
    className: "bg-ink-700 text-muted",
  },
  failed: { label: "Falhou", hint: "O valor voltou para o seu saldo.", className: "bg-danger/15 text-danger" },
};

export function TxStatusBadge({ status }: { status: TxStatus }) {
  const s = txStatusInfo[status];
  return <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${s.className}`}>{s.label}</span>;
}

const steps: TxStatus[] = ["pending", "sent", "on_hold", "completed"];

/** Linha do tempo pending → sent → on_hold → completed. */
export function TxProgress({ status }: { status: TxStatus }) {
  const failed = status === "cancelled" || status === "failed";
  const current = steps.indexOf(status);
  return (
    <ol className="flex items-center gap-1.5" aria-label="Andamento da transação">
      {steps.map((s, i) => {
        const done = !failed && i <= current;
        return (
          <li key={s} className="flex flex-1 flex-col gap-1.5">
            <div
              className={`h-1 rounded-full transition-colors ${
                done ? (i === current && s !== "completed" ? "animate-pulse bg-cyan" : "bg-cyan") : "bg-ink-700"
              }`}
            />
            <span className={`text-[10px] ${done ? "text-cyan-soft" : "text-faint"}`}>{txStatusInfo[s].label}</span>
          </li>
        );
      })}
    </ol>
  );
}
