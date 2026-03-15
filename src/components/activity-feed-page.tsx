"use client";

import { useCallback, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import {
  ActivitySquareIcon,
  CheckCircle2Icon,
  CircleDotIcon,
  GitMergeIcon,
  LinkIcon,
  MessageSquareMoreIcon,
  PlusCircleIcon,
  Loader2Icon,
  RefreshCwIcon,
  ScissorsIcon,
  TrendingUpIcon,
  UserIcon,
} from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { useBackendRevision } from "@/hooks/use-issues";
import { fetchActivity, type ActivityEventRecord } from "@/lib/activity";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/format";

const EVENT_KINDS = [
  { value: "", label: "All" },
  { value: "report", label: "Reports", icon: PlusCircleIcon },
  { value: "refinement", label: "Refinements", icon: MessageSquareMoreIcon },
  { value: "progress", label: "Progress", icon: TrendingUpIcon },
  { value: "closed", label: "Closed", icon: CheckCircle2Icon },
  { value: "reopened", label: "Reopened", icon: RefreshCwIcon },
  { value: "assigned", label: "Assigned", icon: UserIcon },
  { value: "split", label: "Split", icon: ScissorsIcon },
  { value: "combine", label: "Combine", icon: GitMergeIcon },
  { value: "link", label: "Link", icon: LinkIcon },
] as const;

function eventIcon(kind: string) {
  const match = EVENT_KINDS.find((k) => k.value === kind);
  if (match && "icon" in match) {
    const Icon = match.icon;
    return <Icon className="size-4" />;
  }
  return <CircleDotIcon className="size-4" />;
}

function eventLabel(kind: string) {
  const match = EVENT_KINDS.find((k) => k.value === kind);
  return match ? match.label.replace(/s$/, "") : kind;
}

function entityLink(entityType: string, entityId: string) {
  if (entityType === "issue") {
    return `/issues/${entityId}`;
  }
  return "#";
}

function ActivityCard({ event }: { event: ActivityEventRecord }) {
  return (
    <div className="relative flex gap-4 pb-8 last:pb-0">
      {/* Timeline line */}
      <div className="absolute left-[15px] top-9 bottom-0 w-px bg-border/60 last:hidden" />

      {/* Icon */}
      <div className="relative z-10 flex size-8 shrink-0 items-center justify-center rounded-full border border-border/70 bg-background text-muted-foreground">
        {eventIcon(event.kind)}
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1 pt-0.5">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span className="text-sm font-medium">{eventLabel(event.kind)}</span>
          {event.entityId && (
            <Link
              href={entityLink(event.entityType, event.entityId)}
              className="text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              {event.entityId}
            </Link>
          )}
          <span className="text-xs text-muted-foreground">
            {event.createdBy ? `by ${event.createdBy}` : ""}
          </span>
          <span className="text-xs text-muted-foreground/70">
            {formatRelativeTime(event.createdAt)}
          </span>
        </div>

        {event.body && (
          <p className="mt-1.5 whitespace-pre-wrap text-sm leading-relaxed text-foreground/80">
            {event.body}
          </p>
        )}

        {event.participants && event.participants.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {event.participants.map((participant) => (
              <Link
                key={`${event.id}-${participant.entityId}-${participant.role}`}
                href={entityLink(participant.entityType, participant.entityId)}
                className="rounded-full border border-border/70 px-2.5 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-accent/70 hover:text-foreground"
              >
                {participant.entityId} · {participant.role}
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

const PAGE_SIZE = 40;

export function ActivityFeedPage() {
  const [kindFilter, setKindFilter] = useState("");
  const [allEvents, setAllEvents] = useState<ActivityEventRecord[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>(undefined);
  const [loadingMore, setLoadingMore] = useState(false);
  const { data: revision = 0 } = useBackendRevision();

  const { error, isLoading } = useSWR(
    ["activity-feed", revision, kindFilter],
    () => fetchActivity({ limit: PAGE_SIZE, kind: kindFilter || undefined }),
    {
      onSuccess(data) {
        setAllEvents(data.events);
        setNextCursor(data.nextCursor);
      },
    }
  );

  const loadMore = useCallback(async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const data = await fetchActivity({
        limit: PAGE_SIZE,
        cursor: nextCursor,
        kind: kindFilter || undefined,
      });
      setAllEvents((prev) => [...prev, ...data.events]);
      setNextCursor(data.nextCursor);
    } finally {
      setLoadingMore(false);
    }
  }, [nextCursor, loadingMore, kindFilter]);

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        eyebrow="Activity"
        title="Global activity feed"
        subtitle="Cross-issue discussion, status changes, assignments, and relationship operations."
        meta={<Badge variant="outline">{allEvents.length} recent events</Badge>}
        actions={
          <div className="flex flex-wrap items-center gap-1.5">
            {EVENT_KINDS.map((kind) => (
              <button
                key={kind.value}
                onClick={() => setKindFilter(kind.value)}
                className={cn(
                  "rounded-full px-2.5 py-1 text-[11px] font-medium transition-colors",
                  kindFilter === kind.value
                    ? "bg-foreground text-background"
                    : "border border-border/70 text-muted-foreground hover:bg-accent/70 hover:text-foreground"
                )}
              >
                {kind.label}
              </button>
            ))}
          </div>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:px-6">
          {isLoading && (
            <div className="py-8 text-center text-sm text-muted-foreground">
              Loading activity...
            </div>
          )}

          {!isLoading && error && (
            <div className="app-status-warning">
              Activity backend unavailable: {error.message}
            </div>
          )}

          {!isLoading && !error && allEvents.length === 0 && (
            <div className="py-12 text-center text-sm text-muted-foreground">
              <ActivitySquareIcon className="mx-auto mb-3 size-5" />
              {kindFilter
                ? `No ${EVENT_KINDS.find((k) => k.value === kindFilter)?.label.toLowerCase() ?? kindFilter} events yet.`
                : "No activity yet."}
            </div>
          )}

          {allEvents.length > 0 && (
            <div className="pl-0">
              {allEvents.map((event) => (
                <ActivityCard key={event.id} event={event} />
              ))}

              {nextCursor && (
                <div className="flex justify-center py-6">
                  <button
                    onClick={loadMore}
                    disabled={loadingMore}
                    className="inline-flex items-center gap-2 rounded-full border border-border/70 px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent/70 hover:text-foreground disabled:opacity-50"
                  >
                    {loadingMore ? (
                      <>
                        <Loader2Icon className="size-4 animate-spin" />
                        Loading...
                      </>
                    ) : (
                      "Load more"
                    )}
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
