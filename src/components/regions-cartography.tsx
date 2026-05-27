"use client";

import { useMemo } from "react";

import { tagHref } from "@/lib/tags";
import type { Rate, RegionWithMetrics } from "@/lib/regions";

const VIEWBOX_WIDTH = 600;
const VIEWBOX_HEIGHT = 400;
const MARGIN = { top: 28, right: 24, bottom: 36, left: 48 };
const PLOT_WIDTH = VIEWBOX_WIDTH - MARGIN.left - MARGIN.right;
const PLOT_HEIGHT = VIEWBOX_HEIGHT - MARGIN.top - MARGIN.bottom;

const TOP_LABEL_COUNT = 8;

type Props = {
  regions: RegionWithMetrics[];
  windowLabel: string;
};

export function RegionsCartography({ regions, windowLabel }: Props) {
  const flowAvailable = regions.some(
    (entry) => entry.metrics.growth != null || entry.metrics.closure != null
  );

  const points = useMemo(() => computePoints(regions), [regions]);

  if (!flowAvailable) {
    return (
      <div className="app-surface rounded-[1.5rem] p-8 text-center text-sm text-muted-foreground">
        Cartography needs a finite window. Select 7d / 30d / 90d / 180d / 365d to view
        regions plotted by mass and net daily flow.
      </div>
    );
  }

  if (points.length === 0) {
    return (
      <div className="app-surface rounded-[1.5rem] p-8 text-center text-sm text-muted-foreground">
        No regions to plot for window {windowLabel}.
      </div>
    );
  }

  const maxX = Math.max(...points.map((p) => p.x));
  const yExtent = Math.max(
    0.001,
    Math.max(...points.map((p) => Math.abs(p.y)))
  );

  const xScale = (x: number) => (x / Math.max(maxX, 0.001)) * PLOT_WIDTH;
  const yScale = (y: number) => PLOT_HEIGHT / 2 - (y / yExtent) * (PLOT_HEIGHT / 2 - 8);

  const topByMass = [...points]
    .sort((a, b) => b.mass - a.mass)
    .slice(0, TOP_LABEL_COUNT)
    .map((p) => p.id);
  const topLabelSet = new Set(topByMass);

  return (
    <div className="app-surface rounded-[1.5rem] p-5">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">Cartography</h3>
        <p className="text-[11px] text-muted-foreground">
          x: log10(mass + 1) · y: growth − closure per day · window {windowLabel}
        </p>
      </div>

      <svg
        viewBox={`0 0 ${VIEWBOX_WIDTH} ${VIEWBOX_HEIGHT}`}
        className="h-auto w-full"
        role="img"
        aria-label="Region cartography scatter plot"
      >
        <g transform={`translate(${MARGIN.left} ${MARGIN.top})`}>
          {/* Axes */}
          <line
            x1={0}
            y1={PLOT_HEIGHT / 2}
            x2={PLOT_WIDTH}
            y2={PLOT_HEIGHT / 2}
            stroke="currentColor"
            strokeOpacity={0.18}
            strokeDasharray="2 4"
          />
          <line
            x1={0}
            y1={0}
            x2={0}
            y2={PLOT_HEIGHT}
            stroke="currentColor"
            strokeOpacity={0.18}
          />
          <line
            x1={0}
            y1={PLOT_HEIGHT}
            x2={PLOT_WIDTH}
            y2={PLOT_HEIGHT}
            stroke="currentColor"
            strokeOpacity={0.18}
          />

          {/* Quadrant annotations */}
          <QuadrantLabel x={PLOT_WIDTH - 4} y={12} anchor="end">
            growing &amp; large
          </QuadrantLabel>
          <QuadrantLabel x={4} y={12} anchor="start">
            growing &amp; small
          </QuadrantLabel>
          <QuadrantLabel x={PLOT_WIDTH - 4} y={PLOT_HEIGHT - 4} anchor="end">
            shrinking &amp; large
          </QuadrantLabel>
          <QuadrantLabel x={4} y={PLOT_HEIGHT - 4} anchor="start">
            dormant
          </QuadrantLabel>

          {/* Bubbles */}
          {points.map((p) => {
            const cx = xScale(p.x);
            const cy = yScale(p.y);
            const showLabel = topLabelSet.has(p.id);
            return (
              <a
                key={p.id}
                href={tagHref(p.id)}
                data-testid={`region-bubble-${p.id}`}
                data-fill={p.fill}
                className="group cursor-pointer"
              >
                <title>
                  {p.id} — mass {p.mass}, net {p.y.toFixed(2)}/day
                </title>
                <circle
                  cx={cx}
                  cy={cy}
                  r={p.r}
                  fill={p.fill}
                  fillOpacity={0.55}
                  stroke={p.fill}
                  strokeOpacity={0.9}
                  strokeWidth={1.25}
                  className="transition-all group-hover:opacity-100"
                />
                {showLabel && (
                  <text
                    x={cx + p.r + 4}
                    y={cy + 3}
                    fontSize={10}
                    className="fill-foreground/70"
                  >
                    {p.id}
                  </text>
                )}
              </a>
            );
          })}
        </g>

        {/* Axis labels */}
        <text
          x={VIEWBOX_WIDTH - MARGIN.right}
          y={VIEWBOX_HEIGHT - 8}
          textAnchor="end"
          fontSize={10}
          className="fill-muted-foreground"
        >
          Mass →
        </text>
        <text
          x={MARGIN.left}
          y={MARGIN.top - 12}
          textAnchor="start"
          fontSize={10}
          className="fill-muted-foreground"
        >
          ↑ Net daily flow
        </text>
      </svg>
    </div>
  );
}

function QuadrantLabel({
  x,
  y,
  anchor,
  children,
}: {
  x: number;
  y: number;
  anchor: "start" | "end";
  children: React.ReactNode;
}) {
  return (
    <text
      x={x}
      y={y}
      textAnchor={anchor}
      fontSize={9}
      className="fill-muted-foreground/60"
    >
      {children}
    </text>
  );
}

type Point = {
  id: string;
  x: number;
  y: number;
  mass: number;
  r: number;
  fill: string;
};

const FILL_GROWING = "#16a34a"; // green-600
const FILL_SHRINKING = "#dc2626"; // red-600
const FILL_FLAT = "#64748b"; // slate-500

function computePoints(regions: RegionWithMetrics[]): Point[] {
  return regions.map((entry) => {
    const mass = entry.metrics.mass;
    const x = Math.log10(mass + 1);
    const growth = ratePerDay(entry.metrics.growth);
    const closure = ratePerDay(entry.metrics.closure);
    const net = growth - closure;
    const r = Math.max(5, Math.min(22, Math.sqrt(mass) * 3));
    return {
      id: entry.region.key.id,
      x,
      y: net,
      mass,
      r,
      fill: net > 0.01 ? FILL_GROWING : net < -0.01 ? FILL_SHRINKING : FILL_FLAT,
    };
  });
}

function ratePerDay(rate?: Rate | null): number {
  return rate ? rate.perDay : 0;
}
