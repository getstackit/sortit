"use client";

import { useMemo, useState } from "react";
import { Rows3Icon, Rows4Icon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { IssueCard } from "@/components/issue-card";
import { SiteHeader } from "@/components/site-header";
import { Skeleton } from "@/components/ui/skeleton";
import { CompactModeProvider, useCompactMode } from "@/hooks/use-compact-mode";
import {
  IssuesFilterSidebar,
  applyFilter,
  isFilterActive,
  EMPTY_FILTER,
  type IssuesFilter,
} from "@/components/issues-filter-sidebar";
import { useIssues } from "@/hooks/use-issues";
import { Badge } from "@/components/ui/badge";
import type { IssueRecord } from "@/lib/issues";

function CompactToggle() {
  const { isCompact, toggleCompact } = useCompactMode();
  const Icon = isCompact ? Rows4Icon : Rows3Icon;

  return (
    <button
      type="button"
      onClick={toggleCompact}
      title={isCompact ? "Normal view" : "Compact view"}
      className="flex size-8 items-center justify-center rounded-lg border border-border/60 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      <Icon className="size-4" />
    </button>
  );
}

function IssueList({ issues }: { issues: IssueRecord[] }) {
  const { isCompact } = useCompactMode();

  return (
    <div className={isCompact ? "space-y-1 px-4 lg:px-6" : "space-y-2 px-4 lg:px-6"}>
      {issues.map((issue) => (
        <IssueCard
          key={issue.id}
          issue={issue}
          href={`/issues/${issue.id}`}
          compact={isCompact}
        />
      ))}
    </div>
  );
}

export function IssuesBoard() {
  const [filter, setFilter] = useState<IssuesFilter>(EMPTY_FILTER);
  const issueStatus = filter.includeClosed ? "all" : "open";

  const {
    data: issues = [],
    error,
    isLoading: loading,
    mutate: mutateIssues,
  } = useIssues(issueStatus);

  const visibleIssues = isFilterActive(filter)
    ? applyFilter(issues, filter)
    : issues;
  const resultCount = visibleIssues.length;

  const things = useMemo(
    () =>
      visibleIssues.map((issue) => ({
        id: issue.id,
        title: issue.raw.length > 60 ? `${issue.raw.slice(0, 60)}…` : issue.raw,
        href: `/issues/${issue.id}`,
      })),
    [visibleIssues]
  );

  return (
    <CompactModeProvider>
      <AppShell
        sidebar={
          <AppSidebar
            things={things}
            navigateOnCreate={false}
            onIssueCreated={(created) => {
              mutateIssues((prev) => (prev ? [created, ...prev] : [created]), {
                revalidate: false,
              });
            }}
          />
        }
      >
        <SiteHeader
          title="Issues"
          eyebrow="Issues"
          subtitle="Incoming reports, bugs, and ideas waiting to be triaged."
          meta={
            resultCount > 0 ? (
              <Badge variant="outline" className="tabular-nums">
                {resultCount}
              </Badge>
            ) : null
          }
          actions={<CompactToggle />}
        />
        <div className="flex min-h-0 flex-1">
          <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
            <div className="@container/main flex min-h-0 flex-1 flex-col gap-2">
              <div className="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
                {error && (
                  <div className="px-4 lg:px-6">
                    <div className="app-status-warning">
                      Issue backend unavailable: {error?.message}
                    </div>
                  </div>
                )}

                {loading && (
                  <div className="space-y-3 px-4 lg:px-6">
                    {Array.from({ length: 4 }).map((_, index) => (
                      <div
                        key={index}
                        className="app-surface p-5"
                      >
                        <div className="space-y-3">
                          <Skeleton className="h-4 w-3/4" />
                          <Skeleton className="h-4 w-5/6" />
                          <Skeleton className="h-4 w-2/3" />
                        </div>
                        <div className="mt-4 flex items-center gap-2">
                          <Skeleton className="h-6 w-16 rounded-full" />
                          <Skeleton className="h-6 w-14 rounded-full" />
                          <Skeleton className="ml-auto h-4 w-24" />
                        </div>
                      </div>
                    ))}
                  </div>
                )}

                {!loading && issues.length === 0 && (
                  <div className="app-surface mx-4 flex flex-col items-center gap-3 py-20 text-center lg:mx-6">
                    <div className="text-4xl opacity-20">~</div>
                    <p className="text-sm text-muted-foreground/60">
                      Nothing open right now. Drop something in.
                    </p>
                  </div>
                )}

                {!loading && visibleIssues.length === 0 && issues.length > 0 && isFilterActive(filter) && (
                  <div className="app-surface mx-4 flex flex-col items-center gap-3 py-20 text-center lg:mx-6">
                    <div className="text-4xl opacity-20">~</div>
                    <p className="text-sm text-muted-foreground/80">
                      No issues match the current filters.
                    </p>
                    <button
                      type="button"
                      onClick={() => setFilter(EMPTY_FILTER)}
                      className="text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground"
                    >
                      Clear filters
                    </button>
                  </div>
                )}

                {visibleIssues.length > 0 && <IssueList issues={visibleIssues} />}
              </div>
            </div>
          </div>
          <aside className="hidden w-56 shrink-0 overflow-y-auto md:block">
            <IssuesFilterSidebar
              issues={issues}
              filter={filter}
              onFilterChange={setFilter}
            />
          </aside>
        </div>
      </AppShell>
    </CompactModeProvider>
  );
}
