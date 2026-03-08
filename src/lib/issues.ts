import { apiURL } from "@/lib/api";
import { getJSON, postJSON } from "@/lib/http";

export type IssueStatus = "open" | "closed";
export type IssueListStatus = IssueStatus | "all";
export type IssuePostKind = "report" | "refinement" | "progress";

export type IssuePostRecord = {
  id: string;
  issueId?: string;
  raw: string;
  createdBy: string;
  createdAt: string;
  sequence: number;
  kind?: IssuePostKind;
};

export type IssueRecord = {
  id: string;
  raw: string;
  tags: string[];
  createdBy: string;
  createdAt: string;
  status: IssueStatus;
  closedAt?: string | null;
  closedBy?: string;
  discussion?: IssuePostRecord[];
};

type IssuesResponse = {
  issues: IssueRecord[];
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

export async function progressIssue(
  id: string,
  input: ProgressIssueInput
): Promise<IssueRecord> {
  return postJSON<IssueRecord, ProgressIssueInput>(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/progress`),
    input
  );
}
