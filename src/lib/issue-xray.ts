import { uiAPIURL } from "@/lib/api";
import { getJSON } from "@/lib/http";

export type ConnectednessScore = {
  links: number;
  operations: number;
  score: number;
};

export type LifecyclePosition = {
  label: string;
  refinements: number;
  ageDays: number;
};

export type IssueXRay = {
  specificity?: number | null;
  coherence?: number | null;
  definition?: number | null;
  connectedness: ConnectednessScore;
  lifecycle: LifecyclePosition;
};

export async function fetchIssueXRay(
  id: string,
  signal?: AbortSignal,
): Promise<IssueXRay> {
  return getJSON<IssueXRay>(
    uiAPIURL(`/issues/${encodeURIComponent(id)}/xray`),
    { cache: "no-store", signal },
  );
}
