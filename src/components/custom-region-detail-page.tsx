"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import { LayersIcon } from "lucide-react";

import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { TagBadge } from "@/components/tag-badge";
import { useRegion } from "@/hooks/use-region";
import { deleteCustomRegion, getCustomRegion } from "@/lib/custom-regions";

type Props = {
  id: string;
};

export function CustomRegionDetailPage({ id }: Props) {
  const router = useRouter();
  const { data: region, error: regionError, isLoading } = useRegion("custom", id);
  const { data: definition } = useSWR(
    id ? (["custom-region-definition", id] as const) : null,
    () => getCustomRegion(id),
  );

  const handleDelete = async () => {
    if (!confirm("Delete this custom region? Its URL will stop resolving.")) return;
    try {
      await deleteCustomRegion(id);
      router.push("/regions?kind=custom");
    } catch (err) {
      alert(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title={definition?.label || region?.region.label || id}
        eyebrow="Custom region"
        subtitle={definition?.description || "User-defined slice of the corpus."}
        actions={
          <div className="flex items-center gap-2">
            <button
              onClick={handleDelete}
              className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-red-50 hover:text-red-700 dark:hover:bg-red-950/30 dark:hover:text-red-300"
            >
              Delete
            </button>
            <Link
              href="/regions?kind=custom"
              className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              Back to regions
            </Link>
          </div>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="flex w-full flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          {isLoading && <p className="text-sm text-muted-foreground">Loading…</p>}

          {regionError && (
            <div className="app-status-warning text-sm">
              Failed to load: {regionError.message}
            </div>
          )}

          {region && (
            <section className="app-surface rounded-[1.5rem] p-5">
              <div className="flex items-center gap-2">
                <LayersIcon className="size-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">Region metrics</h3>
              </div>

              <div className="mt-4 grid grid-cols-3 gap-3">
                <Cell label="Open" value={region.metrics.massOpen} />
                <Cell label="Closed" value={region.metrics.massClosed} />
                <Cell label="Mass" value={region.metrics.mass} />
              </div>

              {definition && (
                <div className="mt-4 text-xs text-muted-foreground">
                  <p className="text-[10px] font-medium uppercase tracking-[0.14em]">
                    Definition
                  </p>
                  <ul className="mt-2 space-y-1">
                    {definition.definition.tags && definition.definition.tags.length > 0 && (
                      <li>
                        Tags (any of):{" "}
                        {definition.definition.tags.map((t, i) => (
                          <span key={i} className="mr-1 inline-flex items-center gap-1">
                            <TagBadge tag={t.tag} />
                            <span className="tabular-nums">≥ {t.minRelevance.toFixed(2)}</span>
                          </span>
                        ))}
                      </li>
                    )}
                    {definition.definition.statuses && definition.definition.statuses.length > 0 && (
                      <li>Status: {definition.definition.statuses.join(" or ")}</li>
                    )}
                    {definition.definition.maxAgeDays != null && (
                      <li>Max age: {definition.definition.maxAgeDays} days</li>
                    )}
                  </ul>
                </div>
              )}
            </section>
          )}
        </div>
      </div>
    </AppShell>
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
