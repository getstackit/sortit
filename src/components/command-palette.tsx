"use client";

import { useCallback, useDeferredValue, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  FileTextIcon,
  KeyboardIcon,
  LoaderIcon,
  PlusIcon,
  SunMoonIcon,
  TagIcon,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import { StatusBadge } from "@/components/status-badge";
import { TagBadge } from "@/components/tag-badge";
import { UNIFIED_SEARCH_LIMIT, useUnifiedSearch } from "@/hooks/use-search";
import { useRecentHistory } from "@/hooks/use-recent-history";
import { issueHrefFromText } from "@/lib/issues";
import { NAV_ITEMS } from "@/lib/nav";
import { tagHref } from "@/lib/tags";
import { toggleTheme } from "@/lib/theme";
import type { IssueStatus, SearchIssueRecord } from "@/lib/issues";
import type { RelatedTag } from "@/lib/search";

type CommandPaletteProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onNewIssue?: () => void;
  onShowShortcuts?: () => void;
};

type QuickAction = {
  id: string;
  label: string;
  icon: LucideIcon;
  keywords?: string[];
  run: () => void;
};

function matchesQuery(query: string, ...haystacks: string[]) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  return haystacks.some((haystack) => haystack.toLowerCase().includes(normalized));
}

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

export function CommandPalette({
  open,
  onOpenChange,
  onNewIssue,
  onShowShortcuts,
}: CommandPaletteProps) {
  const router = useRouter();
  const [searchText, setSearchText] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const recentItems = useRecentHistory();

  const deferredSearchText = useDeferredValue(searchText);
  const activeQuery = deferredSearchText.trim();

  const { data, isLoading } = useUnifiedSearch(activeQuery);

  const searchItems = useMemo<ResultItem[]>(() => {
    const next: ResultItem[] = [];
    if (!data) return next;

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

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        setSearchText("");
      }
      onOpenChange(nextOpen);
    },
    [onOpenChange]
  );

  const navigateToDirectIssue = useCallback(
    (value: string) => {
      const href = issueHrefFromText(value);
      if (!href) return false;

      router.push(href);
      handleOpenChange(false);
      return true;
    },
    [handleOpenChange, router]
  );

  const navigateToItem = useCallback(
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

  const navigateToHref = useCallback(
    (href: string) => {
      router.push(href);
      handleOpenChange(false);
    },
    [handleOpenChange, router]
  );

  const handlePaste = useCallback(
    (event: React.ClipboardEvent) => {
      const pastedText = event.clipboardData.getData("text");
      if (navigateToDirectIssue(pastedText)) {
        event.preventDefault();
      }
    },
    [navigateToDirectIssue]
  );

  const quickActions = useMemo<QuickAction[]>(() => {
    const list: QuickAction[] = [];
    if (onNewIssue) {
      list.push({
        id: "new-issue",
        label: "New issue",
        icon: PlusIcon,
        keywords: ["create", "add", "compose"],
        run: () => {
          onNewIssue();
          handleOpenChange(false);
        },
      });
    }
    list.push({
      id: "toggle-theme",
      label: "Toggle theme",
      icon: SunMoonIcon,
      keywords: ["dark", "light", "appearance", "mode"],
      run: () => {
        toggleTheme();
        handleOpenChange(false);
      },
    });
    if (onShowShortcuts) {
      list.push({
        id: "keyboard-shortcuts",
        label: "Keyboard shortcuts",
        icon: KeyboardIcon,
        keywords: ["help", "keys", "hotkeys"],
        run: () => {
          onShowShortcuts();
          handleOpenChange(false);
        },
      });
    }
    return list;
  }, [handleOpenChange, onNewIssue, onShowShortcuts]);

  const pageMatches = useMemo(
    () => NAV_ITEMS.filter((item) => matchesQuery(activeQuery, item.label, item.href)),
    [activeQuery]
  );

  const actionMatches = useMemo(
    () =>
      quickActions.filter((action) =>
        matchesQuery(activeQuery, action.label, ...(action.keywords ?? []))
      ),
    [activeQuery, quickActions]
  );

  const hasQuery = activeQuery.length > 0;
  const showPages = pageMatches.length > 0;
  const showActions = actionMatches.length > 0;
  const showRecent = !hasQuery && items.length > 0;
  const showResults = hasQuery && items.length > 0;
  const showTopSection = showPages || showActions;
  const showBottomSection = showRecent || showResults;
  const showEmpty =
    hasQuery && !isLoading && !showTopSection && items.length === 0;

  return (
    <CommandDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Command Palette"
      description="Search issues and tags"
    >
      <Command shouldFilter={false}>
        <div className="relative">
          <CommandInput
            ref={inputRef}
            value={searchText}
            onValueChange={setSearchText}
            onPaste={handlePaste}
            placeholder="Search issues and tags..."
          />
          {isLoading && (
            <div className="absolute right-3 top-1/2 -translate-y-1/2">
              <LoaderIcon className="size-4 animate-spin text-muted-foreground" />
            </div>
          )}
        </div>

        <CommandList>
          {showPages && (
            <CommandGroup heading="Pages">
              {pageMatches.map((item) => (
                <CommandItem
                  key={`page-${item.href}`}
                  value={`page-${item.href}`}
                  onSelect={() => navigateToHref(item.href)}
                >
                  <item.icon className="size-4 shrink-0 text-muted-foreground" />
                  <span className="flex-1 truncate text-sm">{item.label}</span>
                  {item.shortcut && (
                    <CommandShortcut>G {item.shortcut.toUpperCase()}</CommandShortcut>
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {showActions && (
            <CommandGroup heading="Quick actions">
              {actionMatches.map((action) => (
                <CommandItem
                  key={`action-${action.id}`}
                  value={`action-${action.id}`}
                  onSelect={action.run}
                >
                  <action.icon className="size-4 shrink-0 text-muted-foreground" />
                  <span className="flex-1 truncate text-sm">{action.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {showTopSection && showBottomSection && <CommandSeparator />}

          {showEmpty && (
            <CommandEmpty>
              <div className="flex flex-col items-center gap-1 py-6">
                <p className="text-sm text-muted-foreground/80">
                  No results for &ldquo;{activeQuery}&rdquo;
                </p>
                <p className="text-xs text-muted-foreground/50">
                  Try a different search term.
                </p>
              </div>
            </CommandEmpty>
          )}

          {showRecent && (
            <>
              <CommandGroup heading="Recently viewed">
                {items.map((item) => {
                  if (item.kind === "issue") {
                    return (
                      <CommandItem
                        key={`recent-issue-${item.id}`}
                        value={`issue-${item.id}`}
                        onSelect={() => navigateToItem(item)}
                      >
                        <FileTextIcon className="size-4 shrink-0 text-muted-foreground" />
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm">{item.raw}</p>
                          <div className="mt-1 flex items-center gap-1.5">
                            {item.status === "closed" && (
                              <StatusBadge variant="closed" compact />
                            )}
                            {item.tags.map((t) => (
                              <TagBadge key={t.tag} tag={t.tag} compact />
                            ))}
                          </div>
                        </div>
                        <span className="shrink-0 text-[11px] text-muted-foreground">
                          Recent issue
                        </span>
                      </CommandItem>
                    );
                  }

                  return (
                    <CommandItem
                      key={`recent-tag-${item.name}`}
                      value={`tag-${item.name}`}
                      onSelect={() => navigateToItem(item)}
                    >
                      <TagIcon className="size-4 shrink-0 text-muted-foreground" />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm">
                          <TagBadge tag={item.name} />
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
                    </CommandItem>
                  );
                })}
              </CommandGroup>
              <CommandSeparator />
              <p className="px-4 py-2 text-xs text-muted-foreground/70">
                Recently viewed issues and tags appear here.
              </p>
            </>
          )}

          {showResults && (
            <>
              <CommandGroup heading="Search results">
                {items.map((item) => {
                  if (item.kind === "issue") {
                    return (
                      <CommandItem
                        key={`issue-${item.id}`}
                        value={`issue-${item.id}`}
                        onSelect={() => navigateToItem(item)}
                      >
                        <FileTextIcon className="size-4 shrink-0 text-muted-foreground" />
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm">{item.raw}</p>
                          <div className="mt-1 flex items-center gap-1.5">
                            {item.status === "closed" && (
                              <StatusBadge variant="closed" compact />
                            )}
                            {item.tags.slice(0, 3).map((t) => (
                              <TagBadge key={t.tag} tag={t.tag} compact />
                            ))}
                          </div>
                        </div>
                        <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                          {Math.round((item.similarity ?? 0) * 100)}%
                        </span>
                      </CommandItem>
                    );
                  }

                  return (
                    <CommandItem
                      key={`tag-${item.name}`}
                      value={`tag-${item.name}`}
                      onSelect={() => navigateToItem(item)}
                    >
                      <TagIcon className="size-4 shrink-0 text-muted-foreground" />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm">
                          <TagBadge tag={item.name} />
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
                    </CommandItem>
                  );
                })}
              </CommandGroup>
              <CommandSeparator />
              <p className="px-4 py-2 text-xs text-muted-foreground/70">
                Quick switcher showing up to {UNIFIED_SEARCH_LIMIT} matches.
              </p>
            </>
          )}
        </CommandList>
      </Command>
    </CommandDialog>
  );
}
