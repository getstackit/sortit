import { uiAPIURL } from "@/lib/api";
import type { IssueRecord } from "@/lib/issues";
import { getJSON } from "@/lib/http";

export type TagRelevance = {
  tag: string;
  relevance: number;
};

export type PersonTagProfile = {
  person: string;
  issueCount: number;
  tagProfile: TagRelevance[];
};

export type PersonIssueRecommendation = {
  issue: IssueRecord;
  source: "assigned" | "recommended";
  reason: string;
  combinedScore: number;
  semanticScore: number;
  factorScore: number;
  sharedTags: string[];
};

export type PersonDetail = {
  person: string;
  issueCount: number;
  openIssueCount: number;
  closedIssueCount: number;
  tagProfile: TagRelevance[];
  assignedIssues: IssueRecord[];
  nextIssue?: PersonIssueRecommendation;
  recommendedIssues: PersonIssueRecommendation[];
};

export type PersonCorrelation = {
  personA: string;
  personB: string;
  combinedScore: number;
  semanticScore: number;
  factorScore: number;
  sharedTags: string[];
  personAIssueCount: number;
  personBIssueCount: number;
  personAProfile: TagRelevance[];
  personBProfile: TagRelevance[];
};

export type WorkCorrelationsResult = {
  correlations: PersonCorrelation[];
};

export type PeopleListStatus = "open" | "closed" | "all";

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

function normalizeRecommendation(
  recommendation: PersonIssueRecommendation
): PersonIssueRecommendation {
  return {
    ...recommendation,
    issue: normalizeIssueRecord(recommendation.issue),
    sharedTags: Array.isArray(recommendation.sharedTags) ? recommendation.sharedTags : [],
  };
}

export async function fetchPersonProfile(
  person: string,
  status: PeopleListStatus = "all",
  signal?: AbortSignal
): Promise<PersonTagProfile> {
  const params = new URLSearchParams();
  if (status !== "all") {
    params.set("status", status);
  }
  const query = params.toString();
  const url = uiAPIURL(
    `/people/${encodeURIComponent(person)}/profile${query ? `?${query}` : ""}`
  );
  return getJSON<PersonTagProfile>(url, { cache: "no-store", signal });
}

export async function fetchPersonDetail(
  person: string,
  signal?: AbortSignal
): Promise<PersonDetail> {
  const url = uiAPIURL(`/people/${encodeURIComponent(person)}`);
  const payload = await getJSON<PersonDetail>(url, { cache: "no-store", signal });
  return {
    ...payload,
    tagProfile: Array.isArray(payload.tagProfile) ? payload.tagProfile : [],
    assignedIssues: Array.isArray(payload.assignedIssues)
      ? payload.assignedIssues.map(normalizeIssueRecord)
      : [],
    nextIssue: payload.nextIssue ? normalizeRecommendation(payload.nextIssue) : undefined,
    recommendedIssues: Array.isArray(payload.recommendedIssues)
      ? payload.recommendedIssues.map(normalizeRecommendation)
      : [],
  };
}

export async function fetchWorkCorrelations(
  status: PeopleListStatus = "all",
  signal?: AbortSignal
): Promise<WorkCorrelationsResult> {
  const params = new URLSearchParams();
  if (status !== "all") {
    params.set("status", status);
  }
  const query = params.toString();
  const url = uiAPIURL(`/people/correlations${query ? `?${query}` : ""}`);
  return getJSON<WorkCorrelationsResult>(url, { cache: "no-store", signal });
}
