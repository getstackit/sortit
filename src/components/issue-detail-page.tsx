"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { CheckIcon, CopyIcon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { IssueCard } from "@/components/issue-card";
import { SiteHeader } from "@/components/site-header";
import { Button } from "@/components/ui/button";
import {
  closeIssue,
  fetchIssue,
  fetchIssues,
  reopenIssue,
  type IssueRecord,
} from "@/lib/issues";

function formatCreatedAt(value: string) {
  return new Date(value).toLocaleString();
}

async function copyText(value: string) {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }

  if (typeof document === "undefined") {
    throw new Error("Clipboard unavailable");
  }

  const input = document.createElement("textarea");
  input.value = value;
  input.setAttribute("readonly", "");
  input.style.position = "absolute";
  input.style.left = "-9999px";
  document.body.appendChild(input);
  input.select();

  const succeeded = document.execCommand("copy");
  document.body.removeChild(input);

  if (!succeeded) {
    throw new Error("Clipboard unavailable");
  }
}

export function IssueDetailPage({ issueID }: { issueID: string }) {
  const [issue, setIssue] = useState<IssueRecord | null>(null);
  const [issues, setIssues] = useState<IssueRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [copyState, setCopyState] = useState<"idle" | "copied" | "error">(
    "idle"
  );
  const [statusPending, setStatusPending] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    Promise.allSettled([
      fetchIssue(issueID, controller.signal),
      fetchIssues("all", controller.signal),
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

  useEffect(() => {
    if (copyState === "idle") {
      return;
    }

    const timeout = window.setTimeout(() => {
      setCopyState("idle");
    }, 2000);

    return () => window.clearTimeout(timeout);
  }, [copyState]);

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

  async function handleCopyIssue() {
    if (!issue) {
      return;
    }

    try {
      await copyText(issue.raw);
      setCopyState("copied");
    } catch {
      setCopyState("error");
    }
  }

  async function handleStatusChange() {
    if (!issue || statusPending) {
      return;
    }

    setStatusPending(true);

    try {
      const updated =
        issue.status === "closed"
          ? await reopenIssue(issue.id)
          : await closeIssue(issue.id);

      setIssue(updated);
      setIssues((current) =>
        current.map((entry) => (entry.id === updated.id ? updated : entry))
      );
      setError(null);
    } catch (caughtError) {
      const message =
        caughtError instanceof Error
          ? caughtError.message
          : "Unknown backend error";
      setError(message);
    } finally {
      setStatusPending(false);
    }
  }

  return (
    <AppShell sidebar={<AppSidebar things={things} />}>
      <SiteHeader
        title={issue?.id ?? issueID}
        eyebrow="Issue"
        actions={
          <Link
            href="/"
            className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            All issues
          </Link>
        }
      />

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
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded-full bg-muted px-3 py-1 font-medium text-foreground">
                    {issue.id}
                  </span>
                  <span
                    className={
                      issue.status === "closed"
                        ? "rounded-full bg-slate-200 px-3 py-1 font-medium text-slate-700"
                        : "rounded-full bg-emerald-100 px-3 py-1 font-medium text-emerald-700"
                    }
                  >
                    {issue.status === "closed" ? "Closed" : "Open"}
                  </span>
                  <span>{issue.createdBy}</span>
                  <span>&middot;</span>
                  <span>{formatCreatedAt(issue.createdAt)}</span>
                  {issue.status === "closed" && issue.closedAt && (
                    <>
                      <span>&middot;</span>
                      <span>
                        Closed {formatCreatedAt(issue.closedAt)}
                        {issue.closedBy ? ` by ${issue.closedBy}` : ""}
                      </span>
                    </>
                  )}
                </div>
                <Button
                  type="button"
                  variant={issue.status === "closed" ? "outline" : "default"}
                  size="sm"
                  onClick={() => void handleStatusChange()}
                  disabled={statusPending}
                >
                  {statusPending
                    ? issue.status === "closed"
                      ? "Reopening..."
                      : "Closing..."
                    : issue.status === "closed"
                      ? "Reopen"
                      : "Close issue"}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => void handleCopyIssue()}
                >
                  {copyState === "copied" ? (
                    <CheckIcon aria-hidden="true" />
                  ) : (
                    <CopyIcon aria-hidden="true" />
                  )}
                  {copyState === "copied"
                    ? "Copied"
                    : copyState === "error"
                      ? "Copy failed"
                      : "Copy issue"}
                </Button>
              </div>
              <IssueCard issue={issue} className="rounded-3xl p-6 shadow-sm" />
            </>
          )}
        </div>
      </div>
    </AppShell>
  );
}
