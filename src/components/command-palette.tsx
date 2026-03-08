"use client";

import { useCallback, useDeferredValue, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Dialog } from "@base-ui/react/dialog";
import { FileTextIcon, LoaderIcon, SearchIcon, TagIcon } from "lucide-react";
import { useUnifiedSearch } from "@/hooks/use-search";
import { entityStyle } from "@/lib/entity-colors";
import type { SearchIssueRecord } from "@/lib/issues";
import type { RelatedTag } from "@/lib/search";
import { cn } from "@/lib/utils";

type CommandPaletteProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type ResultItem =
  | { kind: "issue"; issue: SearchIssueRecord }
  | { kind: "tag"; tag: RelatedTag };

export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const router = useRouter();
  const [searchText, setSearchText] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const [activeIndex, setActiveIndex] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  const deferredSearchText = useDeferredValue(searchText);
  const activeQuery = deferredSearchText.trim();

  const { data, isLoading } = useUnifiedSearch(activeQuery);

  const items: ResultItem[] = [];
  if (data) {
    for (const issue of data.issues) {
      items.push({ kind: "issue", issue });
    }
    for (const tag of data.relatedTags) {
      items.push({ kind: "tag", tag });
    }
  }

  useEffect(() => {
    setActiveIndex(0);
  }, [activeQuery]);

  useEffect(() => {
    if (!open) {
      setSearchText("");
      setActiveIndex(0);
    }
  }, [open]);

  const navigate = useCallback(
    (item: ResultItem) => {
      if (item.kind === "issue") {
        router.push(`/issues/${item.issue.id}`);
      } else {
        router.push("/tags");
      }
      onOpenChange(false);
    },
    [router, onOpenChange]
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
        const item = items[activeIndex];
        if (item) {
          navigate(item);
        }
      }
    },
    [items, activeIndex, navigate]
  );

  useEffect(() => {
    const activeEl = listRef.current?.querySelector("[data-active='true']");
    if (activeEl) {
      activeEl.scrollIntoView({ block: "nearest" });
    }
  }, [activeIndex]);

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-50 bg-black/40 transition-opacity data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
        <Dialog.Viewport className="fixed inset-0 z-50 flex items-start justify-center pt-[12vh]">
          <Dialog.Popup className="app-surface w-full max-w-xl rounded-[1.5rem] p-0 transition-all data-[ending-style]:scale-95 data-[ending-style]:opacity-0 data-[starting-style]:scale-95 data-[starting-style]:opacity-0">
            <div className="flex items-center gap-3 border-b border-border/60 px-4 py-3">
              <SearchIcon className="size-4 shrink-0 text-muted-foreground" />
              <input
                ref={inputRef}
                value={searchText}
                onChange={(e) => setSearchText(e.target.value)}
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
                <div className="flex items-center justify-center py-12 text-sm text-muted-foreground/50">
                  Search issues and tags...
                </div>
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

              {items.length > 0 && (
                <div className="py-2">
                  {items.map((item, index) => {
                    const isActive = index === activeIndex;

                    if (item.kind === "issue") {
                      return (
                        <button
                          key={`issue-${item.issue.id}`}
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
                              {item.issue.raw}
                            </p>
                            <div className="mt-1 flex items-center gap-1.5">
                              {item.issue.status === "closed" && (
                                <span className="rounded-full bg-slate-200 px-1.5 py-px text-[10px] font-medium text-slate-700">
                                  Closed
                                </span>
                              )}
                              {item.issue.tags.slice(0, 3).map((tag) => (
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
                            {Math.round(item.issue.combinedSimilarity * 100)}%
                          </span>
                        </button>
                      );
                    }

                    return (
                      <button
                        key={`tag-${item.tag.name}`}
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
                              style={entityStyle(item.tag.name)}
                            >
                              {item.tag.name}
                            </span>
                          </p>
                          {item.tag.description && (
                            <p className="mt-1 truncate text-xs text-muted-foreground">
                              {item.tag.description}
                            </p>
                          )}
                        </div>
                        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                          {Math.round(item.tag.similarity * 100)}%
                        </span>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          </Dialog.Popup>
        </Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
