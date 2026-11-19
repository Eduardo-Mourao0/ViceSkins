"use client";

import { useEffect, useState } from "react";
import { formatPrice, parsePriceToCents, WEAR_RANGES } from "@/lib/format";
import type { MarketSort } from "@/lib/types";

type Props = {
  params: URLSearchParams | { get(name: string): string | null; toString(): string };
  setParams: (patch: Record<string, string | null>) => void;
};

const sorts: { id: MarketSort; label: string }[] = [
  { id: "newest", label: "Mais recentes" },
  { id: "price_asc", label: "Menor preço" },
  { id: "price_desc", label: "Maior preço" },
  { id: "float_asc", label: "Menor float" },
];

const inputClass =
  "w-full rounded-lg border border-line bg-ink-900 px-3 py-2 text-sm outline-none placeholder:text-faint focus:border-cyan";

/** Centavos da URL → texto do input ("1550" → "15,50"). */
const centsToInput = (v: string | null) =>
  v ? formatPrice(Number(v)).replace(/^R\$\s?/, "").trim() : "";

export function MarketFilters({ params, setParams }: Props) {
  const [text, setText] = useState(params.get("q") ?? "");
  const [minPrice, setMinPrice] = useState(centsToInput(params.get("minPrice")));
  const [maxPrice, setMaxPrice] = useState(centsToInput(params.get("maxPrice")));
  const [minFloat, setMinFloat] = useState(params.get("minFloat") ?? "");
  const [maxFloat, setMaxFloat] = useState(params.get("maxFloat") ?? "");

  // Busca por texto com debounce: só atualiza a URL 300ms depois de parar de digitar.
  const q = params.get("q") ?? "";
  useEffect(() => {
    if (text.trim() === q) return;
    const id = setTimeout(() => setParams({ q: text.trim() || null }), 300);
    return () => clearTimeout(id);
  }, [text, q, setParams]);

  const wear = new Set((params.get("wear") ?? "").split(",").filter(Boolean));
  function toggleWear(short: string) {
    const next = new Set(wear);
    if (next.has(short)) next.delete(short);
    else next.add(short);
    setParams({ wear: [...next].join(",") || null });
  }

  const priceError =
    (minPrice && parsePriceToCents(minPrice) === null) || (maxPrice && parsePriceToCents(maxPrice) === null);
  function applyPrice() {
    if (priceError) return;
    setParams({
      minPrice: minPrice ? String(parsePriceToCents(minPrice)) : null,
      maxPrice: maxPrice ? String(parsePriceToCents(maxPrice)) : null,
    });
  }

  const floatInvalid = (s: string) => s !== "" && !(Number(s) >= 0 && Number(s) <= 1);
  const floatError = floatInvalid(minFloat) || floatInvalid(maxFloat);
  function applyFloat() {
    if (floatError) return;
    setParams({ minFloat: minFloat || null, maxFloat: maxFloat || null });
  }

  const hasFilters = ["q", "minPrice", "maxPrice", "wear", "minFloat", "maxFloat", "sort"].some((k) =>
    params.get(k),
  );
  function clearAll() {
    setText("");
    setMinPrice("");
    setMaxPrice("");
    setMinFloat("");
    setMaxFloat("");
    setParams({ q: null, minPrice: null, maxPrice: null, wear: null, minFloat: null, maxFloat: null, sort: null });
  }

  const onEnter = (fn: () => void) => (e: React.KeyboardEvent) => e.key === "Enter" && fn();

  return (
    <aside className="card flex h-fit flex-col gap-5 p-4 lg:sticky lg:top-24">
      <Field label="Buscar">
        <input
          type="search"
          placeholder="AK-47, Redline..."
          value={text}
          onChange={(e) => setText(e.target.value)}
          className={inputClass}
        />
      </Field>

      <Field label="Ordenar por">
        <select
          value={params.get("sort") ?? "newest"}
          onChange={(e) => setParams({ sort: e.target.value === "newest" ? null : e.target.value })}
          className={inputClass}
        >
          {sorts.map((s) => (
            <option key={s.id} value={s.id}>
              {s.label}
            </option>
          ))}
        </select>
      </Field>

      <Field label="Preço (R$)">
        <div className="flex items-center gap-2">
          <input
            inputMode="decimal"
            placeholder="Mín"
            value={minPrice}
            onChange={(e) => setMinPrice(e.target.value)}
            onBlur={applyPrice}
            onKeyDown={onEnter(applyPrice)}
            className={inputClass}
          />
          <span className="text-faint">–</span>
          <input
            inputMode="decimal"
            placeholder="Máx"
            value={maxPrice}
            onChange={(e) => setMaxPrice(e.target.value)}
            onBlur={applyPrice}
            onKeyDown={onEnter(applyPrice)}
            className={inputClass}
          />
        </div>
        {priceError && <p className="mt-1 text-xs text-danger">Use valores como 15,50</p>}
      </Field>

      <Field label="Desgaste">
        <div className="flex flex-wrap gap-1.5">
          {WEAR_RANGES.map((w) => {
            const on = wear.has(w.short);
            return (
              <button
                key={w.short}
                type="button"
                title={w.label}
                aria-pressed={on}
                onClick={() => toggleWear(w.short)}
                className={`rounded-md border px-2.5 py-1 text-xs font-semibold transition ${
                  on ? "border-cyan bg-cyan/15 text-cyan-soft" : "border-line text-muted hover:text-text"
                }`}
              >
                {w.short}
              </button>
            );
          })}
        </div>
      </Field>

      <Field label="Float">
        <div className="flex items-center gap-2">
          <input
            inputMode="decimal"
            placeholder="0.00"
            value={minFloat}
            onChange={(e) => setMinFloat(e.target.value.replace(",", "."))}
            onBlur={applyFloat}
            onKeyDown={onEnter(applyFloat)}
            className={inputClass}
          />
          <span className="text-faint">–</span>
          <input
            inputMode="decimal"
            placeholder="1.00"
            value={maxFloat}
            onChange={(e) => setMaxFloat(e.target.value.replace(",", "."))}
            onBlur={applyFloat}
            onKeyDown={onEnter(applyFloat)}
            className={inputClass}
          />
        </div>
        {floatError && <p className="mt-1 text-xs text-danger">Float vai de 0 a 1</p>}
      </Field>

      {hasFilters && (
        <button type="button" onClick={clearAll} className="text-left text-xs text-cyan-soft hover:underline">
          Limpar filtros
        </button>
      )}
    </aside>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="mb-1.5 text-xs font-semibold tracking-wider text-faint uppercase">{label}</p>
      {children}
    </div>
  );
}
