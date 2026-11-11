import { cn } from "@/lib/utils";

// official marks from each project's repo / on-chain ft_metadata, keyed by protocol (venue ids in earn/sources)
const MARK: Record<string, string> = {
  rhea: "rhea.png",
  "rhea-lending": "rhea-lending.png",
  "rhea-dex": "rhea-dex.svg",
  linear: "linear.svg",
  intents: "near-intents.png",
};
// square app-icon marks: a circle crop would cut their corners
const SQUARE = new Set(["rhea-dex", "rhea-lending", "intents"]);

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
      className={cn("shrink-0 object-contain", SQUARE.has(protocol) ? "rounded-[22%]" : "rounded-full", className)}
    />
  );
}
