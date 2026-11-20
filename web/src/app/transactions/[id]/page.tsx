import { Suspense } from "react";
import { Spinner } from "@/components/ui";
import { TransactionView } from "./transaction-view";

export const metadata = { title: "Transação" };

// Com cacheComponents, o id da URL só é conhecido na requisição: o acesso a
// params fica dentro do Suspense, e o resto da página é pré-renderizado.
export default function TransactionPage({ params }: PageProps<"/transactions/[id]">) {
  return (
    <Suspense
      fallback={
        <div className="flex justify-center py-24 text-muted">
          <Spinner className="size-6" />
        </div>
      }
    >
      {params.then(({ id }) => (
        <TransactionView id={Number(id)} />
      ))}
    </Suspense>
  );
}
