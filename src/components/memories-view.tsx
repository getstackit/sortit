"use client";

import { useState } from "react";
import { Sparkles, Check, X, BookMarked } from "lucide-react";
import { MemoryCard } from "@/components/memory-card";
import { ProjectOverviewPanel } from "@/components/project-overview-panel";
import { TagBadge } from "@/components/tag-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import type {
  CreateMemoryInput,
  MemoryKind,
  MemoryProposalRecord,
  MemoryRecord,
} from "@/lib/memories";

// Overview is intentionally absent: it is the project's identity singleton, edited
// through the dedicated panel at the top — not created via the generic picker or
// shown as a card in the grid.
const KINDS: MemoryKind[] = ["decision", "lesson", "constraint", "pattern", "reference", "concept"];

export type MemoriesViewProps = {
  memories: MemoryRecord[];
  proposals: MemoryProposalRecord[];
  overview: MemoryRecord | null;
  loading?: boolean;
  synthesizing?: boolean;
  onCreate: (input: CreateMemoryInput) => Promise<void> | void;
  onSaveOverview: (body: string) => Promise<void> | void;
  onSynthesize: () => Promise<void> | void;
  onAccept: (id: string) => Promise<void> | void;
  onReject: (id: string) => Promise<void> | void;
};

export function MemoriesView({
  memories,
  proposals,
  overview,
  loading,
  synthesizing,
  onCreate,
  onSaveOverview,
  onSynthesize,
  onAccept,
  onReject,
}: MemoriesViewProps) {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [kind, setKind] = useState<MemoryKind>("decision");
  const [subjectTag, setSubjectTag] = useState("");
  const [anchorTags, setAnchorTags] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [kindFilter, setKindFilter] = useState<MemoryKind | "all">("all");

  // A concept is bound 1:1 to a tag, so it needs a subject tag before it can be
  // created.
  const conceptMissingSubject = kind === "concept" && !subjectTag.trim();
  const canSubmit = !!body.trim() && !submitting && !conceptMissingSubject;

  // The overview lives in its own panel, so keep it out of the memories grid.
  const gridMemories = memories.filter((m) => m.kind !== "overview");
  const visibleMemories =
    kindFilter === "all" ? gridMemories : gridMemories.filter((m) => m.kind === kindFilter);

  async function submit() {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      await onCreate({
        title: title.trim() || undefined,
        body: body.trim(),
        kind,
        subjectTag: kind === "concept" ? subjectTag.trim() : undefined,
        anchorTags: anchorTags
          .split(",")
          .map((t) => t.trim())
          .filter(Boolean),
      });
      setTitle("");
      setBody("");
      setSubjectTag("");
      setAnchorTags("");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 p-4 sm:p-6">
      {/* Project overview — the identity singleton, edited in place */}
      <ProjectOverviewPanel overview={overview} onSave={onSaveOverview} />

      {/* Composer */}
      <section className="rounded-2xl border border-border/70 bg-card p-4">
        <div className="mb-3 flex items-center gap-2">
          <BookMarked className="size-4 text-violet-500" />
          <h2 className="text-sm font-semibold">New memory</h2>
          <p className="text-xs text-muted-foreground">
            Permanent, tag-anchored knowledge — it won&apos;t decay like an issue.
          </p>
        </div>
        <div className="flex flex-col gap-3">
          <Input
            placeholder="Title — the landmark label (optional)"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <Textarea
            placeholder="What did we decide / learn? This becomes durable documentation."
            value={body}
            rows={3}
            onChange={(e) => setBody(e.target.value)}
          />
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex flex-wrap gap-1">
              {KINDS.map((k) => (
                <button
                  key={k}
                  type="button"
                  onClick={() => setKind(k)}
                  className={cn(
                    "rounded-full border px-2.5 py-1 text-[11px] font-medium capitalize transition-colors",
                    kind === k
                      ? "border-violet-500/50 bg-violet-500/10 text-violet-600 dark:text-violet-300"
                      : "border-border/70 text-muted-foreground hover:bg-accent"
                  )}
                >
                  {k}
                </button>
              ))}
            </div>
            {kind === "concept" ? (
              <Input
                placeholder="subject tag — the noun this concept profiles (required)"
                value={subjectTag}
                onChange={(e) => setSubjectTag(e.target.value)}
                className="h-8 max-w-xs flex-1 text-xs"
              />
            ) : (
              <Input
                placeholder="anchor tags (comma separated)"
                value={anchorTags}
                onChange={(e) => setAnchorTags(e.target.value)}
                className="h-8 max-w-xs flex-1 text-xs"
              />
            )}
            <Button
              size="sm"
              className="ml-auto"
              disabled={!canSubmit}
              onClick={() => void submit()}
            >
              {submitting ? "Saving…" : "Create memory"}
            </Button>
          </div>
        </div>
      </section>

      {/* Proposals */}
      <section className="flex flex-col gap-3">
        <div className="flex items-center gap-2">
          <h2 className="text-sm font-semibold">Proposals to review</h2>
          {proposals.length > 0 && (
            <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold text-amber-600 dark:text-amber-300">
              {proposals.length}
            </span>
          )}
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            disabled={synthesizing}
            onClick={() => void onSynthesize()}
          >
            <Sparkles className="size-3.5" />
            {synthesizing ? "Synthesizing…" : "Synthesize from corpus"}
          </Button>
        </div>
        {proposals.length === 0 ? (
          <p className="rounded-2xl border border-dashed border-border/70 px-4 py-6 text-center text-xs text-muted-foreground">
            No pending proposals. Synthesis drafts memories from clusters of closed decisions.
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            {proposals.map((proposal) => (
              <div
                key={proposal.id}
                className="flex flex-col gap-2 rounded-2xl border border-amber-500/30 bg-amber-500/[0.04] p-4"
              >
                <div className="flex items-start gap-3">
                  <div className="min-w-0 flex-1">
                    <h3 className="truncate text-sm font-semibold">{proposal.title}</h3>
                    {proposal.rationale && (
                      <p className="mt-0.5 text-[11px] text-muted-foreground">{proposal.rationale}</p>
                    )}
                  </div>
                  <span className="shrink-0 rounded-full border border-border/70 px-2 py-0.5 text-[10px] tabular-nums text-muted-foreground">
                    {(proposal.confidence ?? 0).toFixed(2)} conf
                  </span>
                </div>
                <p className="line-clamp-3 whitespace-pre-wrap text-xs text-muted-foreground">
                  {proposal.body}
                </p>
                <div className="flex items-center gap-2">
                  <div className="flex flex-wrap gap-1">
                    {(proposal.anchorTags ?? []).map((tag) => (
                      <TagBadge key={tag} tag={tag} compact />
                    ))}
                  </div>
                  <span className="text-[10px] text-muted-foreground">
                    {(proposal.sourceIssueIds ?? []).length} sources
                  </span>
                  <div className="ml-auto flex gap-2">
                    <Button size="sm" variant="ghost" onClick={() => void onReject(proposal.id)}>
                      <X className="size-3.5" /> Reject
                    </Button>
                    <Button size="sm" onClick={() => void onAccept(proposal.id)}>
                      <Check className="size-3.5" /> Accept
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Memories grid */}
      <section className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-sm font-semibold">
            Memories
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              {visibleMemories.length}
            </span>
          </h2>
          <div className="ml-auto flex flex-wrap gap-1">
            {(["all", ...KINDS] as const).map((k) => (
              <button
                key={k}
                type="button"
                onClick={() => setKindFilter(k)}
                className={cn(
                  "rounded-full border px-2.5 py-1 text-[11px] font-medium capitalize transition-colors",
                  kindFilter === k
                    ? "border-violet-500/50 bg-violet-500/10 text-violet-600 dark:text-violet-300"
                    : "border-border/70 text-muted-foreground hover:bg-accent"
                )}
              >
                {k}
              </button>
            ))}
          </div>
        </div>
        {loading ? (
          <p className="text-xs text-muted-foreground">Loading…</p>
        ) : gridMemories.length === 0 ? (
          <p className="rounded-2xl border border-dashed border-border/70 px-4 py-10 text-center text-sm text-muted-foreground">
            No memories yet. Create one above, or synthesize from the corpus.
          </p>
        ) : visibleMemories.length === 0 ? (
          <p className="rounded-2xl border border-dashed border-border/70 px-4 py-10 text-center text-sm text-muted-foreground">
            No {kindFilter} memories yet.
          </p>
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {visibleMemories.map((memory) => (
              <MemoryCard key={memory.id} memory={memory} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
