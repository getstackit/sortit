"use client";

import Link from "next/link";
import { cn } from "@/lib/utils";
import type { IssueRecord } from "@/lib/issues";

const TAG_COLORS: Record<string, string> = {
  bug: "bg-red-100 text-red-700",
  crash: "bg-red-100 text-red-700",
  feature: "bg-purple-100 text-purple-700",
  idea: "bg-purple-100 text-purple-700",
  improvement: "bg-green-100 text-green-700",
  ui: "bg-blue-100 text-blue-700",
  ux: "bg-blue-100 text-blue-700",
  frontend: "bg-blue-100 text-blue-700",
  performance: "bg-amber-100 text-amber-700",
  safari: "bg-amber-100 text-amber-700",
};

const FALLBACK_COLOR = "bg-slate-100 text-slate-600";

function tagColor(label: string) {
  return TAG_COLORS[label] ?? FALLBACK_COLOR;
}

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
};

export function IssueCard({ issue, href, className }: IssueCardProps) {
  const content = (
    <>
      <p
        className={cn(
          "whitespace-pre-wrap leading-relaxed",
          looksLikeCode(issue.raw)
            ? "font-mono text-[13px] text-foreground/80"
            : "text-[15px]"
        )}
      >
        {issue.raw}
      </p>
      <div className="mt-3 flex items-center gap-2">
        <div className="flex flex-wrap gap-1.5">
          {issue.tags.map((tag) => (
            <span
              key={tag}
              className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${tagColor(tag)}`}
            >
              {tag}
            </span>
          ))}
        </div>
        <span className="ml-auto text-[11px] tracking-wide text-muted-foreground/40 transition-colors group-hover/card:text-muted-foreground/60">
          {issue.createdBy} &middot; {timeAgo(issue.createdAt)}
        </span>
      </div>
    </>
  );

  if (!href) {
    return (
      <div
        className={cn(
          "rounded-2xl border border-border/60 bg-card p-5",
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
        "group/card block rounded-xl border border-border/60 bg-card p-5 transition-colors hover:border-border hover:bg-accent/30",
        className
      )}
    >
      {content}
    </Link>
  );
}
