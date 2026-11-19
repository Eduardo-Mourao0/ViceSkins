import type { ButtonHTMLAttributes, ReactNode } from "react";

type Variant = "primary" | "secondary" | "ghost" | "danger";

const variants: Record<Variant, string> = {
  primary:
    "bg-pink text-ink-950 font-semibold hover:bg-pink-soft shadow-[0_0_24px_-6px_var(--color-pink)]",
  secondary:
    "border border-cyan/50 text-cyan-soft hover:bg-cyan/10 hover:border-cyan",
  ghost: "text-muted hover:text-text hover:bg-ink-700",
  danger: "border border-danger/50 text-danger hover:bg-danger/10",
};

export function Button({
  variant = "primary",
  className = "",
  loading,
  children,
  disabled,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; loading?: boolean }) {
  return (
    <button
      {...props}
      disabled={disabled || loading}
      className={`inline-flex items-center justify-center gap-2 rounded-lg px-4 py-2 text-sm transition disabled:cursor-not-allowed disabled:opacity-50 ${variants[variant]} ${className}`}
    >
      {loading && <Spinner />}
      {children}
    </button>
  );
}

export function Spinner({ className = "size-4" }: { className?: string }) {
  return (
    <span
      aria-hidden
      className={`inline-block animate-spin rounded-full border-2 border-current border-r-transparent ${className}`}
    />
  );
}

export function SteamIcon({ className = "size-5" }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      className={className}
      aria-hidden
    >
      <circle cx="15.5" cy="8.5" r="3.5" />
      <circle cx="7.5" cy="16" r="2.5" />
      <path d="M9.4 14.3 13 11" />
    </svg>
  );
}

/** Link (não <Link>): o login precisa de navegação completa para ir até a Steam. */
export function SteamLoginButton({ className = "" }: { className?: string }) {
  return (
    <a
      href="/api/auth/steam/login"
      className={`inline-flex items-center gap-2 rounded-lg bg-pink px-4 py-2 text-sm font-semibold text-ink-950 shadow-[0_0_24px_-6px_var(--color-pink)] transition hover:bg-pink-soft ${className}`}
    >
      <SteamIcon />
      Entrar com Steam
    </a>
  );
}

export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="card flex flex-col items-center gap-3 px-6 py-16 text-center">
      <p className="text-lg font-semibold">{title}</p>
      {children && <div className="max-w-md text-sm text-muted">{children}</div>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}

export function Alert({
  tone = "danger",
  children,
}: {
  tone?: "danger" | "warning" | "info";
  children: ReactNode;
}) {
  const tones = {
    danger: "border-danger/40 bg-danger/10 text-[#ffb3c0]",
    warning: "border-warning/40 bg-warning/10 text-[#ffe1a3]",
    info: "border-cyan/40 bg-cyan/10 text-cyan-soft",
  };
  return (
    <div role="alert" className={`rounded-lg border px-4 py-3 text-sm ${tones[tone]}`}>
      {children}
    </div>
  );
}
