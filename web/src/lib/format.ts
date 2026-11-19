const brl = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });

/** Centavos → "R$ 15,50". A divisão é só para exibir; nenhuma conta é feita com float. */
export function formatPrice(cents: number): string {
  return brl.format(cents / 100);
}

/**
 * "15,50" | "15.50" | "1.234,56" | "R$ 15" → centavos (inteiro), ou null se inválido.
 * Converte texto direto para inteiro, sem passar por float (0.1 + 0.2 !== 0.3).
 */
export function parsePriceToCents(input: string): number | null {
  let s = input.replace(/R\$|\s/g, "");
  if (s.includes(",")) s = s.replace(/\./g, "").replace(",", "."); // formato BR
  const m = /^(\d{1,9})(?:\.(\d{1,2}))?$/.exec(s);
  if (!m) return null;
  const reais = Number(m[1]);
  const centavos = Number((m[2] ?? "").padEnd(2, "0"));
  return reais * 100 + centavos;
}

export function formatFloat(f: number): string {
  return f.toFixed(f < 0.01 ? 6 : 4);
}

const rtf = new Intl.RelativeTimeFormat("pt-BR", { numeric: "auto" });

export function timeAgo(iso: string, now = Date.now()): string {
  const secs = Math.round((new Date(iso).getTime() - now) / 1000);
  const abs = Math.abs(secs);
  if (abs < 60) return rtf.format(secs, "second");
  if (abs < 3600) return rtf.format(Math.round(secs / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(secs / 3600), "hour");
  return rtf.format(Math.round(secs / 86400), "day");
}

export function formatDate(iso: string): string {
  return new Date(iso).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" });
}

/** Faixas de desgaste do CS2: Factory New ... Battle-Scarred. */
export const WEAR_RANGES = [
  { short: "FN", label: "Factory New", min: 0, max: 0.07 },
  { short: "MW", label: "Minimal Wear", min: 0.07, max: 0.15 },
  { short: "FT", label: "Field-Tested", min: 0.15, max: 0.38 },
  { short: "WW", label: "Well-Worn", min: 0.38, max: 0.45 },
  { short: "BS", label: "Battle-Scarred", min: 0.45, max: 1 },
] as const;

export function wearShort(exterior: string): string | undefined {
  return WEAR_RANGES.find((w) => w.label === exterior)?.short;
}
