"use client";

import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ForceGraphMethods, NodeObject } from "react-force-graph-2d";
import { forceX, forceY } from "d3-force";
import type { VaultGraph } from "@multica/core/vault";

// react-force-graph-2d touches `window`/canvas at import time, so defer the
// import to the client via React.lazy (packages/views can't use next/dynamic).
// During SSR the Suspense fallback renders; the chunk loads on hydration.
const ForceGraph2D = lazy(() => import("react-force-graph-2d"));

type GraphNode = NodeObject & { id: string; title: string };

/**
 * Obsidian-style force-directed graph of the vault: one node per note, one
 * link per resolved [[wikilink]]. Clicking a node opens that note. Colors are
 * read from the theme's CSS custom properties so the canvas tracks light/dark.
 */
export function VaultGraphView({
  graph,
  onSelect,
}: {
  graph: VaultGraph;
  onSelect: (path: string) => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods | undefined>(undefined);
  const configured = useRef(false);
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [colors, setColors] = useState({ node: "#6366f1", link: "#3f3f46", text: "#a1a1aa" });

  // Tune the simulation to an Obsidian-like layout: gentle center gravity
  // (forceX/forceY) so orphan notes pull inward instead of being flung to the
  // edges, capped repulsion range, and shorter links so clusters stay tight.
  // Set on the first engine tick — by then the lazily-loaded canvas has mounted
  // and populated the ref; the guard makes it a one-shot.
  const tuneForces = useCallback(() => {
    if (configured.current) return;
    const fg = fgRef.current;
    if (!fg) return;
    configured.current = true;
    fg.d3Force("charge")?.strength(-60).distanceMax(220);
    fg.d3Force("link")?.distance(30);
    fg.d3Force("x", forceX(0).strength(0.07));
    fg.d3Force("y", forceY(0).strength(0.07));
    fg.d3ReheatSimulation();
  }, []);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const measure = () => setSize({ width: el.clientWidth, height: el.clientHeight });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    const cs = getComputedStyle(el);
    const v = (name: string, fallback: string) => cs.getPropertyValue(name).trim() || fallback;
    setColors({
      node: v("--primary", "#6366f1"),
      link: v("--border", "#3f3f46"),
      text: v("--muted-foreground", "#a1a1aa"),
    });
    return () => ro.disconnect();
  }, []);

  // ForceGraph mutates node/link objects (adds x/y/vx/vy). Clone so the
  // TanStack Query cache object is never frozen-mutated. Re-clone on identity.
  const data = useMemo(
    () => ({
      nodes: graph.nodes.map((n) => ({ ...n })),
      links: graph.links.map((l) => ({ ...l })),
    }),
    [graph],
  );

  return (
    <div ref={containerRef} className="relative h-full w-full overflow-hidden">
      <Suspense fallback={<p className="p-6 text-sm text-muted-foreground">Loading graph…</p>}>
        {size.width > 0 && (
          <ForceGraph2D
            ref={fgRef}
            graphData={data}
            width={size.width}
            height={size.height}
            nodeId="id"
            nodeRelSize={4}
            nodeColor={() => colors.node}
            linkColor={() => colors.link}
            linkWidth={1}
            warmupTicks={60}
            cooldownTicks={120}
            onEngineTick={tuneForces}
            onEngineStop={() => fgRef.current?.zoomToFit(400, 40)}
            onNodeClick={(node: NodeObject) => {
              const id = (node as GraphNode).id;
              if (id) onSelect(id);
            }}
            nodeCanvasObjectMode={() => "after"}
            nodeCanvasObject={(node: NodeObject, ctx: CanvasRenderingContext2D, scale: number) => {
              const label = (node as GraphNode).title;
              if (!label || scale < 1.5) return; // declutter when zoomed out
              const fontSize = 12 / scale;
              ctx.font = `${fontSize}px sans-serif`;
              ctx.fillStyle = colors.text;
              ctx.textAlign = "center";
              ctx.textBaseline = "top";
              ctx.fillText(label, node.x ?? 0, (node.y ?? 0) + 5);
            }}
          />
        )}
      </Suspense>
    </div>
  );
}
