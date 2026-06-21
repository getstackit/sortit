"use client";

import { useState } from "react";
import { Compass } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { MemoryRecord } from "@/lib/memories";

export type ProjectOverviewPanelProps = {
  overview: MemoryRecord | null;
  onSave: (body: string) => Promise<void> | void;
};

// ProjectOverviewPanel is the in-product editor for the project's identity
// singleton. The overview grounds issue tagging (the analyzer reads it to tag
// every issue within the project's identity), so it gets a dedicated, always-
// visible home at the top of the memories view rather than living in the generic
// create picker.
export function ProjectOverviewPanel({ overview, onSave }: ProjectOverviewPanelProps) {
  const overviewBody = overview?.body ?? "";
  const [body, setBody] = useState(overviewBody);
  const [saving, setSaving] = useState(false);

  // Re-seed the editor when the stored overview changes (initial load, or after
  // a save supersedes it with a new record). This is the render-time "adjust
  // state when a prop changes" pattern — no effect, so no cascading render.
  const [lastSeeded, setLastSeeded] = useState(overviewBody);
  if (overviewBody !== lastSeeded) {
    setLastSeeded(overviewBody);
    setBody(overviewBody);
  }

  const trimmed = body.trim();
  const dirty = trimmed !== overviewBody.trim();
  const canSave = !!trimmed && dirty && !saving;

  async function save() {
    if (!canSave) return;
    setSaving(true);
    try {
      await onSave(trimmed);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="rounded-2xl border border-teal-500/30 bg-teal-500/[0.04] p-4">
      <div className="mb-2 flex items-center gap-2">
        <Compass className="size-4 text-teal-500" />
        <h2 className="text-sm font-semibold">Project overview</h2>
        <p className="text-xs text-muted-foreground">
          One paragraph on what this project is — it grounds how every issue is tagged.
        </p>
      </div>
      <Textarea
        placeholder="Describe what this project is: its purpose and core domain. The tagger reads this to tag issues within the project's identity."
        value={body}
        rows={3}
        onChange={(e) => setBody(e.target.value)}
      />
      <div className="mt-2 flex items-center gap-3">
        <span className="text-[10px] text-muted-foreground">
          {overview ? "Saving replaces the current overview." : "No overview set yet."}
        </span>
        <Button
          size="sm"
          className="ml-auto"
          disabled={!canSave}
          onClick={() => void save()}
        >
          {saving ? "Saving…" : "Save overview"}
        </Button>
      </div>
    </section>
  );
}
