"use client";

import Link from "next/link";
import { TAG_COLORS, dominantTag } from "@/features/map/model";
import type { TagRelevance } from "@/features/map/types";
import { TagBadge } from "@/components/tag-badge";
import { cn } from "@/lib/utils";

type IssueListItemIssue = {
  id: string;
  raw: string;
  status: "open" | "closed";
  assignedTo?: string;
};

type IssueListItemProps = {
  issue: IssueListItemIssue;
  tags: string[] | TagRelevance[];
  href?: string;
  onClick?: () => void;
  maxLabelLength?: number;
  className?: string;
};

function normalizeTags(tags: IssueListItemProps["tags"]): TagRelevance[] {
  return tags.map((tag) =>
    typeof tag === "string" ? { tag, relevance: 1 } : tag
  );
}

function issueLabel(raw: string, maxLength: number) {
  return raw.length > maxLength ? `${raw.slice(0, maxLength)}...` : raw;
}

export function IssueListItem({
  issue,
  tags,
  href,
  onClick,
  maxLabelLength = 60,
  className,
}: IssueListItemProps) {
  const normalizedTags = normalizeTags(tags);
  const displayTags = normalizedTags.slice(0, 3);
  const dominantTagColor =
    TAG_COLORS[dominantTag(normalizedTags)] ?? "#94a3b8";
  const content = (
    <>
      <div
        className="mt-1 h-2 w-2 shrink-0 rounded-full"
        style={{ backgroundColor: dominantTagColor }}
      />
      <div className="min-w-0 flex-1">
        <p className="truncate text-[11px] text-foreground">
          {issueLabel(issue.raw, maxLabelLength)}
        </p>
        <div className="mt-1 flex flex-wrap gap-1">
          <span
            className={cn(
              "rounded-full px-1.5 py-0.5 text-[9px] font-medium",
              issue.assignedTo
                ? "bg-violet-100 text-violet-700"
                : "bg-muted text-muted-foreground"
            )}
          >
            {issue.assignedTo ?? "Unassigned"}
          </span>
          {displayTags.map(({ tag }) => (
            <TagBadge key={tag} tag={tag} compact />
          ))}
        </div>
      </div>
      <span
        className={
          issue.status === "closed"
            ? "shrink-0 rounded-full bg-slate-200 px-1.5 py-0.5 text-[9px] font-medium text-slate-700"
            : "shrink-0 rounded-full bg-emerald-100 px-1.5 py-0.5 text-[9px] font-medium text-emerald-700"
        }
      >
        {issue.status === "closed" ? "Closed" : "Open"}
      </span>
    </>
  );

  const sharedClassName = cn(
    "flex w-full items-start gap-2 rounded-md px-1.5 py-1 text-left transition-colors hover:bg-muted/60",
    className
  );

  if (href) {
    return (
      <Link href={href} className={sharedClassName}>
        {content}
      </Link>
    );
  }

  return (
    <button type="button" onClick={onClick} className={sharedClassName}>
      {content}
    </button>
  );
}
