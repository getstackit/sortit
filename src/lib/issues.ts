import { apiURL } from "@/lib/api";

export type IssueRecord = {
  id: string;
  raw: string;
  tags: string[];
  createdBy: string;
  createdAt: string;
};

type IssuesResponse = {
  issues: IssueRecord[];
};

type CreateIssueInput = {
  raw: string;
  tags?: string[];
  createdBy?: string;
};

export async function fetchIssues(signal?: AbortSignal): Promise<IssueRecord[]> {
  const response = await fetch(apiURL("/api/v1/issues"), {
    cache: "no-store",
    signal,
  });

  if (!response.ok) {
    throw new Error(`Request failed with ${response.status}`);
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
    let message = `Request failed with ${response.status}`;

    try {
      const payload = (await response.json()) as { error?: string };
      if (payload.error) {
        message = payload.error;
      }
    } catch {}

    throw new Error(message);
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
    let message = `Request failed with ${response.status}`;

    try {
      const payload = (await response.json()) as { error?: string };
      if (payload.error) {
        message = payload.error;
      }
    } catch {}

    throw new Error(message);
  }

  return (await response.json()) as IssueRecord;
}
