import useSWR from "swr";
import {
  fetchIssues,
  fetchIssue,
  searchIssues,
  type IssueListStatus,
} from "@/lib/issues";
import { fetchMapData as fetchSharedMapData } from "@/features/map/api";

export function useIssues(status: IssueListStatus = "open", enabled = true) {
  return useSWR(enabled ? ["issues", status] : null, () => fetchIssues(status));
}

export function useIssueSearch(
  query: string,
  status: IssueListStatus = "open",
  limit = 8
) {
  const trimmed = query.trim();
  return useSWR(
    trimmed ? ["issues-search", trimmed, status, limit] : null,
    () => searchIssues(trimmed, { status, limit })
  );
}

export function useIssue(id: string) {
  return useSWR(["issue", id], () => fetchIssue(id));
}

export function useIssueMapData() {
  return useSWR("issue-detail-map", () =>
    fetchSharedMapData("status=all&edgeThreshold=0.4")
  );
}
