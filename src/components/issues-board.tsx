"use client";

import { useEffect, useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { IssueCard } from "@/components/issue-card";
import { SiteHeader } from "@/components/site-header";
import { Skeleton } from "@/components/ui/skeleton";
import { fetchIssues, type IssueRecord } from "@/lib/issues";

export function IssuesBoard() {
  const [issues, setIssues] = useState<IssueRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    fetchIssues("open", controller.signal)
      .then((items) => {
        setIssues(items);
        setError(null);
      })
      .catch((caughtError) => {
        if (controller.signal.aborted) {
          return;
        }

        const message =
          caughtError instanceof Error
            ? caughtError.message
            : "Unknown backend error";
        setError(message);
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => controller.abort();
  }, []);

  const things = useMemo(
    () =>
      issues.map((issue) => ({
        id: issue.id,
        title: issue.raw.length > 60 ? `${issue.raw.slice(0, 60)}…` : issue.raw,
        href: `/issues/${issue.id}`,
      })),
    [issues]
  );

  return (
    <AppShell
      sidebar={
        <AppSidebar
          things={things}
          navigateOnCreate={false}
          onIssueCreated={(created) => {
            setIssues((prev) => [created, ...prev]);
            setError(null);
          }}
        />
      }
    >
      <SiteHeader
        title="Open issues"
        eyebrow="Issues"
        subtitle="Incoming reports, bugs, and ideas waiting to be triaged."
        meta={
          issues.length > 0 ? (
            <span className="app-chip tabular-nums">
              {issues.length}
            </span>
          ) : null
        }
      />
      <div className="flex flex-1 flex-col">
        <div className="@container/main flex flex-1 flex-col gap-2">
          <div className="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
            {error && (
              <div className="px-4 lg:px-6">
                <div className="app-status-warning">
                  Issue backend unavailable: {error}
                </div>
              </div>
            )}

            {loading && (
              <div className="space-y-3 px-4 lg:px-6">
                {Array.from({ length: 4 }).map((_, index) => (
                  <div
                    key={index}
                    className="app-surface animate-fade-in-up p-5"
                    style={{ animationDelay: `${index * 75}ms` }}
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

            {issues.length > 0 && (
              <div className="space-y-2 px-4 lg:px-6">
                {issues.map((issue) => (
                  <IssueCard
                    key={issue.id}
                    issue={issue}
                    href={`/issues/${issue.id}`}
                  />
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </AppShell>
  );
}
