import { useId, type SVGProps } from "react";
import { cn } from "@/lib/utils";

type Props = SVGProps<SVGSVGElement> & { width?: number; height?: number; x?: number; y?: number; strokeDasharray?: string };

// SVG line grid that fills its positioned parent.
export function GridPattern({ width = 40, height = 40, x = -1, y = -1, strokeDasharray = "0", className, ...props }: Props) {
  const id = useId();
  return (
    <svg aria-hidden className={cn("pointer-events-none absolute inset-0 h-full w-full stroke-foreground/10", className)} {...props}>
      <defs>
        <pattern id={id} width={width} height={height} patternUnits="userSpaceOnUse" x={x} y={y}>
          <path d={`M.5 ${height}V.5H${width}`} fill="none" strokeDasharray={strokeDasharray} />
        </pattern>
      </defs>
      <rect width="100%" height="100%" strokeWidth={0} fill={`url(#${id})`} />
    </svg>
  );
}

// dashed grid behind navbar + page top; lower part blurs and fades into the page
export function GridBackdrop() {
  return (
    <div aria-hidden className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[640px] overflow-hidden">
      <GridPattern
        width={64}
        height={64}
        strokeDasharray="6 4"
        className="[mask-image:radial-gradient(70%_80%_at_50%_0%,white,transparent)]"
      />
      <div className="absolute inset-x-0 bottom-0 h-2/3 backdrop-blur-[3px] [mask-image:linear-gradient(to_bottom,transparent,black)]" />
      <div className="absolute inset-x-0 bottom-0 h-1/3 bg-gradient-to-b from-transparent to-background" />
    </div>
  );
}
