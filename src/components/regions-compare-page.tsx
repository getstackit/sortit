"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useMemo } from "react";
import { ArrowLeftRightIcon } from "lucide-react";

import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { TagBadge } from "@/components/tag-badge";
import { useRegion } from "@/hooks/use-region";
import type {
  Rate,
  RegionGetResponse,
  RegionKind,
  RegionMetrics,
} from "@/lib/regions";

type ParsedKey = {
  kind: RegionKind;
  id: string;
};

function parseKeyParam(raw: string | null): ParsedKey | null {
  if (!raw) return null;
  const trimmed = raw.trim();
  if (!trimmed) return null;
  const slash = trimmed.indexOf("/");
  if (slash <= 0 || slash === trimmed.length - 1) return null;
  const kind = trimmed.slice(0, slash);
  const id = trimmed.slice(slash + 1);
  if (kind !== "tag" && kind !== "cluster") return null;
  return { kind, id };
}

function serializeKey(key: ParsedKey): string {
  return `${key.kind}/${key.id}`;
}

export function RegionsComparePage() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const searchParamsString = searchParams?.toString() ?? "";

  const keyA = useMemo(
    () => parseKeyParam(new URLSearchParams(searchParamsString).get("a")),
    [searchParamsString],
  );
  const keyB = useMemo(
    () => parseKeyParam(new URLSearchParams(searchParamsString).get("b")),
    [searchParamsString],
  );

  const responseA = useRegion(keyA?.kind ?? null, keyA?.id ?? null);
  const responseB = useRegion(keyB?.kind ?? null, keyB?.id ?? null);

  const handleSwap = useCallback(() => {
    if (!keyA || !keyB) return;
    const params = new URLSearchParams(searchParamsString);
    params.set("a", serializeKey(keyB));
    params.set("b", serializeKey(keyA));
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }, [keyA, keyB, pathname, router, searchParamsString]);

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="Compare regions"
        eyebrow="Tier 1"
        subtitle="Side-by-side metrics for two regions in the current corpus."
        actions={
          <div className="flex items-center gap-2">
            {keyA && keyB && (
              <button
                type="button"
                onClick={handleSwap}
                className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              >
                <ArrowLeftRightIcon className="size-3.5" />
                Swap
              </button>
            )}
            <Link
              href="/regions"
              className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              Back to regions
            </Link>
          </div>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="flex w-full flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          {(!keyA || !keyB) && <MissingParamsHelp />}

          {keyA && keyB && (
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-[1fr_1fr_minmax(0,18rem)]">
              <RegionColumn label="A" parsed={keyA} response={responseA} />
              <RegionColumn label="B" parsed={keyB} response={responseB} />
              <DeltaColumn
                a={responseA.data?.metrics ?? null}
                b={responseB.data?.metrics ?? null}
              />
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}

function MissingParamsHelp() {
  return (
    <div className="app-surface rounded-[1.5rem] p-5">
      <h3 className="text-sm font-semibold">Pick two regions to compare</h3>
      <p className="mt-2 text-xs text-muted-foreground">
        Compare expects two URL params: <code>?a=tag/auth&amp;b=tag/billing</code> or{" "}
        <code>?a=cluster/c-abc&amp;b=cluster/c-def</code>. Each side uses{" "}
        <code>kind/id</code> separated by a slash.
      </p>
    </div>
  );
}

function RegionColumn({
  label,
  parsed,
  response,
}: {
  label: string;
  parsed: ParsedKey;
  response: ReturnType<typeof useRegion>;
}) {
  if (response.isLoading) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <Header label={label} parsed={parsed} title={parsed.id} />
        <p className="mt-4 text-sm text-muted-foreground">Loading…</p>
      </section>
    );
  }

  if (response.error) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <Header label={label} parsed={parsed} title={parsed.id} />
        <p className="mt-4 text-sm text-muted-foreground">
          Failed to load: {response.error.message}
        </p>
      </section>
    );
  }

  const data = response.data;
  if (!data || data.metrics.mass === 0) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <Header label={label} parsed={parsed} title={parsed.id} />
        <p className="mt-4 text-sm text-muted-foreground">
          No region with that key — it may belong to an older corpus snapshot.
        </p>
      </section>
    );
  }

  return (
    <section className="app-surface rounded-[1.5rem] p-5">
      <Header label={label} parsed={parsed} title={data.region.label} />
      <MetricsBlock metrics={data.metrics} response={data} />
    </section>
  );
}

function Header({
  label,
  parsed,
  title,
}: {
  label: string;
  parsed: ParsedKey;
  title: string;
}) {
  return (
    <div className="flex items-start justify-between gap-2">
      <div>
        <p className="text-[10px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
          {label} · {parsed.kind}
        </p>
        <div className="mt-1">
          {parsed.kind === "tag" ? (
            <TagBadge tag={title} />
          ) : (
            <span className="font-mono text-xs text-foreground">{title}</span>
          )}
        </div>
      </div>
      <span className="font-mono text-[10px] text-muted-foreground">{parsed.id}</span>
    </div>
  );
}

function MetricsBlock({
  metrics,
  response,
}: {
  metrics: RegionMetrics;
  response: RegionGetResponse;
}) {
  void response;
  return (
    <>
      <div className="mt-4 grid grid-cols-3 gap-3">
        <Cell label="Open" value={metrics.massOpen} />
        <Cell label="Closed" value={metrics.massClosed} />
        <Cell label="Mass" value={metrics.mass} />
      </div>
      <div className="mt-4 grid grid-cols-3 gap-3">
        <FlowCell label={`Growth (${metrics.window.label})`} rate={metrics.growth} />
        <FlowCell label={`Closure (${metrics.window.label})`} rate={metrics.closure} />
        <FlowCell label={`Churn (${metrics.window.label})`} rate={metrics.churn} />
      </div>
      {metrics.density != null && (
        <p className="mt-4 text-[11px] tabular-nums text-muted-foreground">
          density {Math.round(metrics.density * 100)}%
        </p>
      )}
    </>
  );
}

function Cell({ label, value }: { label: string; value: number }) {
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

function DeltaColumn({
  a,
  b,
}: {
  a: RegionMetrics | null;
  b: RegionMetrics | null;
}) {
  if (!a || !b) {
    return (
      <section className="app-surface rounded-[1.5rem] p-5">
        <h3 className="text-sm font-semibold">Δ B − A</h3>
        <p className="mt-3 text-xs text-muted-foreground">
          Both sides need to resolve before deltas appear.
        </p>
      </section>
    );
  }

  const massDelta = b.mass - a.mass;
  const openDelta = b.massOpen - a.massOpen;
  const closedDelta = b.massClosed - a.massClosed;
  const growthDelta = (b.growth?.perDay ?? 0) - (a.growth?.perDay ?? 0);
  const closureDelta = (b.closure?.perDay ?? 0) - (a.closure?.perDay ?? 0);
  const churnDelta = (b.churn?.perDay ?? 0) - (a.churn?.perDay ?? 0);

  return (
    <section className="app-surface rounded-[1.5rem] p-5">
      <h3 className="text-sm font-semibold">Δ B − A</h3>
      <ul className="mt-4 space-y-2 text-xs">
        <DeltaRow label="Mass" value={massDelta} />
        <DeltaRow label="Open" value={openDelta} />
        <DeltaRow label="Closed" value={closedDelta} />
        <DeltaRow label="Growth/day" value={growthDelta} fractional />
        <DeltaRow label="Closure/day" value={closureDelta} fractional />
        <DeltaRow label="Churn/day" value={churnDelta} fractional />
      </ul>
    </section>
  );
}

function DeltaRow({
  label,
  value,
  fractional = false,
}: {
  label: string;
  value: number;
  fractional?: boolean;
}) {
  const text = fractional ? value.toFixed(2) : value.toString();
  const sign = value > 0 ? "+" : "";
  const color =
    value > 0
      ? "text-green-600 dark:text-green-400"
      : value < 0
        ? "text-red-600 dark:text-red-400"
        : "text-muted-foreground";
  return (
    <li className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className={`tabular-nums ${color}`}>
        {sign}
        {text}
      </span>
    </li>
  );
}
