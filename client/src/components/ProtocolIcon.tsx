import { cn } from "@/lib/utils";

// official marks from each project's repo / on-chain ft_metadata, keyed by protocol (venue ids in earn/sources)
const MARK: Record<string, string> = {
  rhea: "rhea.png",
  "rhea-lending": "rhea-lending.png",
  "rhea-dex": "rhea-dex.svg",
  linear: "linear.svg",
  intents: "near-intents.png",
};

export function ProtocolIcon({ protocol, size = 20, className }: { protocol: string; size?: number; className?: string }) {
  const file = MARK[protocol];
  if (!file) return null;
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={`/protocols/${file}`}
      alt={protocol}
      width={size}
      height={size}
      style={{ width: size, height: size }}
      className={cn("shrink-0 rounded-full object-contain", className)}
    />
  );
}
