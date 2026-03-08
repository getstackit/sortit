"use client";

import Link from "next/link";
import { cn } from "@/lib/utils";
import { entityStyle } from "@/lib/entity-colors";
import type { IssueRecord } from "@/lib/issues";

function looksLikeCode(text: string) {
  return (
    text.includes("Error") ||
    text.includes("at ") ||
    text.includes("=>") ||
    text.includes("function") ||
    text.includes("{") ||
    /^\s*(import|const|let|var|def|class)\b/m.test(text)
  );
}

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
      <p
        className={cn(
          "whitespace-pre-wrap leading-relaxed",
          issue.status === "closed" && "text-muted-foreground",
          compact
            ? "line-clamp-2 text-xs"
            : looksLikeCode(issue.raw)
              ? "font-mono text-[13px] text-foreground/80"
              : "text-[15px]"
        )}
      >
        {issue.raw}
      </p>
      <div className={cn("flex items-center gap-2", compact ? "mt-1.5" : "mt-3")}>
        <div className="flex flex-wrap gap-1.5">
          {issue.status === "closed" && (
            <span className="rounded-full bg-slate-200 px-2 py-0.5 text-[11px] font-medium text-slate-700">
              Closed
            </span>
          )}
          {issue.tags.map((tag) => (
            <span
              key={tag}
              className={cn(
                "rounded-full border font-medium",
                compact ? "px-1.5 py-px text-[10px]" : "px-2 py-0.5 text-[11px]"
              )}
              style={entityStyle(tag)}
            >
              {tag}
            </span>
          ))}
          {(() => {
            const progressCount = (issue.discussion ?? []).filter(
              (p) => p.kind === "progress"
            ).length;
            if (progressCount > 0) {
              return (
                <span
                  className={cn(
                    "rounded-full bg-blue-100 font-medium text-blue-700",
                    compact ? "px-1.5 py-px text-[10px]" : "px-2 py-0.5 text-[11px]"
                  )}
                >
                  {progressCount} progress
                </span>
              );
            }
            return null;
          })()}
        </div>
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
