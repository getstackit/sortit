"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useMemo } from "react";

import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { FilterToggleGroup } from "@/components/filter-toggle";
import { RegionsCartography } from "@/components/regions-cartography";
import { SiteHeader } from "@/components/site-header";
import { TagBadge } from "@/components/tag-badge";
import { useRegions } from "@/hooks/use-regions";
import { tagHref } from "@/lib/tags";
import type {
  AgeBucket,
  CorpusOrphans,
  Rate,
  RegionWindow,
  RegionWithMetrics,
} from "@/lib/regions";
import {
  parseRegionsURLState,
  regionsURLQuery,
  type RegionSortKey,
  type RegionsKindFilter,
  type RegionsView,
} from "@/lib/regions-url-state";

const WINDOW_OPTIONS: readonly RegionWindow[] = [
  "7d",
  "30d",
  "90d",
  "180d",
  "365d",
  "all",
];

const SORT_OPTIONS: readonly RegionSortKey[] = ["mass", "net", "growth", "closure", "name"];

const SORT_LABELS: Record<RegionSortKey, string> = {
  mass: "Mass",
  net: "Net flow",
  growth: "Growth",
  closure: "Closure",
  name: "Name",
};

const VIEW_OPTIONS: readonly RegionsView[] = ["list", "map"];

const VIEW_LABELS: Record<RegionsView, string> = {
  list: "List",
  map: "Cartography",
};

const KIND_OPTIONS: readonly RegionsKindFilter[] = ["tag", "cluster"];

const KIND_LABELS: Record<RegionsKindFilter, string> = {
  tag: "Tags",
  cluster: "Clusters",
};

const BUCKET_ORDER = ["<1w", "1-4w", "1-3m", "3m+"] as const;

export function RegionsPage() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const searchParamsString = searchParams?.toString() ?? "";

  const urlState = useMemo(
    () => parseRegionsURLState(new URLSearchParams(searchParamsString)),
    [searchParamsString]
  );

  const { data, error, isLoading } = useRegions(urlState.kind, urlState.window);

  const updateURL = useCallback(
    (next: Partial<typeof urlState>) => {
      const merged = { ...urlState, ...next };
      const query = regionsURLQuery(merged);
      const target = query ? `${pathname}?${query}` : pathname;
      router.replace(target, { scroll: false });
    },
    [pathname, router, urlState]
  );

  const sortedRegions = useMemo(
    () => sortRegions(data?.regions ?? [], urlState.sort),
    [data?.regions, urlState.sort]
  );

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="Regions"
        eyebrow="Tier 1"
        subtitle="Tagged regions of work and their flow over the selected window."
        meta={
          data ? (
            <span className="text-xs text-muted-foreground tabular-nums">
              {sortedRegions.length} region{sortedRegions.length === 1 ? "" : "s"}
            </span>
          ) : undefined
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <FilterToggleGroup<RegionsKindFilter>
              options={KIND_OPTIONS}
              value={urlState.kind}
              onChange={(kind) => updateURL({ kind })}
              formatLabel={(value) => KIND_LABELS[value]}
            />
            <FilterToggleGroup<RegionsView>
              options={VIEW_OPTIONS}
              value={urlState.view}
              onChange={(view) => updateURL({ view })}
              formatLabel={(value) => VIEW_LABELS[value]}
            />
            <FilterToggleGroup<RegionWindow>
              options={WINDOW_OPTIONS}
              value={urlState.window}
              onChange={(window) => updateURL({ window })}
            />
          </div>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="flex w-full flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          {urlState.view === "list" && (
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                Sort by
              </p>
              <FilterToggleGroup<RegionSortKey>
                options={SORT_OPTIONS}
                value={urlState.sort}
                onChange={(sort) => updateURL({ sort })}
                formatLabel={(value) => SORT_LABELS[value]}
              />
            </div>
          )}

          {isLoading && (
            <p className="text-sm text-muted-foreground">Loading regions…</p>
          )}

          {error && (
            <div className="app-status-warning text-sm">
              Failed to load regions: {error.message}
            </div>
          )}

          {!isLoading && !error && sortedRegions.length === 0 && (
            <p className="text-sm text-muted-foreground">
              No regions yet. Issues need at least one tag scored at relevance &ge; 0.4
              to form a region.
            </p>
          )}

          {data?.orphans && data.orphans.total > 0 && urlState.view === "list" && (
            <OrphansCard orphans={data.orphans} />
          )}

          {sortedRegions.length > 0 && urlState.view === "list" && (
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
              {sortedRegions.map((entry) => (
                <RegionTile key={entry.region.key.id} entry={entry} />
              ))}
            </div>
          )}

          {sortedRegions.length > 0 && urlState.view === "map" && (
            <RegionsCartography
              regions={sortedRegions}
              windowLabel={data?.window.label ?? urlState.window}
            />
          )}
        </div>
      </div>
    </AppShell>
  );
}

function RegionTile({ entry }: { entry: RegionWithMetrics }) {
  const { region, metrics } = entry;
  const isCluster = region.key.kind === "cluster";
  const href = isCluster
    ? `/regions/cluster/${encodeURIComponent(region.key.id)}`
    : tagHref(region.key.id);
  const orderedBuckets = BUCKET_ORDER.map(
    (label) => metrics.ageBuckets?.find((b) => b.label === label) ?? { label, count: 0 }
  );

  return (
    <Link
      href={href}
      className="app-surface block rounded-[1.5rem] p-5 transition-colors hover:bg-accent/30"
    >
      <div className="flex items-center justify-between gap-2">
        {isCluster ? (
          <ClusterTileHeader label={region.label} id={region.key.id} />
        ) : (
          <TagBadge tag={region.label} />
        )}
        <div className="flex items-center gap-2">
          {metrics.density != null && <DensityChip value={metrics.density} />}
          <span className="text-[11px] tabular-nums text-muted-foreground">
            {metrics.mass} total
          </span>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-3 gap-3">
        <MetricCell label="Open" value={metrics.massOpen} />
        <MetricCell label="Closed" value={metrics.massClosed} />
        <MetricCell label="Mass" value={metrics.mass} />
      </div>

      {orderedBuckets.some((b) => b.count > 0) && (
        <AgeBuckets buckets={orderedBuckets} />
      )}

      <div className="mt-4 grid grid-cols-3 gap-3">
        <FlowCell label={`Growth (${metrics.window.label})`} rate={metrics.growth} />
        <FlowCell label={`Closure (${metrics.window.label})`} rate={metrics.closure} />
        <FlowCell label={`Churn (${metrics.window.label})`} rate={metrics.churn} />
      </div>
    </Link>
  );
}

function ClusterTileHeader({ label, id }: { label: string; id: string }) {
  const tags = label.split(/[+,/]/).map((t) => t.trim()).filter(Boolean);
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <div className="flex flex-wrap items-center gap-1">
        {tags.length > 0 ? (
          tags.map((tag) => <TagBadge key={tag} tag={tag} />)
        ) : (
          <span className="text-sm font-medium">{label}</span>
        )}
      </div>
      <span className="font-mono text-[10px] text-muted-foreground">{id}</span>
    </div>
  );
}

function OrphansCard({ orphans }: { orphans: CorpusOrphans }) {
  const pct = Math.round(orphans.fraction * 100);
  return (
    <section className="app-surface rounded-[1.5rem] border-amber-300/40 bg-amber-50/40 p-5 dark:border-amber-500/20 dark:bg-amber-950/20">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-amber-700 dark:text-amber-300">
            Dead zones
          </p>
          <h3 className="mt-1 text-base font-semibold">
            {orphans.total} issue{orphans.total === 1 ? "" : "s"} don&apos;t fit any region
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {orphans.open} currently open · {pct}% of the embedded corpus. These issues
            have no tag at relevance &ge; 0.4 and no nearby tag in embedding space.
          </p>
        </div>
      </div>
    </section>
  );
}

function DensityChip({ value }: { value: number }) {
  const pct = Math.round(value * 100);
  return (
    <span
      title={`Embedding cohesion: ${value.toFixed(3)} (mean cosine sim to centroid)`}
      className="rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[10px] font-medium tabular-nums text-muted-foreground"
    >
      density {pct}%
    </span>
  );
}

function MetricCell({ label, value }: { label: string; value: number }) {
  return (
    <div className="app-subtle-surface rounded-[1.25rem] px-3 py-2.5">
      <p className="text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-1 text-xl font-semibold tabular-nums">{value}</p>
    </div>
  );
}

function FlowCell({ label, rate }: { label: string; rate?: Rate | null }) {
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

function AgeBuckets({ buckets }: { buckets: AgeBucket[] }) {
  const max = Math.max(1, ...buckets.map((b) => b.count));
  return (
    <div className="mt-4">
      <p className="text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
        Open by age
      </p>
      <ol className="mt-2 space-y-1.5">
        {buckets.map((bucket) => (
          <li key={bucket.label} className="flex items-center gap-2 text-xs">
            <span className="w-10 text-muted-foreground tabular-nums">{bucket.label}</span>
            <div className="relative h-2 flex-1 rounded-full bg-border">
              <div
                className="absolute inset-y-0 left-0 rounded-full bg-primary/60"
                style={{ width: `${(bucket.count / max) * 100}%` }}
              />
            </div>
            <span className="w-6 text-right tabular-nums text-muted-foreground">
              {bucket.count}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}

function sortRegions(regions: RegionWithMetrics[], key: RegionSortKey): RegionWithMetrics[] {
  const out = [...regions];
  out.sort((a, b) => {
    switch (key) {
      case "name":
        return a.region.key.id.localeCompare(b.region.key.id);
      case "growth":
        return rateValue(b.metrics.growth) - rateValue(a.metrics.growth);
      case "closure":
        return rateValue(b.metrics.closure) - rateValue(a.metrics.closure);
      case "net":
        return netFlow(b) - netFlow(a);
      case "mass":
      default:
        if (b.metrics.mass !== a.metrics.mass) return b.metrics.mass - a.metrics.mass;
        return a.region.key.id.localeCompare(b.region.key.id);
    }
  });
  return out;
}

function rateValue(rate?: Rate | null): number {
  return rate ? rate.perDay : 0;
}

function netFlow(entry: RegionWithMetrics): number {
  return rateValue(entry.metrics.growth) - rateValue(entry.metrics.closure);
}
