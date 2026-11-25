"use client";

import { useEffect, useState } from "react";

/** Hora atual em estado, atualizada a cada intervalMs: o render continua puro. */
export function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(t);
  }, [intervalMs]);
  return now;
}
