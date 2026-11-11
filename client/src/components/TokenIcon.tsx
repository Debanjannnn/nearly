import { symbolOf } from "@/lib/tokens";
import { cn } from "@/lib/utils";

// brand colours by token key; unknown keys get a neutral chip
const COLOR: Record<string, string> = { usdc: "#2775CA", wnear: "#00EC97", near: "#00EC97" };

// token = deployments key ("usdc" | "wnear") or "near"
export function TokenIcon({ token, size = 24, className }: { token: string; size?: number; className?: string }) {
  const sym = token === "near" ? "NEAR" : symbolOf(token);
  const bg = COLOR[token];
  return (
    <span
      role="img"
      aria-label={sym}
      title={sym}
      className={cn("inline-grid shrink-0 place-items-center rounded-full font-semibold", bg ? "text-black" : "bg-secondary", className)}
      style={{ width: size, height: size, fontSize: Math.round(size * 0.42), ...(bg ? { backgroundColor: bg } : {}) }}
    >
      {(sym.replace(/^w/, "") || sym).charAt(0).toUpperCase()}
    </span>
  );
}

export function IconStack({ tokens, size, ring }: { tokens: string[]; size: number; ring: string }) {
  return (
    <span aria-hidden className="flex shrink-0 -space-x-2">
      {tokens.slice(0, 3).map((t, i) => (
        <TokenIcon
          key={`${t}${i}`}
          token={t}
          size={size}
          className={cn("ring-2 transition-transform duration-200 ease-out hover:z-10 hover:-translate-y-1", ring)}
        />
      ))}
    </span>
  );
}
