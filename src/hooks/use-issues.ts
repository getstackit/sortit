import useSWR from "swr";
import { fetchIssues, fetchIssue, type IssueListStatus } from "@/lib/issues";
import { fetchMapData as fetchSharedMapData } from "@/features/map/api";

export function useIssues(status: IssueListStatus = "open") {
  return useSWR(["issues", status], () => fetchIssues(status));
}

export function useIssue(id: string) {
  return useSWR(["issue", id], () => fetchIssue(id));
}

export function useIssueMapData() {
  return useSWR("issue-detail-map", () =>
    fetchSharedMapData("status=all&edgeThreshold=0.4")
  );
}
