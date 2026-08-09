import { uiAPIURL } from "@/lib/api";
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

export type EvidenceRange = {
  start: number;
  end: number;
  text?: string;
  source?: string;
  kind?: string;
};

export type NegationProvenance =
  | "analyzer-negation"
  | "verifier-dominance"
  | "dismiss"
  | "cooccurrence";

export type IssueTagScore = {
  tag: string;
  relevance: number;
  suggested?: boolean;
  description?: string;
  candidateSources?: string[];
  alignment?: number | null;
  specificity?: number | null;
  verificationVerdict?: "keep" | "down-rank" | "flagged";
  verificationReason?: string;
  dominatedBy?: string;
  dominanceGap?: number | null;
  evidence?: EvidenceRange[];
  negation?: number | null;
  negationProvenance?: NegationProvenance;
  negationEvidence?: EvidenceRange[];
  negationReason?: string;
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

export type IssueLifecycleMetricsRecord = {
  churn?: number;
  maturity?: number;
  velocity?: number;
  snapshotCount?: number;
  transitionCount?: number;
  refinementCount?: number;
  progressCount?: number;
  recentActivityCount?: number;
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
  closedReason?: string;
  closedReasonNote?: string;
  assignedTo?: string;
  discussion?: IssuePostRecord[];
  links?: IssueLinkRecord[];
  operations?: IssueOperationRecord[];
  enrichmentStatus?: IssueEnrichmentStatus;
  enrichmentError?: string;
  enrichmentTargetSequence?: number;
  lifecycleMetrics?: IssueLifecycleMetricsRecord;
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
  reason?: string;
  reasonNote?: string;
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

function normalizeIssueRecord(record: IssueRecord): IssueRecord {
  return {
    ...record,
    tags: Array.isArray(record.tags) ? record.tags : [],
    tagScores: Array.isArray(record.tagScores) ? record.tagScores : undefined,
    discussion: Array.isArray(record.discussion) ? record.discussion : undefined,
    links: Array.isArray(record.links) ? record.links : undefined,
    operations: Array.isArray(record.operations) ? record.operations : undefined,
  };
}

export async function fetchIssues(
  status: IssueListStatus = "open",
  signal?: AbortSignal
): Promise<IssueRecord[]> {
  const params = new URLSearchParams();
  params.set("status", status);

  const payload = await getJSON<IssuesResponse>(
    uiAPIURL(`/issues?${params.toString()}`),
    {
    cache: "no-store",
    signal,
    }
  );
  return payload.issues.map(normalizeIssueRecord);
}

export async function fetchIssue(
  id: string,
  signal?: AbortSignal
): Promise<IssueRecord> {
  const payload = await getJSON<IssueRecord>(uiAPIURL(`/issues/${encodeURIComponent(id)}`), {
    cache: "no-store",
    signal,
  });
  return normalizeIssueRecord(payload);
}

export async function fetchRevision(signal?: AbortSignal): Promise<number> {
  const payload = await getJSON<RevisionResponse>(uiAPIURL("/revision"), {
    cache: "no-store",
    signal,
  });
  return payload.revision;
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
  const payload = await postJSON<IssueRecord, CreateIssueInput>(uiAPIURL("/issues"), input);
  return normalizeIssueRecord(payload);
}

export async function closeIssue(
  id: string,
  input: CloseIssueInput = {}
): Promise<IssueRecord> {
  const payload = await postJSON<IssueRecord, CloseIssueInput>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/close`),
    input
  );
  return normalizeIssueRecord(payload);
}

export async function reopenIssue(id: string): Promise<IssueRecord> {
  const payload = await postJSON<IssueRecord, Record<string, never>>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/reopen`),
    {}
  );
  return normalizeIssueRecord(payload);
}

export async function refineIssue(
  id: string,
  input: RefineIssueInput
): Promise<IssueRecord> {
  const payload = await postJSON<IssueRecord, RefineIssueInput>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/refine`),
    input
  );
  return normalizeIssueRecord(payload);
}

export async function assignIssue(
  id: string,
  input: AssignIssueInput
): Promise<IssueRecord> {
  const payload = await postJSON<IssueRecord, AssignIssueInput>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/assign`),
    input
  );
  return normalizeIssueRecord(payload);
}

export async function progressIssue(
  id: string,
  input: ProgressIssueInput
): Promise<IssueRecord> {
  const payload = await postJSON<IssueRecord, ProgressIssueInput>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/progress`),
    input
  );
  return normalizeIssueRecord(payload);
}

export type IssueR2Tag = {
  tag: string;
  relevance: number;
  alignment: number;
};

export type IssueR2ResidualTagMatch = {
  tag: string;
  similarity: number;
  assigned: boolean;
};

export type IssueR2ResidualNeighbor = {
  id: string;
  raw: string;
  similarity: number;
  r2: number;
  tags: string[];
};

export type IssueR2Result = {
  id: string;
  raw: string;
  r2: number;
  tagCount: number;
  tags: IssueR2Tag[];
  embeddingDim: number;
  explainedNorm: number;
  factorAlignment: number;
  residualNorm: number;
  nearestResidualTags: IssueR2ResidualTagMatch[];
  residualNeighbors: IssueR2ResidualNeighbor[];
  diagnosis: string[];
  skipped: boolean;
  skipReason?: string;
};

export async function fetchIssueR2(
  id: string,
  signal?: AbortSignal
): Promise<IssueR2Result> {
  return getJSON<IssueR2Result>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/r2`),
    { cache: "no-store", signal }
  );
}

export async function reEnrichIssue(id: string): Promise<IssueRecord> {
  const payload = await postJSON<IssueRecord, Record<string, never>>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/re-enrich`),
    {}
  );
  return normalizeIssueRecord(payload);
}
