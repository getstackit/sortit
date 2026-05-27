"use client";

import { ActivityIcon } from "lucide-react";

import { useRegion } from "@/hooks/use-region";
import type { Rate, RegionKind } from "@/lib/regions";

type Props = {
  kind: RegionKind;
  id: string;
};

const BUCKET_ORDER = ["<1w", "1-4w", "1-3m", "3m+"] as const;

export function RegionMetricsSection({ kind, id }: Props) {
  const { data, error, isLoading } = useRegion(kind, id);

  if (isLoading) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <RegionMetricsHeader />
        <p className="mt-4 text-sm text-muted-foreground">Loading region metrics…</p>
      </section>
    );
  }

  if (error) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <RegionMetricsHeader />
        <p className="mt-4 text-sm text-muted-foreground">
          Failed to load region metrics: {error.message}
        </p>
      </section>
    );
  }

  if (!data) {
    return null;
  }

  const { metrics } = data;
  const buckets = metrics.ageBuckets ?? [];
  const orderedBuckets = BUCKET_ORDER.map(
    (label) => buckets.find((b) => b.label === label) ?? { label, count: 0 }
  );
  const maxCount = Math.max(1, ...orderedBuckets.map((b) => b.count));

  return (
    <section className="app-surface rounded-[1.5rem] p-5">
      <RegionMetricsHeader />

      <div className="mt-4 grid grid-cols-2 gap-3">
        <div className="app-subtle-surface rounded-[1.25rem] px-3 py-2.5">
          <p className="text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
            Open at floor
          </p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{metrics.massOpen}</p>
        </div>
        <div className="app-subtle-surface rounded-[1.25rem] px-3 py-2.5">
          <p className="text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
            Closed at floor
          </p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{metrics.massClosed}</p>
        </div>
      </div>

      {metrics.massOpen > 0 && orderedBuckets.some((b) => b.count > 0) && (
        <div className="mt-4">
          <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
            Open issues by age
          </p>
          <ol className="mt-2 space-y-1.5">
            {orderedBuckets.map((bucket) => {
              const widthPct = (bucket.count / maxCount) * 100;
              return (
                <li key={bucket.label} className="flex items-center gap-2 text-xs">
                  <span className="w-10 text-muted-foreground tabular-nums">{bucket.label}</span>
                  <div className="relative h-2 flex-1 rounded-full bg-border">
                    <div
                      className="absolute inset-y-0 left-0 rounded-full bg-primary/60"
                      style={{ width: `${widthPct}%` }}
                    />
                  </div>
                  <span className="w-6 text-right tabular-nums text-muted-foreground">
                    {bucket.count}
                  </span>
                </li>
              );
            })}
          </ol>
        </div>
      )}

      <div className="mt-4 grid grid-cols-3 gap-3">
        <FlowTile
          label={`Growth (${metrics.window.label})`}
          rate={metrics.growth}
        />
        <FlowTile
          label={`Closure (${metrics.window.label})`}
          rate={metrics.closure}
        />
        <FlowTile
          label={`Churn (${metrics.window.label})`}
          rate={metrics.churn}
        />
      </div>

      <p className="mt-4 text-[11px] text-muted-foreground">
        Membership uses a 0.4 relevance floor (the verifier&apos;s keep threshold), so these
        counts can be lower than &ldquo;issues tagged&rdquo;.
      </p>
    </section>
  );
}

function RegionMetricsHeader() {
  return (
    <div className="flex items-center gap-2">
      <ActivityIcon className="size-4 text-muted-foreground" />
      <h3 className="text-sm font-semibold">Region metrics</h3>
    </div>
  );
}

function FlowTile({ label, rate }: { label: string; rate?: Rate | null }) {
  return (
    <div className="app-subtle-surface rounded-[1.25rem] px-3 py-2.5">
      <p className="text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
        {label}
      </p>
      {rate ? (
        <>
          <p className="mt-1 text-xl font-semibold tabular-nums">{rate.count}</p>
          <p className="text-[10px] tabular-nums text-muted-foreground">
            {rate.perDay.toFixed(2)}/day
          </p>
        </>
      ) : (
        <p className="mt-1 text-xl font-semibold tabular-nums text-muted-foreground">—</p>
      )}
    </div>
  );
}
