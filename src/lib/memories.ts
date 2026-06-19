import { uiAPIURL } from "@/lib/api";
import { getJSON, postJSON } from "@/lib/http";
import type { IssueTagScore } from "@/lib/issues";

export type MemoryKind =
  | "decision"
  | "lesson"
  | "constraint"
  | "pattern"
  | "reference";

export type MemoryStatus = "active" | "superseded" | "archived";
export type MemorySource = "manual" | "synthesized";
export type MemoryListStatus = MemoryStatus | "all";

export type MemoryRecord = {
  id: string;
  title: string;
  body: string;
  kind: MemoryKind;
  anchorTags?: string[];
  anchorRegion?: string;
  tagScores?: IssueTagScore[];
  status: MemoryStatus;
  supersededBy?: string;
  source: MemorySource;
  sourceIssueIds?: string[];
  confidence: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  lastReinforcedAt?: string | null;
  reinforcementCount: number;
};

export type MemoryProposalStatus = "pending" | "accepted" | "rejected";

export type MemoryProposalRecord = {
  id: string;
  title: string;
  body: string;
  kind: MemoryKind;
  anchorTags?: string[];
  anchorRegion?: string;
  sourceIssueIds?: string[];
  confidence: number;
  status: MemoryProposalStatus;
  rationale?: string;
  acceptedMemoryId?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateMemoryInput = {
  title?: string;
  body: string;
  kind?: MemoryKind;
  anchorTags?: string[];
  anchorRegion?: string;
};

type MemoriesResponse = { memories: MemoryRecord[] };
type MemoryProposalsResponse = { proposals: MemoryProposalRecord[] };

function normalizeMemory(record: MemoryRecord): MemoryRecord {
  return {
    ...record,
    anchorTags: Array.isArray(record.anchorTags) ? record.anchorTags : [],
    sourceIssueIds: Array.isArray(record.sourceIssueIds) ? record.sourceIssueIds : [],
    tagScores: Array.isArray(record.tagScores) ? record.tagScores : undefined,
  };
}

export async function fetchMemories(
  status: MemoryListStatus = "active",
  signal?: AbortSignal
): Promise<MemoryRecord[]> {
  const params = new URLSearchParams();
  params.set("status", status);
  const payload = await getJSON<MemoriesResponse>(
    uiAPIURL(`/memories?${params.toString()}`),
    { cache: "no-store", signal }
  );
  return (payload.memories ?? []).map(normalizeMemory);
}

export async function fetchMemory(
  id: string,
  signal?: AbortSignal
): Promise<MemoryRecord> {
  const payload = await getJSON<MemoryRecord>(
    uiAPIURL(`/memories/${encodeURIComponent(id)}`),
    { cache: "no-store", signal }
  );
  return normalizeMemory(payload);
}

export async function createMemory(input: CreateMemoryInput): Promise<MemoryRecord> {
  const payload = await postJSON<MemoryRecord, CreateMemoryInput>(
    uiAPIURL("/memories"),
    input
  );
  return normalizeMemory(payload);
}

export async function supersedeMemory(
  id: string,
  supersededBy = ""
): Promise<MemoryRecord> {
  const payload = await postJSON<MemoryRecord, { supersededBy: string }>(
    uiAPIURL(`/memories/${encodeURIComponent(id)}/supersede`),
    { supersededBy }
  );
  return normalizeMemory(payload);
}

export async function archiveMemory(id: string): Promise<MemoryRecord> {
  const payload = await postJSON<MemoryRecord, Record<string, never>>(
    uiAPIURL(`/memories/${encodeURIComponent(id)}/archive`),
    {}
  );
  return normalizeMemory(payload);
}

export async function fetchMemoryProposals(
  status: MemoryProposalStatus | "all" = "pending",
  signal?: AbortSignal
): Promise<MemoryProposalRecord[]> {
  const params = new URLSearchParams();
  params.set("status", status);
  const payload = await getJSON<MemoryProposalsResponse>(
    uiAPIURL(`/memories/proposals?${params.toString()}`),
    { cache: "no-store", signal }
  );
  return payload.proposals ?? [];
}

export async function synthesizeMemoryProposals(): Promise<MemoryProposalRecord[]> {
  const payload = await postJSON<MemoryProposalsResponse, Record<string, never>>(
    uiAPIURL("/memories/proposals/synthesize"),
    {}
  );
  return payload.proposals ?? [];
}

export async function acceptMemoryProposal(id: string): Promise<MemoryRecord> {
  const payload = await postJSON<MemoryRecord, Record<string, never>>(
    uiAPIURL(`/memories/proposals/${encodeURIComponent(id)}/accept`),
    {}
  );
  return normalizeMemory(payload);
}

export async function rejectMemoryProposal(
  id: string
): Promise<MemoryProposalRecord> {
  return postJSON<MemoryProposalRecord, Record<string, never>>(
    uiAPIURL(`/memories/proposals/${encodeURIComponent(id)}/reject`),
    {}
  );
}
