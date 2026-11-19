import { Suspense } from "react";
import { Spinner } from "@/components/ui";
import { Market } from "./market";

export const metadata = { title: "Mercado" };

// A página em si é estática; a parte que lê a URL (filtros) roda no navegador
// dentro do Suspense, como pede o Next com cacheComponents.
export default function MarketPage() {
  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold">Mercado</h1>
        <p className="text-sm text-muted">Skins à venda por outros jogadores.</p>
      </div>
      <Suspense
        fallback={
          <div className="flex justify-center py-24 text-muted">
            <Spinner className="size-6" />
          </div>
        }
      >
        <Market />
      </Suspense>
    </div>
  );
}
