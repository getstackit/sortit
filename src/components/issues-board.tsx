"use client";

import { useEffect, useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { IssueCard } from "@/components/issue-card";
import { SiteHeader } from "@/components/site-header";
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
    <AppShell sidebar={<AppSidebar things={things} />}>
      <SiteHeader
        title="Open issues"
        meta={
          issues.length > 0 ? (
            <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground tabular-nums">
              {issues.length}
            </span>
          ) : null
        }
        navigateOnCreate={false}
        onIssueCreated={(created) => {
          setIssues((prev) => [created, ...prev]);
          setError(null);
        }}
      />
      <div className="flex flex-1 flex-col">
        <div className="@container/main flex flex-1 flex-col gap-2">
          <div className="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
            {error && (
              <div className="px-4 lg:px-6">
                <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
                  Issue backend unavailable: {error}
                </div>
              </div>
            )}

            {loading && (
              <div className="px-4 lg:px-6">
                <div className="rounded-xl border border-border/60 bg-card p-5 text-sm text-muted-foreground">
                  Loading issues...
                </div>
              </div>
            )}

            {!loading && issues.length === 0 && (
              <div className="flex flex-col items-center gap-3 py-20 text-center">
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
