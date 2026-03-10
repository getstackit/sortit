import { apiURL } from "@/lib/api";
import { getJSON } from "@/lib/http";
import type { IssueStatus } from "@/lib/issues";

export type ActivityIssueRecord = {
  id: string;
  raw: string;
  status: IssueStatus;
};

export type ActivityParticipantRecord = {
  issueId: string;
  role: string;
  issue?: ActivityIssueRecord;
};

export type ActivityEventRecord = {
  id: string;
  kind: string;
  createdAt: string;
  createdBy: string;
  issue?: ActivityIssueRecord;
  participants?: ActivityParticipantRecord[];
  body?: string;
};

export type ActivityResponse = {
  events: ActivityEventRecord[];
  nextCursor?: string;
};

export async function fetchActivity(
  options: {
    limit?: number;
    cursor?: string;
    kind?: string;
  } = {},
  signal?: AbortSignal
): Promise<ActivityResponse> {
  const params = new URLSearchParams();
  if (options.limit) {
    params.set("limit", String(options.limit));
  }
  if (options.cursor) {
    params.set("cursor", options.cursor);
  }
  if (options.kind) {
    params.set("kind", options.kind);
  }

  return getJSON<ActivityResponse>(
    apiURL(`/api/v1/activity?${params.toString()}`),
    {
      cache: "no-store",
      signal,
    }
  );
}
