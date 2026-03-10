"use client";

import Link from "next/link";
import useSWR from "swr";
import { ActivitySquareIcon, GitMergeIcon, MessageSquareMoreIcon, UserIcon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { useBackendRevision } from "@/hooks/use-issues";
import { fetchActivity, type ActivityEventRecord } from "@/lib/activity";
import { cn } from "@/lib/utils";

function formatRelativeTime(value: string) {
  const timestamp = new Date(value).getTime();
  const diffSeconds = Math.round((timestamp - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

  const ranges: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["year", 60 * 60 * 24 * 365],
    ["month", 60 * 60 * 24 * 30],
    ["week", 60 * 60 * 24 * 7],
    ["day", 60 * 60 * 24],
    ["hour", 60 * 60],
    ["minute", 60],
  ];

  for (const [unit, seconds] of ranges) {
    if (Math.abs(diffSeconds) >= seconds || unit === "minute") {
      return formatter.format(Math.round(diffSeconds / seconds), unit);
    }
  }

  return "just now";
}

function eventIcon(kind: string) {
  if (kind === "split" || kind === "combine" || kind === "link") {
    return <GitMergeIcon className="size-4" />;
  }
  if (kind === "assigned") {
    return <UserIcon className="size-4" />;
  }
  return <MessageSquareMoreIcon className="size-4" />;
}

function eventLabel(kind: string) {
  switch (kind) {
    case "report":
      return "Report";
    case "refinement":
      return "Refinement";
    case "progress":
      return "Progress";
    case "closed":
      return "Closed";
    case "reopened":
      return "Reopened";
    case "assigned":
      return "Assignment";
    case "split":
      return "Split";
    case "combine":
      return "Combine";
    case "link":
      return "Link";
    default:
      return kind;
  }
}

function ActivityCard({ event }: { event: ActivityEventRecord }) {
  return (
    <article className="app-surface rounded-[1.5rem] p-5">
      <div className="flex items-start gap-3">
        <div className="rounded-full bg-muted p-2 text-muted-foreground">
          {eventIcon(event.kind)}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold">{eventLabel(event.kind)}</span>
            <span className="app-chip">{formatRelativeTime(event.createdAt)}</span>
            <span className="text-xs text-muted-foreground">{event.createdBy}</span>
          </div>

          {event.issue && (
            <div className="mt-2">
              <Link
                href={`/issues/${event.issue.id}`}
                className="text-sm font-medium transition-colors hover:text-foreground/80"
              >
                {event.issue.raw}
              </Link>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {event.issue.id} · {event.issue.status}
              </p>
            </div>
          )}

          {event.body && (
            <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-foreground/85">
              {event.body}
            </p>
          )}

          {event.participants && event.participants.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {event.participants.map((participant) => (
                <Link
                  key={`${event.id}-${participant.issueId}-${participant.role}`}
                  href={`/issues/${participant.issueId}`}
                  className={cn(
                    "rounded-full border border-border/70 px-2.5 py-1 text-[11px] font-medium transition-colors hover:bg-accent/70"
                  )}
                >
                  {(participant.issue?.id ?? participant.issueId)} · {participant.role}
                </Link>
              ))}
            </div>
          )}
        </div>
      </div>
    </article>
  );
}

export function ActivityFeedPage() {
  const { data: revision = 0 } = useBackendRevision();
  const { data, error, isLoading } = useSWR(
    ["activity-feed", revision],
    () => fetchActivity({ limit: 80 })
  );

  const events = data?.events ?? [];

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        eyebrow="Activity"
        title="Global activity feed"
        subtitle="Cross-issue discussion, status changes, assignments, and relationship operations."
        meta={<span className="app-chip">{events.length} recent events</span>}
        actions={
          <Link
            href="/"
            className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            All issues
          </Link>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 px-4 py-6 lg:px-6">
          {isLoading && (
            <div className="app-subtle-surface rounded-[1.5rem] p-5 text-sm text-muted-foreground">
              Loading activity...
            </div>
          )}

          {!isLoading && error && (
            <div className="app-status-warning">
              Activity backend unavailable: {error.message}
            </div>
          )}

          {!isLoading && !error && events.length === 0 && (
            <div className="app-subtle-surface rounded-[1.5rem] p-8 text-center text-sm text-muted-foreground">
              <ActivitySquareIcon className="mx-auto mb-3 size-5" />
              No activity yet.
            </div>
          )}

          {events.map((event) => (
            <ActivityCard key={event.id} event={event} />
          ))}
        </div>
      </div>
    </AppShell>
  );
}
