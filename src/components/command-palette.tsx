"use client";

import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Dialog } from "@base-ui/react/dialog";
import { FileTextIcon, LoaderIcon, SearchIcon, TagIcon } from "lucide-react";
import { UNIFIED_SEARCH_LIMIT, useUnifiedSearch } from "@/hooks/use-search";
import { useRecentHistory } from "@/hooks/use-recent-history";
import { issueHrefFromText } from "@/lib/issues";
import { tagHref } from "@/lib/tags";
import { entityStyle } from "@/lib/entity-colors";
import type { IssueStatus, SearchIssueRecord } from "@/lib/issues";
import type { RelatedTag } from "@/lib/search";
import { cn } from "@/lib/utils";

type CommandPaletteProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type ResultItem =
  | { kind: "issue"; issue: SearchIssueRecord }
  | { kind: "tag"; tag: RelatedTag };

type PaletteItem =
  | {
      kind: "issue";
      source: "search" | "recent";
      id: string;
      raw: string;
      status: IssueStatus;
      tags: Array<{ tag: string; relevance: number }>;
      similarity?: number;
    }
  | {
      kind: "tag";
      source: "search" | "recent";
      name: string;
      description?: string;
      similarity?: number;
    };

export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const router = useRouter();
  const [searchText, setSearchText] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const recentItems = useRecentHistory();

  const deferredSearchText = useDeferredValue(searchText);
  const activeQuery = deferredSearchText.trim();

  const { data, isLoading } = useUnifiedSearch(activeQuery);

  const searchItems = useMemo<ResultItem[]>(() => {
    const next: ResultItem[] = [];
    if (!data) {
      return next;
    }

    for (const issue of data.issues) {
      next.push({ kind: "issue", issue });
    }
    for (const tag of data.relatedTags) {
      next.push({ kind: "tag", tag });
    }

    return next;
  }, [data]);

  const recentPaletteItems = useMemo<PaletteItem[]>(
    () =>
      recentItems.map((item) =>
        item.kind === "issue"
          ? {
              kind: "issue",
              source: "recent",
              id: item.id,
              raw: item.raw,
              status: item.status,
              tags: item.tags.map((tag) => ({ tag, relevance: 1 })),
            }
          : {
              kind: "tag",
              source: "recent",
              name: item.name,
              description: item.description,
            }
      ),
    [recentItems]
  );

  const items = useMemo<PaletteItem[]>(
    () =>
      activeQuery.length === 0
        ? recentPaletteItems
        : searchItems.map((item) =>
            item.kind === "issue"
              ? {
                  kind: "issue",
                  source: "search",
                  id: item.issue.id,
                  raw: item.issue.raw,
                  status: item.issue.status,
                  tags: item.issue.tags,
                  similarity: item.issue.combinedSimilarity,
                }
              : {
                  kind: "tag",
                  source: "search",
                  name: item.tag.name,
                  description: item.tag.description,
                  similarity: item.tag.similarity,
                }
          ),
    [activeQuery.length, recentPaletteItems, searchItems]
  );

  const resolvedActiveIndex = items.length === 0 ? 0 : Math.min(activeIndex, items.length - 1);

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        setSearchText("");
        setActiveIndex(0);
      }
      onOpenChange(nextOpen);
    },
    [onOpenChange]
  );

  const navigateToDirectIssue = useCallback(
    (value: string) => {
      const href = issueHrefFromText(value);
      if (!href) {
        return false;
      }

      router.push(href);
      handleOpenChange(false);
      return true;
    },
    [handleOpenChange, router]
  );

  const navigate = useCallback(
    (item: PaletteItem) => {
      if (item.kind === "issue") {
        router.push(`/issues/${item.id}`);
      } else {
        router.push(tagHref(item.name));
      }
      handleOpenChange(false);
    },
    [handleOpenChange, router]
  );

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setActiveIndex((prev) => (prev + 1) % Math.max(items.length, 1));
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        setActiveIndex((prev) =>
          prev <= 0 ? Math.max(items.length - 1, 0) : prev - 1
        );
      } else if (event.key === "Enter") {
        event.preventDefault();
        if (navigateToDirectIssue(searchText)) {
          return;
        }
        const item = items[resolvedActiveIndex];
        if (item) {
          navigate(item);
        }
      }
    },
    [items, navigate, navigateToDirectIssue, resolvedActiveIndex, searchText]
  );

  useEffect(() => {
    const activeEl = listRef.current?.querySelector("[data-active='true']");
    if (activeEl) {
      activeEl.scrollIntoView({ block: "nearest" });
    }
  }, [resolvedActiveIndex]);

  return (
    <Dialog.Root open={open} onOpenChange={handleOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-50 bg-black/40 transition-opacity data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
        <Dialog.Viewport className="fixed inset-0 z-50 flex items-start justify-center pt-[12vh]">
          <Dialog.Popup className="app-surface w-full max-w-xl rounded-[1.5rem] p-0 transition-all data-[ending-style]:scale-95 data-[ending-style]:opacity-0 data-[starting-style]:scale-95 data-[starting-style]:opacity-0">
            <div className="flex items-center gap-3 border-b border-border/60 px-4 py-3">
              <SearchIcon className="size-4 shrink-0 text-muted-foreground" />
              <input
                value={searchText}
                onChange={(e) => {
                  setSearchText(e.target.value);
                  setActiveIndex(0);
                }}
                onPaste={(event) => {
                  const pastedText = event.clipboardData.getData("text");
                  if (navigateToDirectIssue(pastedText)) {
                    event.preventDefault();
                  }
                }}
                onKeyDown={handleKeyDown}
                placeholder="Search issues and tags..."
                autoFocus
                className="min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground/50"
              />
              {isLoading && (
                <LoaderIcon className="size-4 shrink-0 animate-spin text-muted-foreground" />
              )}
            </div>

            <div
              ref={listRef}
              className="max-h-[min(60vh,24rem)] overflow-y-auto"
            >
              {activeQuery.length === 0 && (
                items.length > 0 ? (
                  <div className="py-2">
                    <div className="px-4 py-2 text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground/70">
                      Recently viewed
                    </div>
                    {items.map((item, index) => {
                      const isActive = index === resolvedActiveIndex;

                      if (item.kind === "issue") {
                        return (
                          <button
                            key={`recent-issue-${item.id}`}
                            type="button"
                            data-active={isActive}
                            onClick={() => navigate(item)}
                            onMouseEnter={() => setActiveIndex(index)}
                            className={cn(
                              "flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors",
                              isActive
                                ? "bg-accent text-accent-foreground"
                                : "text-foreground"
                            )}
                          >
                            <FileTextIcon className="size-4 shrink-0 text-muted-foreground" />
                            <div className="min-w-0 flex-1">
                              <p className="truncate text-sm">{item.raw}</p>
                              <div className="mt-1 flex items-center gap-1.5">
                                {item.status === "closed" && (
                                  <span className="rounded-full bg-slate-200 px-1.5 py-px text-[10px] font-medium text-slate-700">
                                    Closed
                                  </span>
                                )}
                                {item.tags.map((tag) => (
                                  <span
                                    key={tag.tag}
                                    className="rounded-full border px-1.5 py-px text-[10px] font-medium"
                                    style={entityStyle(tag.tag)}
                                  >
                                    {tag.tag}
                                  </span>
                                ))}
                              </div>
                            </div>
                            <span className="shrink-0 text-[11px] text-muted-foreground">
                              Recent issue
                            </span>
                          </button>
                        );
                      }

                      return (
                        <button
                          key={`recent-tag-${item.name}`}
                          type="button"
                          data-active={isActive}
                          onClick={() => navigate(item)}
                          onMouseEnter={() => setActiveIndex(index)}
                          className={cn(
                            "flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors",
                            isActive
                              ? "bg-accent text-accent-foreground"
                              : "text-foreground"
                          )}
                        >
                          <TagIcon className="size-4 shrink-0 text-muted-foreground" />
                          <div className="min-w-0 flex-1">
                            <p className="text-sm">
                              <span
                                className="rounded-full border px-2 py-0.5 text-[11px] font-medium"
                                style={entityStyle(item.name)}
                              >
                                {item.name}
                              </span>
                            </p>
                            {item.description && (
                              <p className="mt-1 truncate text-xs text-muted-foreground">
                                {item.description}
                              </p>
                            )}
                          </div>
                          <span className="shrink-0 text-[11px] text-muted-foreground">
                            Recent tag
                          </span>
                        </button>
                      );
                    })}

                    <div className="border-t border-border/50 px-4 py-2 text-xs text-muted-foreground/70">
                      Recently viewed issues and tags appear here.
                    </div>
                  </div>
                ) : (
                  <div className="flex flex-col items-center gap-1 py-12 text-center">
                    <p className="text-sm text-muted-foreground/80">
                      Search issues and tags...
                    </p>
                    <p className="text-xs text-muted-foreground/50">
                      Recently viewed issues and tags will show up here.
                    </p>
                  </div>
                )
              )}

              {activeQuery.length > 0 && !isLoading && items.length === 0 && (
                <div className="flex flex-col items-center gap-1 py-12 text-center">
                  <p className="text-sm text-muted-foreground/80">
                    No results for &ldquo;{activeQuery}&rdquo;
                  </p>
                  <p className="text-xs text-muted-foreground/50">
                    Try a different search term.
                  </p>
                </div>
              )}

              {activeQuery.length > 0 && items.length > 0 && (
                <div className="py-2">
                  {items.map((item, index) => {
                    const isActive = index === resolvedActiveIndex;

                    if (item.kind === "issue") {
                      return (
                        <button
                          key={`issue-${item.id}`}
                          type="button"
                          data-active={isActive}
                          onClick={() => navigate(item)}
                          onMouseEnter={() => setActiveIndex(index)}
                          className={cn(
                            "flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors",
                            isActive
                              ? "bg-accent text-accent-foreground"
                              : "text-foreground"
                          )}
                        >
                          <FileTextIcon className="size-4 shrink-0 text-muted-foreground" />
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm">
                              {item.raw}
                            </p>
                            <div className="mt-1 flex items-center gap-1.5">
                              {item.status === "closed" && (
                                <span className="rounded-full bg-slate-200 px-1.5 py-px text-[10px] font-medium text-slate-700">
                                  Closed
                                </span>
                              )}
                              {item.tags.slice(0, 3).map((tag) => (
                                <span
                                  key={tag.tag}
                                  className="rounded-full border px-1.5 py-px text-[10px] font-medium"
                                  style={entityStyle(tag.tag)}
                                >
                                  {tag.tag}
                                </span>
                              ))}
                            </div>
                          </div>
                          <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                            {Math.round((item.similarity ?? 0) * 100)}%
                          </span>
                        </button>
                      );
                    }

                    return (
                      <button
                        key={`tag-${item.name}`}
                        type="button"
                        data-active={isActive}
                        onClick={() => navigate(item)}
                        onMouseEnter={() => setActiveIndex(index)}
                        className={cn(
                          "flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors",
                          isActive
                            ? "bg-accent text-accent-foreground"
                            : "text-foreground"
                        )}
                      >
                        <TagIcon className="size-4 shrink-0 text-muted-foreground" />
                        <div className="min-w-0 flex-1">
                          <p className="text-sm">
                            <span
                              className="rounded-full border px-2 py-0.5 text-[11px] font-medium"
                              style={entityStyle(item.name)}
                            >
                              {item.name}
                            </span>
                          </p>
                          {item.description && (
                            <p className="mt-1 truncate text-xs text-muted-foreground">
                              {item.description}
                            </p>
                          )}
                        </div>
                        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                          {Math.round((item.similarity ?? 0) * 100)}%
                        </span>
                      </button>
                    );
                  })}

                  <div className="border-t border-border/50 px-4 py-2 text-xs text-muted-foreground/70">
                    Quick switcher showing up to {UNIFIED_SEARCH_LIMIT} matches.
                  </div>
                </div>
              )}
            </div>
          </Dialog.Popup>
        </Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
