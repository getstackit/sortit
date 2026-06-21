"use client";

import Link from "next/link";
import { toast } from "sonner";
import { Archive, ArrowLeft, History } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { KIND_STYLES } from "@/components/memory-card";
import { TagBadge } from "@/components/tag-badge";
import { Button } from "@/components/ui/button";
import { useMemory } from "@/hooks/use-memories";
import { archiveMemory, supersedeMemory } from "@/lib/memories";
import { tagHref } from "@/lib/tags";
import { cn } from "@/lib/utils";

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function MemoryDetailPage({ memoryID }: { memoryID: string }) {
  const { data: memory, isLoading, error } = useMemory(memoryID);

  async function handleSupersede() {
    try {
      await supersedeMemory(memoryID);
      toast.success("Memory superseded");
    } catch (e) {
      toast.error(errorMessage(e, "Failed to supersede"));
    }
  }

  async function handleArchive() {
    try {
      await archiveMemory(memoryID);
      toast.success("Memory archived");
    } catch (e) {
      toast.error(errorMessage(e, "Failed to archive"));
    }
  }

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title={memory?.title || "Memory"}
        eyebrow="Knowledge"
        meta={
          memory ? (
            <span
              className={cn(
                "rounded-full border px-2 py-0.5 text-[10px] font-medium",
                memory.status === "active"
                  ? "border-emerald-500/40 text-emerald-600 dark:text-emerald-300"
                  : "border-amber-500/40 text-amber-600 dark:text-amber-300"
              )}
            >
              {memory.status}
            </span>
          ) : undefined
        }
      />
      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-5 p-4 sm:p-6">
          <Link
            href="/memories"
            className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground"
          >
            <ArrowLeft className="size-3.5" /> All memories
          </Link>

          {isLoading && <p className="text-sm text-muted-foreground">Loading…</p>}
          {error && (
            <p className="text-sm text-destructive">Could not load this memory.</p>
          )}

          {memory && (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <span
                  className={cn(
                    "rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide",
                    KIND_STYLES[memory.kind] ?? KIND_STYLES.decision
                  )}
                >
                  {memory.kind}
                </span>
                <span className="rounded-full border border-border/70 px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                  {memory.source}
                </span>
                {memory.reinforcementCount > 0 && (
                  <span className="rounded-full border border-sky-500/40 px-2 py-0.5 text-[10px] font-medium text-sky-600 dark:text-sky-300">
                    reinforced ×{memory.reinforcementCount}
                  </span>
                )}
              </div>

              <h1 className="text-xl font-semibold">{memory.title || "Untitled memory"}</h1>

              {memory.kind === "concept" && memory.subjectTag && (
                <p className="text-sm text-muted-foreground">
                  Profiles tag{" "}
                  <Link
                    href={tagHref(memory.subjectTag)}
                    className="font-medium text-indigo-600 hover:underline dark:text-indigo-300"
                  >
                    #{memory.subjectTag}
                  </Link>
                </p>
              )}

              <div className="whitespace-pre-wrap rounded-2xl border border-border/70 bg-card p-4 text-sm leading-relaxed">
                {memory.body}
              </div>

              {memory.anchorTags && memory.anchorTags.length > 0 && (
                <div>
                  <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                    Anchored to
                  </p>
                  <div className="flex flex-wrap gap-1">
                    {memory.anchorTags.map((tag) => (
                      <TagBadge key={tag} tag={tag} />
                    ))}
                  </div>
                </div>
              )}

              {memory.sourceIssueIds && memory.sourceIssueIds.length > 0 && (
                <div>
                  <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                    Learned from
                  </p>
                  <div className="flex flex-wrap gap-1.5">
                    {memory.sourceIssueIds.map((id) => (
                      <Link
                        key={id}
                        href={`/issues/${encodeURIComponent(id)}`}
                        className="rounded-lg border border-border/70 px-2 py-1 font-mono text-[11px] text-muted-foreground hover:bg-accent hover:text-foreground"
                      >
                        {id.slice(0, 10)}
                      </Link>
                    ))}
                  </div>
                </div>
              )}

              {memory.status === "active" && (
                <div className="flex gap-2 border-t border-border/60 pt-4">
                  <Button size="sm" variant="outline" onClick={() => void handleSupersede()}>
                    <History className="size-3.5" /> Supersede
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => void handleArchive()}>
                    <Archive className="size-3.5" /> Archive
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </AppShell>
  );
}
