"use client";

import Link from "next/link";
import { TagBadge } from "@/components/tag-badge";
import { cn } from "@/lib/utils";
import type { MemoryKind, MemoryRecord, MemoryStatus } from "@/lib/memories";

export const KIND_STYLES: Record<MemoryKind, string> = {
  decision: "border-violet-500/40 bg-violet-500/10 text-violet-600 dark:text-violet-300",
  lesson: "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-300",
  constraint: "border-rose-500/40 bg-rose-500/10 text-rose-600 dark:text-rose-300",
  pattern: "border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300",
  reference: "border-sky-500/40 bg-sky-500/10 text-sky-600 dark:text-sky-300",
  // A concept is the canonical profile of a noun — the node the rest of the
  // corpus orbits — so it gets a stronger, distinct indigo fill.
  concept: "border-indigo-500/60 bg-indigo-500/20 text-indigo-700 dark:text-indigo-200",
  // The overview is the project's identity singleton — a distinct teal so it
  // reads as the frame the rest of the corpus sits inside.
  overview: "border-teal-500/60 bg-teal-500/20 text-teal-700 dark:text-teal-200",
};

const STATUS_STYLES: Record<MemoryStatus, string> = {
  active: "border-emerald-500/40 text-emerald-600 dark:text-emerald-300",
  superseded: "border-amber-500/40 text-amber-600 dark:text-amber-300",
  archived: "border-border text-muted-foreground",
};

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

export function MemoryCard({ memory }: { memory: MemoryRecord }) {
  const reinforcement = Math.max(0, Math.min(1, memory.confidence ?? 0));
  return (
    <Link
      href={`/memories/${encodeURIComponent(memory.id)}`}
      className="group flex flex-col gap-3 rounded-2xl border border-border/70 bg-card p-4 transition-all hover:border-border hover:shadow-sm"
    >
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide",
            KIND_STYLES[memory.kind] ?? KIND_STYLES.decision
          )}
        >
          {memory.kind}
        </span>
        {memory.kind === "concept" && memory.subjectTag && (
          <span className="min-w-0 truncate rounded-full border border-indigo-500/40 px-2 py-0.5 text-[10px] font-medium text-indigo-600 dark:text-indigo-300">
            #{memory.subjectTag}
          </span>
        )}
        {memory.source === "synthesized" && (
          <span className="rounded-full border border-border/70 px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
            synthesized
          </span>
        )}
        <span
          className={cn(
            "ml-auto rounded-full border px-2 py-0.5 text-[10px] font-medium",
            STATUS_STYLES[memory.status] ?? STATUS_STYLES.archived
          )}
        >
          {memory.status}
        </span>
      </div>

      <div className="min-w-0">
        <h3 className="truncate text-sm font-semibold group-hover:text-foreground">
          {memory.title || "Untitled memory"}
        </h3>
        <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
          {memory.body}
        </p>
      </div>

      {memory.anchorTags && memory.anchorTags.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {memory.anchorTags.map((tag) => (
            <TagBadge key={tag} tag={tag} compact />
          ))}
        </div>
      )}

      <div className="mt-auto flex items-center gap-3 pt-1">
        <div className="flex flex-1 items-center gap-2" title="Confidence">
          <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-gradient-to-r from-violet-500 to-sky-500"
              style={{ width: `${Math.round(reinforcement * 100)}%` }}
            />
          </div>
          <span className="text-[10px] tabular-nums text-muted-foreground">
            {reinforcement.toFixed(2)}
          </span>
        </div>
        <span className="text-[10px] text-muted-foreground">{formatDate(memory.createdAt)}</span>
      </div>
    </Link>
  );
}
