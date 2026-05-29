import useSWR from "swr";

import { fetchIssueXRay, type IssueXRay } from "@/lib/issue-xray";

export function useIssueXRay(id: string | null) {
  return useSWR<IssueXRay, Error>(
    id ? (["issue-xray", id] as const) : null,
    () => fetchIssueXRay(id!),
  );
}
