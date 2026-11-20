import Link from "next/link";
import { Alert } from "./ui";

/** Aviso para cadastrar a trade URL antes de comprar ou anunciar. */
export function TradeURLNotice({ action }: { action: "comprar" | "anunciar" }) {
  return (
    <Alert tone="warning">
      Para {action}, cadastre a sua trade URL da Steam. É por ela que as ofertas de troca chegam.{" "}
      <Link href="/profile" className="font-semibold underline underline-offset-2">
        Cadastrar
      </Link>
    </Alert>
  );
}
