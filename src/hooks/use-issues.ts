import useSWR from "swr";
import {
  fetchRevision,
  fetchIssues,
  fetchIssue,
  searchIssues,
  type IssueListStatus,
} from "@/lib/issues";
import { fetchMapData as fetchSharedMapData } from "@/features/map/api";

export function useBackendRevision() {
  return useSWR("backend-revision", () => fetchRevision(), {
    refreshInterval: 2000,
    revalidateOnFocus: true,
    revalidateOnReconnect: true,
  });
}

export function useIssues(status: IssueListStatus = "open", enabled = true) {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(
    enabled ? ["issues", status, revision] : null,
    (_key: string, currentStatus: IssueListStatus) => fetchIssues(currentStatus)
  );
}

export function useIssueSearch(
  query: string,
  status: IssueListStatus = "open",
  limit = 8
) {
  const trimmed = query.trim();
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(
    trimmed ? ["issues-search", trimmed, status, limit, revision] : null,
    (
      _key: string,
      currentQuery: string,
      currentStatus: IssueListStatus,
      currentLimit: number
    ) =>
      searchIssues(currentQuery, { status: currentStatus, limit: currentLimit })
  );
}

export function useIssue(id: string) {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(["issue", id, revision], (_key: string, issueID: string) =>
    fetchIssue(issueID)
  );
}

export function useIssueMapData() {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(["issue-detail-map", revision], () =>
    fetchSharedMapData("status=all&edgeThreshold=0.4")
  );
}
