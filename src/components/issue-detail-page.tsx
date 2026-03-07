"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { AppShell, AppShellToggle } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { IssueCard } from "@/components/issue-card";
import { fetchIssue, fetchIssues, type IssueRecord } from "@/lib/issues";

function formatCreatedAt(value: string) {
  return new Date(value).toLocaleString();
}

export function IssueDetailPage({ issueID }: { issueID: string }) {
  const [issue, setIssue] = useState<IssueRecord | null>(null);
  const [issues, setIssues] = useState<IssueRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    Promise.allSettled([
      fetchIssue(issueID, controller.signal),
      fetchIssues(controller.signal),
    ])
      .then(([issueResult, issuesResult]) => {
        if (issueResult.status === "fulfilled") {
          setIssue(issueResult.value);
          setError(null);
        } else if (!controller.signal.aborted) {
          const message =
            issueResult.reason instanceof Error
              ? issueResult.reason.message
              : "Unknown backend error";
          setIssue(null);
          setError(message);
        }

        if (issuesResult.status === "fulfilled") {
          setIssues(issuesResult.value);
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => controller.abort();
  }, [issueID]);

  const things = useMemo(
    () =>
      issues.map((entry) => ({
        id: entry.id,
        title:
          entry.raw.length > 60 ? `${entry.raw.slice(0, 60)}…` : entry.raw,
        href: `/issues/${entry.id}`,
      })),
    [issues]
  );

  return (
    <AppShell sidebar={<AppSidebar things={things} />}>
      <header className="sticky top-0 z-10 shrink-0 border-b bg-background">
        <div className="flex min-h-12 items-center gap-2 px-4">
          <AppShellToggle className="-ml-1" />
          <div className="mr-2 h-4 w-px shrink-0 bg-border" />
          <div className="min-w-0">
            <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
              Issue
            </p>
            <h1 className="truncate text-sm font-medium">
              {issue?.id ?? issueID}
            </h1>
          </div>
          <Link
            href="/"
            className="ml-auto rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            All issues
          </Link>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-6 lg:px-6">
          {loading && (
            <div className="rounded-2xl border border-border/60 bg-card p-5 text-sm text-muted-foreground">
              Loading issue...
            </div>
          )}

          {!loading && error && (
            <div className="rounded-2xl border border-amber-200 bg-amber-50 p-5 text-sm text-amber-900">
              {error === "issue not found"
                ? `No issue exists for "${issueID}".`
                : `Issue backend unavailable: ${error}`}
            </div>
          )}

          {!loading && issue && (
            <>
              <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <span className="rounded-full bg-muted px-3 py-1 font-medium text-foreground">
                  {issue.id}
                </span>
                <span>{issue.createdBy}</span>
                <span>&middot;</span>
                <span>{formatCreatedAt(issue.createdAt)}</span>
              </div>
              <IssueCard issue={issue} className="rounded-3xl p-6 shadow-sm" />
            </>
          )}
        </div>
      </div>
    </AppShell>
  );
}
