export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly body: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }

  get retryAfterSeconds(): number | undefined {
    const b = this.body as { retryAfterSeconds?: unknown } | null;
    return typeof b?.retryAfterSeconds === "number" ? b.retryAfterSeconds : undefined;
  }
}

/** Chama a API Go através do proxy /api do Next. */
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });

  if (res.status === 204) return undefined as T;

  const body: unknown = await res.json().catch(() => null);
  if (!res.ok) {
    const message =
      (body as { error?: string } | null)?.error ?? `Erro ${res.status}`;
    throw new ApiError(res.status, message, body);
  }
  return body as T;
}

export const isApiError = (e: unknown, status?: number): e is ApiError =>
  e instanceof ApiError && (status === undefined || e.status === status);
