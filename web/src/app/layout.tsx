import type { Metadata } from "next";
import { Sora, Yellowtail } from "next/font/google";
import { Providers } from "./providers";
import { Header } from "@/components/header";
import "./globals.css";

const sora = Sora({ variable: "--font-sora", subsets: ["latin"] });
const yellowtail = Yellowtail({ variable: "--font-yellowtail", subsets: ["latin"], weight: "400" });

export const metadata: Metadata = {
  title: { default: "ViceSkins", template: "%s · ViceSkins" },
  description: "Marketplace de skins de CS2.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="pt-BR" className={`${sora.variable} ${yellowtail.variable} h-full antialiased`}>
      <body className="flex min-h-full flex-col font-sans">
        <Providers>
          <Header />
          <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-8 sm:px-6">{children}</main>
          <footer className="border-t border-line/60 py-6 text-center text-xs text-faint">
            ViceSkins · projeto de portfólio · o saldo é fictício (Pix simulado), as trocas são reais na Steam · não
            afiliado à Valve
          </footer>
        </Providers>
      </body>
    </html>
  );
}
