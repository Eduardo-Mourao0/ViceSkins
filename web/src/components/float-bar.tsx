import { WEAR_RANGES, formatFloat } from "@/lib/format";

// Cores das faixas, do Factory New (bom) ao Battle-Scarred (gasto).
const wearColors = ["#2de2e6", "#3ee08f", "#ffc24b", "#ff8a4b", "#ff5470"];

/** Barra de desgaste com marcador na posição do float. */
export function FloatBar({ value, compact = false }: { value: number; compact?: boolean }) {
  const pct = Math.min(Math.max(value, 0), 1) * 100;
  return (
    <div className="w-full" title={`Float ${value}`}>
      <div className="relative flex h-1 w-full overflow-visible rounded-full">
        {WEAR_RANGES.map((w, i) => (
          <div
            key={w.short}
            style={{ width: `${(w.max - w.min) * 100}%`, backgroundColor: wearColors[i] }}
            className="h-full opacity-70 first:rounded-l-full last:rounded-r-full"
          />
        ))}
        <div
          className="absolute -top-1 h-3 w-0.5 -translate-x-1/2 rounded-full bg-text shadow-[0_0_6px_white]"
          style={{ left: `${pct}%` }}
        />
      </div>
      {!compact && (
        <p className="mt-1.5 font-mono text-[11px] text-muted tabular-nums">{formatFloat(value)}</p>
      )}
    </div>
  );
}
