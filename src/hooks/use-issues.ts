import { useSyncExternalStore } from "react";
import useSWR from "swr";
import {
  fetchRevision,
  fetchIssues,
  fetchIssue,
  fetchIssueR2,
  searchIssues,
  type IssueListStatus,
} from "@/lib/issues";
import { uiAPIURL } from "@/lib/api";
import { fetchMapData as fetchSharedMapData } from "@/features/map/api";

// Module-scoped revision store backed by a single EventSource connection
// (with a polling fallback for environments without EventSource, e.g. jsdom
// without a mock). All callers of useBackendRevision share this state.
let revisionState: number | undefined;
const revisionListeners = new Set<() => void>();
let revisionSource: EventSource | null = null;
let revisionPollTimer: ReturnType<typeof setInterval> | null = null;
let revisionStarted = false;

function setRevision(next: number) {
  if (next === revisionState) return;
  revisionState = next;
  for (const listener of revisionListeners) listener();
}

function startRevisionTransport() {
  if (revisionStarted) return;
  if (typeof window === "undefined") return;
  revisionStarted = true;

  if (typeof window.EventSource !== "undefined") {
    const source = new EventSource(uiAPIURL("/revision/stream"));
    source.onmessage = (event) => {
      const parsed = Number(event.data);
      if (Number.isFinite(parsed)) setRevision(parsed);
    };
    // EventSource auto-reconnects on transient errors; no extra handling.
    revisionSource = source;
    return;
  }

  const poll = async () => {
    try {
      setRevision(await fetchRevision());
    } catch {
      // Ignore; next tick retries.
    }
  };
  void poll();
  revisionPollTimer = setInterval(poll, 2000);
}

function subscribeRevision(listener: () => void) {
  revisionListeners.add(listener);
  startRevisionTransport();
  return () => {
    revisionListeners.delete(listener);
  };
}

function getRevisionSnapshot() {
  return revisionState;
}

function getRevisionServerSnapshot(): number | undefined {
  return undefined;
}

export function useBackendRevision() {
  const data = useSyncExternalStore(
    subscribeRevision,
    getRevisionSnapshot,
    getRevisionServerSnapshot
  );
  return { data };
}

// Test-only reset hook; not exported from a public barrel. Clears module state
// between tests that install different EventSource / fetchRevision mocks.
export function __resetBackendRevisionForTests() {
  revisionSource?.close();
  revisionSource = null;
  if (revisionPollTimer !== null) {
    clearInterval(revisionPollTimer);
    revisionPollTimer = null;
  }
  revisionState = undefined;
  revisionStarted = false;
  revisionListeners.clear();
}

export function useIssues(status: IssueListStatus = "open", enabled = true) {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(
    enabled ? ["issues", status, revision] : null,
    ([, currentStatus]: [string, IssueListStatus, number]) => fetchIssues(currentStatus)
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
    ([, currentQuery, currentStatus, currentLimit]: [
      string,
      string,
      IssueListStatus,
      number,
      number,
    ]) =>
      searchIssues(currentQuery, { status: currentStatus, limit: currentLimit })
  );
}

export function useIssue(id?: string | null) {
  const { data: revision = 0 } = useBackendRevision();
  const normalizedID = typeof id === "string" ? id.trim() : "";
  return useSWR(
    normalizedID && normalizedID !== "undefined" ? ["issue", normalizedID, revision] : null,
    ([, issueID]: [string, string, number]) => fetchIssue(issueID)
  );
}

export function useIssueR2(id?: string | null) {
  const { data: revision = 0 } = useBackendRevision();
  const normalizedID = typeof id === "string" ? id.trim() : "";
  return useSWR(
    normalizedID && normalizedID !== "undefined"
      ? ["issue-r2", normalizedID, revision]
      : null,
    ([, issueID]: [string, string, number]) => fetchIssueR2(issueID)
  );
}

export function useIssueMapData() {
  const { data: revision = 0 } = useBackendRevision();
  return useSWR(["issue-detail-map", revision], () =>
    fetchSharedMapData("status=all&edgeThreshold=0.4")
  );
}
