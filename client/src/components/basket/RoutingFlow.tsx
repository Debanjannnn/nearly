"use client";

import { useEffect, useMemo, useRef } from "react";
import {
  BaseEdge,
  Handle,
  Position,
  ReactFlow,
  ReactFlowProvider,
  getSmoothStepPath,
  useReactFlow,
  type Edge,
  type EdgeProps,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { ArrowDownUp, Landmark, Layers, Receipt, Wallet, type LucideIcon } from "lucide-react";
import { fmtNum, pct } from "@/lib/api";
import { FEE_BPS, TOTAL_BPS, WNEAR_POOL } from "@/lib/config";
import { symbolOf } from "@/lib/tokens";
import type { Leg, Rates, Source } from "@/lib/types";
import { cn } from "@/lib/utils";
import { TokenIcon } from "@/components/TokenIcon";
import { legApy, SOURCE_LABEL, VENUE_NAME } from "./legs";

const ICON: Record<Source | "swap" | "fee", LucideIcon> = { lending: Landmark, staking: Layers, hold: Wallet, swap: ArrowDownUp, fee: Receipt };

type CardData = { [k: string]: unknown; title: string; sub?: string; positive?: boolean; token?: string; icon?: keyof typeof ICON; ends?: "left" | "right" };
type CardNode = Node<CardData, "card">;

function Card({ data }: NodeProps<CardNode>) {
  const Icon = data.icon ? ICON[data.icon] : null;
  return (
    <div className={cn("flex h-full items-center gap-2.5 rounded-xl border px-3", data.ends === "left" ? "border-primary/40 bg-primary/5" : "border-border bg-card")}>
      {data.ends !== "left" && <Handle type="target" position={Position.Left} className="!opacity-0" />}
      {data.token && <TokenIcon token={data.token} size={22} />}
      {Icon && (
        <span className="grid size-[22px] shrink-0 place-items-center rounded-full bg-secondary text-muted-foreground">
          <Icon className="size-3" />
        </span>
      )}
      <div className="min-w-0">
        <div className="truncate text-sm leading-tight font-medium">{data.title}</div>
        {data.sub && <div className={cn("tnum mt-1 truncate text-[11px] leading-tight", data.positive ? "text-positive" : "text-muted-foreground")}>{data.sub}</div>}
      </div>
      {data.ends !== "right" && <Handle type="source" position={Position.Right} className="!opacity-0" />}
    </div>
  );
}

function FanEdge({ id, sourceX, sourceY, targetX, targetY, style }: EdgeProps) {
  const [path] = getSmoothStepPath({ sourceX, sourceY, targetX, targetY, sourcePosition: Position.Right, targetPosition: Position.Left, borderRadius: 12 });
  return <BaseEdge id={id} path={path} style={style} />;
}

const nodeTypes = { card: Card };
const edgeTypes = { fan: FanEdge };

// uniform height: unequal heights bow same-row edges
const ROW = 72;
const HEIGHT = 56;
const WIDE = { width: [150, 180, 190], col: [0, 190, 410] };
const COMPACT = { width: [120, 160, 168], col: [0, 136, 314] };

const fmtAmt = (n: number) => fmtNum(n, n >= 100 ? 2 : 4);

// usdc in → open fee → per token (kept or swapped on Rhea) → per leg venue; amount in USDC
export function RoutingFlow({ amount, legs, rates, compact = false, fill = false }: { amount: number; legs: Leg[]; rates: Rates | null | undefined; compact?: boolean; fill?: boolean }) {
  const { nodes, edges, rows } = useMemo(() => {
    const { width: W, col: C } = compact ? COMPACT : WIDE;
    const nodes: CardNode[] = [];
    const edges: Edge[] = [];
    const node = (id: string, x: number, y: number, w: number, data: CardData): CardNode => ({ id, type: "card", position: { x, y }, data, style: { width: w, height: HEIGHT }, draggable: false });
    const edge = (source: string, target: string): Edge => ({ id: `${source}>${target}`, source, target, type: "fan", animated: true });
    const net = amount * (1 - FEE_BPS / TOTAL_BPS);

    nodes.push(node("fee", C[1], 0, W[1], { title: "Open fee", sub: `${FEE_BPS / 100}% · $${fmtAmt(amount - net)} → treasury`, icon: "fee", ends: "right" }));
    edges.push(edge("in", "fee"));
    let row = 1;
    for (const token of [...new Set(legs.map((l) => l.token))]) {
      const mine = legs.filter((l) => l.token === token);
      const share = mine.reduce((a, l) => a + l.weight_bps, 0) / TOTAL_BPS;
      const tid = `tok:${token}`;
      nodes.push(
        node(tid, C[1], (row + (mine.length - 1) / 2) * ROW, W[1], {
          title: token === "usdc" ? "Keep USDC" : `Swap to ${symbolOf(token)}`,
          sub: token === "usdc" ? `$${fmtAmt(net * share)}` : `Rhea pool ${WNEAR_POOL} · $${fmtAmt(net * share)}`,
          ...(token === "usdc" ? { token } : { icon: "swap" as const }),
        }),
      );
      edges.push(edge("in", tid));
      mine.forEach((l) => {
        const id = `${tid}:${l.source}`;
        const apy = legApy(l, rates);
        nodes.push(
          node(id, C[2], row * ROW, W[2], {
            title: VENUE_NAME[l.source],
            sub: `${SOURCE_LABEL[l.source]} · ${l.source === "hold" ? "no yield" : `${apy === null ? "-" : pct(apy)} APY`}`,
            positive: l.source !== "hold" && apy !== null,
            icon: l.source,
            ends: "right",
          }),
        );
        edges.push(edge(tid, id));
        row++;
      });
    }
    nodes.unshift(node("in", C[0], ((row - 1) * ROW) / 2, W[0], { title: compact ? "Account" : "Your account", sub: `$${fmtAmt(amount)} USDC`, token: "usdc", ends: "left" }));
    return { nodes, edges, rows: row };
  }, [amount, legs, rates, compact]);

  if (legs.length === 0) return <div className="flex h-40 items-center justify-center px-6 text-center text-sm text-muted-foreground">Add a leg to see where funds go</div>;

  return (
    <ReactFlowProvider>
      <Diagram nodes={nodes} edges={edges} height={fill ? "100%" : rows * ROW + 24} zoom={fill ? 1.6 : 1} />
    </ReactFlowProvider>
  );
}

// refit on resize: panel can mount before its column settles
function Diagram({ nodes, edges, height, zoom }: { nodes: CardNode[]; edges: Edge[]; height: number | string; zoom: number }) {
  const box = useRef<HTMLDivElement>(null);
  const { fitView } = useReactFlow();

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(() => void fitView({ padding: 0.06, maxZoom: zoom, duration: 150 }));
    ro.observe(el);
    return () => ro.disconnect();
  }, [fitView, zoom]);

  useEffect(() => {
    void fitView({ padding: 0.06, maxZoom: zoom, duration: 150 });
  }, [nodes.length, fitView, zoom]);

  return (
    <div ref={box} className="px-2 pb-2" style={{ height }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        fitView
        fitViewOptions={{ padding: 0.06, maxZoom: zoom }}
        minZoom={0.3}
        maxZoom={zoom}
        proOptions={{ hideAttribution: true }}
        nodesConnectable={false}
        elementsSelectable={false}
        panOnDrag={false}
        zoomOnScroll={false}
        zoomOnPinch={false}
        zoomOnDoubleClick={false}
        preventScrolling={false}
      />
    </div>
  );
}
