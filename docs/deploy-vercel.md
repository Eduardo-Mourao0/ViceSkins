# Deploy na Vercel

Dois projetos na Vercel, do mesmo repositório, mais um Postgres e um cron externos:

```
navegador ──▶ web (Next, projeto "viceskins")  ──/api/*──▶  api (Go, projeto "viceskins-api") ──▶ Postgres (Supabase)
                                                                   ▲
                                         cron-job.org, a cada minuto: POST /internal/jobs/run
```

**Por que o cron externo:** na Vercel a API só roda enquanto atende uma requisição, então os jobs em goroutine (expirar Pix, verificar trocas, liberar saldo, resolver disputas) parariam. Com `JOBS_MODE=external` eles não sobem sozinhos, e um cron chama `POST /internal/jobs/run` a cada minuto. O cron da própria Vercel não serve: no plano grátis ele roda uma vez por dia.

## 1. Banco (Supabase, grátis)

1. Crie um projeto em https://supabase.com e guarde a senha do banco.
2. Em **Connect**, copie a connection string do **Session pooler** (porta 5432). A conexão direta é só IPv6, e a Vercel sai por IPv4. Não use o *Transaction pooler* (porta 6543): o pgx usa prepared statements, que ele não suporta.
3. Acrescente `pool_max_conns=4` à string (cada instância da API abre no máximo 4 conexões):
   `postgresql://postgres.xxxx:SENHA@aws-0-sa-east-1.pooler.supabase.com:5432/postgres?sslmode=require&pool_max_conns=4`

Não precisa rodar migrations à mão: com `MIGRATE_ON_START=true` a API aplica as que faltarem ao subir.

> Por que não o Neon: o plano grátis dele limita as horas de banco ligado, e um cron a cada minuto não deixa o banco dormir nunca.

## 2. API (projeto `viceskins-api`)

1. **Add New → Project**, importe o repositório, **Root Directory: `api`**. O `api/vercel.json` já escolhe o preset Go, e ele acha o `cmd/api/main.go` sozinho.
2. Variáveis de ambiente (Production):

| Variável | Valor |
|---|---|
| `DATABASE_URL` | a string do Supabase do passo 1 |
| `MIGRATE_ON_START` | `true` |
| `JOBS_MODE` | `external` |
| `JOBS_SECRET` | `openssl rand -hex 32` |
| `JWT_SECRET` | `openssl rand -hex 32` |
| `SECRETS_KEY` | `openssl rand -base64 32` (perder = vendedores recadastram a API key) |
| `STEAM_API_KEY` | https://steamcommunity.com/dev/apikey, com o domínio do **front** |
| `TRADE_PROVIDER` | `steam` |
| `COOKIE_SECURE` | `true` |
| `PUBLIC_API_URL` | `https://<front>.vercel.app/api` |
| `AUTH_REDIRECT_URL` | `https://<front>.vercel.app/inventory` |
| `ADMIN_STEAM_IDS` | seu SteamID64 |

3. Deploy. Confira `https://<api>.vercel.app/healthz` → `{"db":"up","status":"ok"}`.

`PUBLIC_API_URL` aponta para o **front** de propósito: o login da Steam volta por `/api/auth/steam/callback` no domínio do site, e o cookie de sessão fica nesse domínio.

## 3. Front (projeto `viceskins`)

1. Novo projeto, mesmo repositório, **Root Directory: `web`** (preset Next.js).
2. Variável `API_INTERNAL_URL` = `https://<api>.vercel.app`. Ela é lida **no build** (destino do proxy `/api`): se mudar, faça redeploy.
3. Deploy, e volte na API para conferir se `PUBLIC_API_URL` e `AUTH_REDIRECT_URL` usam este domínio.

## 4. Cron (cron-job.org, grátis)

Crie um job em https://cron-job.org:

- URL: `https://<api>.vercel.app/internal/jobs/run`
- Método: `POST`, a cada 1 minuto
- Header: `Authorization: Bearer <JOBS_SECRET>`

A resposta mostra o resultado de cada job (`"ok"` ou `"error"`; o detalhe fica nos logs da Vercel). Cada rodada tem 25 s: o que não couber fica para a próxima.

## 5. Teste de fumaça

1. Entrar com a Steam (o inventário precisa estar público) e cadastrar a trade URL no perfil.
2. Depositar por Pix e clicar em **Simular pagamento**.
3. Anunciar um item com uma conta, comprar com a outra e seguir a troca real na Steam.

## O que observar

- **Login:** a Vercel precisa devolver o `Set-Cookie` e o redirecionamento da API através do proxy `/api`. No Next local funciona; se na Vercel o login voltar sem sessão, é aqui.
- **Inventário da Steam:** o endpoint de inventário tem rate limit agressivo, e os IPs da Vercel são compartilhados entre clientes. Se a sincronização responder "a Steam está limitando as requisições", é isso, não bug do código. O cache no banco e o cooldown de 5 min ajudam.
- **Limites por IP:** atrás da Vercel, a API vê o IP do proxy, não o do usuário. Por isso o limite de compra, depósito e API key é **por usuário**, e o limite por IP é alto.
- **Plano Hobby** da Vercel é para uso pessoal e não comercial. Um portfólio com dinheiro fictício se encaixa.
