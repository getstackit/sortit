import useSWR from "swr";
import { useBackendRevision } from "@/hooks/use-issues";
import {
  fetchMemories,
  fetchMemory,
  fetchMemoryProposals,
  fetchOverview,
  type MemoryListStatus,
  type MemoryProposalStatus,
} from "@/lib/memories";

export function useMemories(status: MemoryListStatus = "active") {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(
    ["memories", status, revision],
    ([, currentStatus]: [string, MemoryListStatus, number]) =>
      fetchMemories(currentStatus)
  );
}

export function useMemory(id?: string | null) {
  const { data: revision = 0 } = useBackendRevision();
  const normalizedID = typeof id === "string" ? id.trim() : "";
  return useSWR(
    normalizedID && normalizedID !== "undefined"
      ? ["memory", normalizedID, revision]
      : null,
    ([, memoryID]: [string, string, number]) => fetchMemory(memoryID)
  );
}

export function useOverview() {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(["memory-overview", revision], () => fetchOverview());
}

export function useMemoryProposals(
  status: MemoryProposalStatus | "all" = "pending"
) {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(
    ["memory-proposals", status, revision],
    ([, currentStatus]: [string, MemoryProposalStatus | "all", number]) =>
      fetchMemoryProposals(currentStatus)
  );
}
