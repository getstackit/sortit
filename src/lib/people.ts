import { uiAPIURL } from "@/lib/api";
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
