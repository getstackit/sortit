import { apiURL } from "@/lib/api";

export type IssueStatus = "open" | "closed";
export type IssueListStatus = IssueStatus | "all";

export type IssueRecord = {
  id: string;
  raw: string;
  tags: string[];
  createdBy: string;
  createdAt: string;
  status: IssueStatus;
  closedAt?: string | null;
  closedBy?: string;
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

async function parseError(response: Response) {
  let message = `Request failed with ${response.status}`;

  try {
    const payload = (await response.json()) as { error?: string };
    if (payload.error) {
      message = payload.error;
    }
  } catch {}

  return message;
}

export async function fetchIssues(
  status: IssueListStatus = "open",
  signal?: AbortSignal
): Promise<IssueRecord[]> {
  const params = new URLSearchParams();
  params.set("status", status);

  const response = await fetch(apiURL(`/api/v1/issues?${params.toString()}`), {
    cache: "no-store",
    signal,
  });

  if (!response.ok) {
    throw new Error(await parseError(response));
  }

  const payload = (await response.json()) as IssuesResponse;
  return payload.issues;
}

export async function fetchIssue(
  id: string,
  signal?: AbortSignal
): Promise<IssueRecord> {
  const response = await fetch(apiURL(`/api/v1/issues/${encodeURIComponent(id)}`), {
    cache: "no-store",
    signal,
  });

  if (!response.ok) {
    throw new Error(await parseError(response));
  }

  return (await response.json()) as IssueRecord;
}

export async function createIssue(input: CreateIssueInput): Promise<IssueRecord> {
  const response = await fetch(apiURL("/api/v1/issues"), {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(input),
  });

  if (!response.ok) {
    throw new Error(await parseError(response));
  }

  return (await response.json()) as IssueRecord;
}

export async function closeIssue(
  id: string,
  input: CloseIssueInput = {}
): Promise<IssueRecord> {
  const response = await fetch(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/close`),
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    }
  );

  if (!response.ok) {
    throw new Error(await parseError(response));
  }

  return (await response.json()) as IssueRecord;
}

export async function reopenIssue(id: string): Promise<IssueRecord> {
  const response = await fetch(
    apiURL(`/api/v1/issues/${encodeURIComponent(id)}/reopen`),
    {
      method: "POST",
    }
  );

  if (!response.ok) {
    throw new Error(await parseError(response));
  }

  return (await response.json()) as IssueRecord;
}
