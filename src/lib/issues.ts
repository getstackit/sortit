import { apiURL } from "@/lib/api";
import { getJSON, postJSON } from "@/lib/http";

export type IssueStatus = "open" | "closed";
export type IssueEnrichmentStatus = "complete" | "pending" | "failed";
export type IssueListStatus = IssueStatus | "all";
export type IssuePostKind =
  | "report"
  | "refinement"
  | "progress"
  | "closed"
  | "reopened"
  | "assigned";
export type IssueLinkType =
  | "parent_of"
  | "child_of"
  | "merged_into"
  | "derived_from"
  | "related_to"
  | "duplicate_of";
export type IssueOperationKind = "split" | "combine" | "link";

export type IssuePostRecord = {
  id: string;
  issueId?: string;
  raw: string;
  createdBy: string;
  createdAt: string;
  sequence: number;
  kind?: IssuePostKind;
};

export type IssueTagScore = {
  tag: string;
  relevance: number;
};

export type IssueReferenceRecord = {
  id: string;
  raw: string;
  status: IssueStatus;
};

export type IssueLinkRecord = {
  id: string;
  type: IssueLinkType;
  sourceIssueId: string;
  targetIssueId: string;
  direction?: "incoming" | "outgoing";
  createdBy: string;
  createdAt: string;
  note?: string;
  operationId?: string;
  relatedIssue?: IssueReferenceRecord;
};

export type IssueOperationParticipantRecord = {
  issueId: string;
  role: string;
  issue?: IssueReferenceRecord;
};

export type IssueOperationRecord = {
  id: string;
  kind: IssueOperationKind;
  createdBy: string;
  createdAt: string;
  note?: string;
  participants?: IssueOperationParticipantRecord[];
};

export type IssueSearchQuery = {
  raw: string;
  tags: IssueTagScore[];
};

export type SearchIssueRecord = {
  id: string;
  raw: string;
  status: IssueStatus;
  tags: IssueTagScore[];
  semanticSimilarity: number;
  factorSimilarity: number;
  combinedSimilarity: number;
  reason: string;
};

export type IssueSearchResponse = {
  query: IssueSearchQuery;
  relatedIssues: SearchIssueRecord[];
};

const ISSUE_ULID_PATTERN = /^[0-9A-HJKMNP-TV-Z]{26}$/i;
const LEGACY_ISSUE_ID_PATTERN = /^issue-[a-z0-9][a-z0-9-]*$/i;

export type IssueRecord = {
  id: string;
  raw: string;
  tags: string[];
  tagScores?: IssueTagScore[];
  createdBy: string;
  createdAt: string;
  status: IssueStatus;
  closedAt?: string | null;
  closedBy?: string;
  assignedTo?: string;
  discussion?: IssuePostRecord[];
  links?: IssueLinkRecord[];
  operations?: IssueOperationRecord[];
  enrichmentStatus?: IssueEnrichmentStatus;
  enrichmentError?: string;
  enrichmentTargetSequence?: number;
};

type IssuesResponse = {
  issues: IssueRecord[];
};

type RevisionResponse = {
  revision: number;
};

type CreateIssueInput = {
  raw: string;
  tags?: string[];
  createdBy?: string;
};

type CloseIssueInput = {
  closedBy?: string;
};

type RefineIssueInput = {
  raw: string;
  createdBy?: string;
};

type ProgressIssueInput = {
  raw: string;
  createdBy?: string;
};

type AssignIssueInput = {
  assignedTo: string;
};

type SearchIssuesOptions = {
  status?: IssueListStatus;
  limit?: number;
};

export async function fetchIssues(
  status: IssueListStatus = "open",
  signal?: AbortSignal
): Promise<IssueRecord[]> {
  const params = new URLSearchParams();
  params.set("status", status);

  const payload = await getJSON<IssuesResponse>(
    apiURL(`/api/v1/issues?${params.toString()}`),
    {
    cache: "no-store",
    signal,
    }
  );
  return payload.issues;
}

export async function fetchIssue(
  id: string,
  signal?: AbortSignal
): Promise<IssueRecord> {
  return getJSON<IssueRecord>(apiURL(`/api/v1/issues/${encodeURIComponent(id)}`), {
    cache: "no-store",
    signal,
  });
}

export async function fetchRevision(signal?: AbortSignal): Promise<number> {
  const payload = await getJSON<RevisionResponse>(apiURL("/api/v1/revision"), {
    cache: "no-store",
    signal,
  });
  return payload.revision;
}

export async function searchIssues(
  query: string,
  options: SearchIssuesOptions = {},
  signal?: AbortSignal
): Promise<IssueSearchResponse> {
  const params = new URLSearchParams();
  params.set("q", query);
  params.set("status", options.status ?? "open");
  if (options.limit) {
    params.set("limit", String(options.limit));
  }

  return getJSON<IssueSearchResponse>(
    apiURL(`/api/v1/issues/search?${params.toString()}`),
    {
      cache: "no-store",
      signal,
    }
  );
}

export function parseIssueID(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }

  if (ISSUE_ULID_PATTERN.test(trimmed)) {
    return trimmed.toUpperCase();
  }

  if (LEGACY_ISSUE_ID_PATTERN.test(trimmed)) {
    return trimmed;
  }

  return null;
}

export function issueHrefFromText(value: string): string | null {
  const issueID = parseIssueID(value);
  if (!issueID) {
    return null;
  }

  return `/issues/${encodeURIComponent(issueID)}`;
}

export async function createIssue(input: CreateIssueInput): Promise<IssueRecord> {
  return postJSON<IssueRecord, CreateIssueInput>(apiURL("/api/v1/issues"), input);
}

export async function closeIssue(
  id: string,
  input: CloseIssueInput = {}
): Promise<IssueRecord> {
  return postJSON<IssueRecord, CloseIssueInput>(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/close`),
    input
  );
}

export async function reopenIssue(id: string): Promise<IssueRecord> {
  return postJSON<IssueRecord, Record<string, never>>(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/reopen`),
    {}
  );
}

export async function refineIssue(
  id: string,
  input: RefineIssueInput
): Promise<IssueRecord> {
  return postJSON<IssueRecord, RefineIssueInput>(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/refine`),
    input
  );
}

export async function assignIssue(
  id: string,
  input: AssignIssueInput
): Promise<IssueRecord> {
  return postJSON<IssueRecord, AssignIssueInput>(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/assign`),
    input
  );
}

export async function progressIssue(
  id: string,
  input: ProgressIssueInput
): Promise<IssueRecord> {
  return postJSON<IssueRecord, ProgressIssueInput>(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/progress`),
    input
  );
}
