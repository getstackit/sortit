"use client";

import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/status-badge";
import { TagBadge } from "@/components/tag-badge";
import { cn } from "@/lib/utils";
import type { IssueRecord } from "@/lib/issues";
import { Markdown } from "@/components/markdown";

function timeAgo(timestamp: string) {
  const date = new Date(timestamp);
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return date.toLocaleDateString();
}

type IssueCardProps = {
  issue: IssueRecord;
  href?: string;
  className?: string;
  compact?: boolean;
};

export function IssueCard({ issue, href, className, compact }: IssueCardProps) {
  const content = (
    <>
      <div
        className={cn(
          issue.status === "closed" && "text-muted-foreground",
          compact && "line-clamp-2"
        )}
      >
        <Markdown compact={compact}>{issue.raw}</Markdown>
      </div>
      <div className={cn("flex items-center gap-2", compact ? "mt-1.5" : "mt-3")}>
        <div className="flex flex-wrap gap-1.5">
          {issue.enrichmentStatus === "pending" && (
            <StatusBadge variant="analyzing" compact={compact} />
          )}
          {issue.enrichmentStatus === "failed" && (
            <StatusBadge variant="failed" compact={compact} />
          )}
          {issue.status === "closed" && (
            <StatusBadge variant="closed" compact={compact} />
          )}
          {issue.tags.map((tag) => (
            <TagBadge key={tag} tag={tag} compact={compact} />
          ))}
          {(() => {
            const progressCount = (issue.discussion ?? []).filter(
              (p) => p.kind === "progress"
            ).length;
            if (progressCount > 0) {
              return (
                <Badge variant="secondary" className={cn(
                  "rounded-full bg-blue-100 text-blue-700",
                  compact ? "h-auto px-1.5 py-px text-[10px]" : "h-auto px-2 py-0.5 text-[11px]"
                )}>
                  {progressCount} progress
                </Badge>
              );
            }
            return null;
          })()}
        </div>
        {issue.assignedTo && (
          <Badge variant="secondary" className={cn(
            "rounded-full bg-violet-100 text-violet-700",
            compact ? "h-auto px-1.5 py-px text-[10px]" : "h-auto px-2 py-0.5 text-[11px]"
          )}>
            {issue.assignedTo}
          </Badge>
        )}
        <span className={cn(
          "ml-auto tracking-wide text-muted-foreground/40 transition-colors group-hover/card:text-muted-foreground/60",
          compact ? "text-[10px]" : "text-[11px]"
        )}>
          {issue.createdBy} &middot; {timeAgo(issue.createdAt)}
        </span>
      </div>
    </>
  );

  const padding = compact ? "px-3 py-2" : "p-5";

  if (!href) {
    return (
      <div
        className={cn(
          "app-surface",
          padding,
          issue.status === "closed" && "bg-muted/45",
          className
        )}
      >
        {content}
      </div>
    );
  }

  return (
    <Link
      href={href}
      className={cn(
        "group/card app-surface block transition-all hover:-translate-y-0.5 hover:border-foreground/10 hover:bg-card/95",
        padding,
        issue.status === "closed" && "bg-muted/45",
        className
      )}
    >
      {content}
    </Link>
  );
}
