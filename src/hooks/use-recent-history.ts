"use client";

import { useSyncExternalStore } from "react";
import type { IssueRecord, IssueStatus } from "@/lib/issues";
import type { TagRecord } from "@/lib/tags";

export const RECENT_HISTORY_STORAGE_KEY = "sortit-recent-history";
export const MAX_RECENT_HISTORY_ITEMS = 8;

export type RecentIssueItem = {
  kind: "issue";
  id: string;
  raw: string;
  status: IssueStatus;
  tags: string[];
  viewedAt: number;
};

export type RecentTagItem = {
  kind: "tag";
  name: string;
  description?: string;
  viewedAt: number;
};

export type RecentHistoryItem = RecentIssueItem | RecentTagItem;

let cachedStorageValue: string | null = null;
let cachedSnapshot: RecentHistoryItem[] = [];

function subscribe(callback: () => void) {
  window.addEventListener("storage", callback);
  return () => window.removeEventListener("storage", callback);
}

function emitChange() {
  window.dispatchEvent(new Event("storage"));
}

function isRecentIssueItem(value: unknown): value is RecentIssueItem {
  if (!value || typeof value !== "object") {
    return false;
  }

  const item = value as Partial<RecentIssueItem>;
  return (
    item.kind === "issue" &&
    typeof item.id === "string" &&
    typeof item.raw === "string" &&
    (item.status === "open" || item.status === "closed") &&
    Array.isArray(item.tags) &&
    typeof item.viewedAt === "number"
  );
}

function isRecentTagItem(value: unknown): value is RecentTagItem {
  if (!value || typeof value !== "object") {
    return false;
  }

  const item = value as Partial<RecentTagItem>;
  return (
    item.kind === "tag" &&
    typeof item.name === "string" &&
    typeof item.viewedAt === "number" &&
    (typeof item.description === "string" || typeof item.description === "undefined")
  );
}

function normalizeRecentHistory(value: unknown): RecentHistoryItem[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value
    .filter((item): item is RecentHistoryItem => isRecentIssueItem(item) || isRecentTagItem(item))
    .sort((left, right) => right.viewedAt - left.viewedAt)
    .slice(0, MAX_RECENT_HISTORY_ITEMS);
}

function readStorage(): RecentHistoryItem[] {
  if (typeof window === "undefined") {
    return [];
  }

  try {
    const raw = window.localStorage.getItem(RECENT_HISTORY_STORAGE_KEY);
    if (raw === cachedStorageValue) {
      return cachedSnapshot;
    }

    cachedStorageValue = raw;
    cachedSnapshot = raw ? normalizeRecentHistory(JSON.parse(raw)) : [];
    return cachedSnapshot;
  } catch {
    cachedStorageValue = null;
    cachedSnapshot = [];
    return cachedSnapshot;
  }
}

function writeStorage(items: RecentHistoryItem[]) {
  const nextValue = JSON.stringify(items.slice(0, MAX_RECENT_HISTORY_ITEMS));
  cachedStorageValue = nextValue;
  cachedSnapshot = normalizeRecentHistory(JSON.parse(nextValue));
  window.localStorage.setItem(RECENT_HISTORY_STORAGE_KEY, nextValue);
  emitChange();
}

function sameEntry(left: RecentHistoryItem, right: RecentHistoryItem) {
  if (left.kind !== right.kind) {
    return false;
  }

  if (left.kind === "issue" && right.kind === "issue") {
    return left.id === right.id;
  }

  if (left.kind === "tag" && right.kind === "tag") {
    return left.name.localeCompare(right.name, undefined, { sensitivity: "accent" }) === 0;
  }

  return false;
}

function rememberEntry(nextItem: RecentHistoryItem) {
  if (typeof window === "undefined") {
    return;
  }

  const deduped = readStorage().filter((item) => !sameEntry(item, nextItem));
  writeStorage([nextItem, ...deduped]);
}

export function readRecentHistory(): RecentHistoryItem[] {
  return readStorage();
}

export function clearRecentHistory() {
  if (typeof window === "undefined") {
    return;
  }

  cachedStorageValue = null;
  cachedSnapshot = [];
  window.localStorage.removeItem(RECENT_HISTORY_STORAGE_KEY);
  emitChange();
}

export function rememberRecentIssue(
  issue: Pick<IssueRecord, "id" | "raw" | "status" | "tags">
) {
  rememberEntry({
    kind: "issue",
    id: issue.id,
    raw: issue.raw,
    status: issue.status,
    tags: issue.tags.slice(0, 3),
    viewedAt: Date.now(),
  });
}

export function rememberRecentTag(
  tag: Pick<TagRecord, "name" | "description">
) {
  rememberEntry({
    kind: "tag",
    name: tag.name,
    description: tag.description,
    viewedAt: Date.now(),
  });
}

export function useRecentHistory() {
  return useSyncExternalStore(subscribe, readRecentHistory, () => []);
}
