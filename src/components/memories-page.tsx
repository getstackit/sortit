"use client";

import { useState } from "react";
import { toast } from "sonner";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { MemoriesView } from "@/components/memories-view";
import { SiteHeader } from "@/components/site-header";
import { useMemories, useMemoryProposals } from "@/hooks/use-memories";
import {
  acceptMemoryProposal,
  createMemory,
  rejectMemoryProposal,
  synthesizeMemoryProposals,
  type CreateMemoryInput,
} from "@/lib/memories";

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function MemoriesPage() {
  const { data: memories = [], isLoading } = useMemories("active");
  const { data: proposals = [] } = useMemoryProposals("pending");
  const [synthesizing, setSynthesizing] = useState(false);

  async function handleCreate(input: CreateMemoryInput) {
    try {
      await createMemory(input);
      toast.success("Memory created");
    } catch (error) {
      toast.error(errorMessage(error, "Failed to create memory"));
    }
  }

  async function handleSynthesize() {
    setSynthesizing(true);
    try {
      const created = await synthesizeMemoryProposals();
      toast.success(
        created.length
          ? `${created.length} proposal${created.length === 1 ? "" : "s"} drafted`
          : "No new proposals — corpus already covered"
      );
    } catch (error) {
      toast.error(errorMessage(error, "Failed to synthesize proposals"));
    } finally {
      setSynthesizing(false);
    }
  }

  async function handleAccept(id: string) {
    try {
      await acceptMemoryProposal(id);
      toast.success("Accepted — memory created");
    } catch (error) {
      toast.error(errorMessage(error, "Failed to accept proposal"));
    }
  }

  async function handleReject(id: string) {
    try {
      await rejectMemoryProposal(id);
      toast.success("Proposal rejected");
    } catch (error) {
      toast.error(errorMessage(error, "Failed to reject proposal"));
    }
  }

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="Memories"
        eyebrow="Knowledge"
        subtitle="Permanent, tag-anchored decisions that reinforce instead of decaying."
        meta={
          <span className="text-xs text-muted-foreground tabular-nums">
            {memories.length} active
          </span>
        }
      />
      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <MemoriesView
          memories={memories}
          proposals={proposals}
          loading={isLoading}
          synthesizing={synthesizing}
          onCreate={handleCreate}
          onSynthesize={handleSynthesize}
          onAccept={handleAccept}
          onReject={handleReject}
        />
      </div>
    </AppShell>
  );
}
