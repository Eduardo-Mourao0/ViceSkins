import type { NextConfig } from "next";

// Endereço da API visto pelo servidor do Next (no Docker é o serviço "api").
// O navegador nunca fala com a API direto: chama /api/* e o Next faz o proxy.
// Assim o front e a API ficam na mesma origem, sem CORS, e o cookie de sessão
// httpOnly é enviado automaticamente.
const apiURL = process.env.API_INTERNAL_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  images: {
    remotePatterns: [
      new URL("https://community.fastly.steamstatic.com/economy/image/**"),
      new URL("https://cdn.steamstatic.com/apps/730/**"),
      new URL("https://avatars.steamstatic.com/**"),
    ],
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          // Ninguém embute o site num iframe (clickjacking no botão de comprar).
          { key: "Content-Security-Policy", value: "frame-ancestors 'none'" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
        ],
      },
    ];
  },
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiURL}/:path*` }];
  },
};

export default nextConfig;
