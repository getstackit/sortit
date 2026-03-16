"use client";

import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { ClosedFactorAttributionChart } from "@/components/closed-factor-attribution-chart";
import { SiteHeader } from "@/components/site-header";
import { useIssues } from "@/hooks/use-issues";

export function AnalyticsPage() {
  const {
    data: closedIssues,
    error: closedIssuesError,
    isLoading: closedIssuesLoading,
  } = useIssues("closed");

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="Analytics"
        eyebrow="Insights"
        subtitle="Closure trends and factor attribution over time"
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="flex w-full flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          <section>
            <h2 className="text-lg font-semibold tracking-tight">Closed Factor Timeline</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Factor attribution across tickets as they close, shown as a stacked time series of closed issues.
            </p>

            {closedIssuesLoading && (
              <div className="mt-4 text-sm text-muted-foreground">Loading...</div>
            )}

            {closedIssuesError && (
              <div className="app-status-warning mt-4 text-sm">
                Failed to load closed issue timeline: {closedIssuesError.message}
              </div>
            )}

            {!closedIssuesLoading && !closedIssuesError && (
              <div className="mt-4">
                <ClosedFactorAttributionChart issues={closedIssues ?? []} />
              </div>
            )}
          </section>
        </div>
      </div>
    </AppShell>
  );
}
