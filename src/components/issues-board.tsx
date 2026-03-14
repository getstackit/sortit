"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import { motion, AnimatePresence } from "motion/react";
import { FilterIcon, Rows3Icon, Rows4Icon, SearchIcon, XIcon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { IssueCard } from "@/components/issue-card";
import { SiteHeader } from "@/components/site-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { CompactModeProvider, useCompactMode } from "@/hooks/use-compact-mode";
import {
  IssuesFilterSidebar,
  applyFilter,
  isFilterActive,
  EMPTY_FILTER,
  type IssuesFilter,
} from "@/components/issues-filter-sidebar";
import { useIssueSearch, useIssues } from "@/hooks/use-issues";
import { issueHrefFromText } from "@/lib/issues";
import { StatusBadge } from "@/components/status-badge";
import { TagBadge } from "@/components/tag-badge";
import type { IssueRecord, SearchIssueRecord } from "@/lib/issues";
import { cn } from "@/lib/utils";

const ISSUE_SEARCH_RESULT_LIMIT = 8;

function parseIssuesSearchState(searchParams: Pick<URLSearchParams, "get">) {
  const query =
    searchParams.get("q")?.trim() ?? searchParams.get("query")?.trim() ?? "";
  const includeClosed = query !== "" && searchParams.get("status") === "all";

  return {
    query,
    includeClosed,
  };
}

function CompactToggle() {
  const { isCompact, toggleCompact } = useCompactMode();
  const Icon = isCompact ? Rows4Icon : Rows3Icon;

  return (
    <button
      type="button"
      onClick={toggleCompact}
      title={isCompact ? "Normal view" : "Compact view"}
      className="flex size-8 items-center justify-center rounded-lg border border-border/60 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      <Icon className="size-4" />
    </button>
  );
}

function IssueList({ issues }: { issues: IssueRecord[] }) {
  const { isCompact } = useCompactMode();

  return (
    <div className={isCompact ? "space-y-1 px-4 lg:px-6" : "space-y-2 px-4 lg:px-6"}>
      <AnimatePresence mode="popLayout">
        {issues.map((issue, index) => (
          <motion.div
            key={issue.id}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            transition={{ duration: 0.2, ease: "easeOut", delay: index * 0.04 }}
          >
            <IssueCard
              issue={issue}
              href={`/issues/${issue.id}`}
              compact={isCompact}
            />
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}

function SearchResultCard({
  issue,
  compact,
}: {
  issue: SearchIssueRecord;
  compact: boolean;
}) {
  return (
    <Link
      href={`/issues/${issue.id}`}
      className={cn(
        "group/card app-surface block transition-all hover:-translate-y-0.5 hover:border-foreground/10 hover:bg-card/95",
        compact ? "px-3 py-2" : "p-5"
      )}
    >
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p
            className={cn(
              "whitespace-pre-wrap leading-relaxed",
              compact ? "line-clamp-2 text-xs" : "text-[15px]"
            )}
          >
            {issue.raw}
          </p>
          <p
            className={cn(
              "text-muted-foreground",
              compact ? "mt-1.5 line-clamp-1 text-[10px]" : "mt-2 text-xs"
            )}
          >
            {issue.reason}
          </p>
        </div>
        <div className="rounded-xl border border-border/60 bg-background/85 px-2 py-1 text-right shadow-sm">
          <div className="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
            match
          </div>
          <div className="text-sm font-semibold tabular-nums">
            {Math.round(issue.combinedSimilarity * 100)}%
          </div>
        </div>
      </div>

      <div className={cn("flex flex-wrap items-center gap-1.5", compact ? "mt-2" : "mt-3")}>
        {issue.status === "closed" && (
          <StatusBadge variant="closed" compact={compact} />
        )}
        {issue.tags.slice(0, compact ? 3 : 5).map((tag) => (
          <TagBadge key={tag.tag} tag={tag.tag} compact={compact} />
        ))}
        <span className="rounded-full border border-border/60 bg-muted/50 px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
          semantic {Math.round(issue.semanticSimilarity * 100)}%
        </span>
        <span className="rounded-full border border-border/60 bg-muted/50 px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
          factor {Math.round(issue.factorSimilarity * 100)}%
        </span>
      </div>
    </Link>
  );
}

function SearchResultList({ issues }: { issues: SearchIssueRecord[] }) {
  const { isCompact } = useCompactMode();

  return (
    <div className={isCompact ? "space-y-1 px-4 lg:px-6" : "space-y-2 px-4 lg:px-6"}>
      <AnimatePresence mode="popLayout">
        {issues.map((issue, index) => (
          <motion.div
            key={issue.id}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            transition={{ duration: 0.2, ease: "easeOut", delay: index * 0.04 }}
          >
            <SearchResultCard issue={issue} compact={isCompact} />
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}

export function IssuesBoard() {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const searchParamsString = searchParams.toString();
  const initialSearchState = useMemo(
    () => parseIssuesSearchState(searchParams),
    [searchParamsString]
  );
  const [searchText, setSearchText] = useState(initialSearchState.query);
  const [includeClosed, setIncludeClosed] = useState(initialSearchState.includeClosed);
  const [filter, setFilter] = useState<IssuesFilter>(EMPTY_FILTER);
  const [filterOpen, setFilterOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const lastWrittenSearchRef = useRef(searchParamsString);
  const deferredSearchText = useDeferredValue(searchText);
  const activeQuery = deferredSearchText.trim();
  const hasQuery = activeQuery.length > 0;
  const searchStatus = includeClosed ? "all" : "open";

  const {
    data: issues = [],
    error: issuesError,
    isLoading: issuesLoading,
    mutate: mutateIssues,
  } = useIssues("open", !hasQuery);
  const {
    data: searchResponse,
    error: searchError,
    isLoading: searchLoading,
    mutate: mutateSearch,
  } = useIssueSearch(activeQuery, searchStatus, ISSUE_SEARCH_RESULT_LIMIT);

  const searchResults = searchResponse?.relatedIssues ?? [];
  const unfilteredIssues = hasQuery ? searchResults : issues;
  const visibleIssues = isFilterActive(filter)
    ? applyFilter(unfilteredIssues, filter)
    : unfilteredIssues;
  const error = hasQuery ? searchError : issuesError;
  const loading = hasQuery ? searchLoading : issuesLoading;
  const resultCount = visibleIssues.length;

  useEffect(() => {
    if (lastWrittenSearchRef.current === searchParamsString) {
      return;
    }

    lastWrittenSearchRef.current = searchParamsString;

    setSearchText((current) =>
      current === initialSearchState.query ? current : initialSearchState.query
    );
    setIncludeClosed((current) =>
      current === initialSearchState.includeClosed
        ? current
        : initialSearchState.includeClosed
    );
  }, [initialSearchState.includeClosed, initialSearchState.query, searchParamsString]);

  useEffect(() => {
    if (!hasQuery && includeClosed) {
      setIncludeClosed(false);
    }
  }, [hasQuery, includeClosed]);

  useEffect(() => {
    const params = new URLSearchParams();

    if (activeQuery) {
      params.set("q", activeQuery);
      if (includeClosed) {
        params.set("status", "all");
      }
    }

    const nextSearch = params.toString();
    const nextURL = nextSearch ? `${pathname}?${nextSearch}` : pathname;
    const currentURL = `${window.location.pathname}${window.location.search}`;
    lastWrittenSearchRef.current = nextSearch;

    if (nextURL !== currentURL) {
      window.history.replaceState(window.history.state, "", nextURL);
    }
  }, [activeQuery, includeClosed, pathname]);

  const focusSearchInput = useCallback(() => {
    const input = searchInputRef.current;
    if (!input) {
      return;
    }

    input.focus();
    input.select();
  }, []);

  const navigateToDirectIssue = useCallback(
    (value: string) => {
      const href = issueHrefFromText(value);
      if (!href) {
        return false;
      }

      router.push(href);
      return true;
    },
    [router]
  );

  const shortcuts = useMemo(
    () => [
      {
        key: "s",
        description: "Focus semantic search",
        action: focusSearchInput,
      },
    ],
    [focusSearchInput]
  );

  const things = useMemo(
    () =>
      visibleIssues.map((issue) => ({
        id: issue.id,
        title: issue.raw.length > 60 ? `${issue.raw.slice(0, 60)}…` : issue.raw,
        href: `/issues/${issue.id}`,
      })),
    [visibleIssues]
  );

  return (
    <CompactModeProvider>
      <AppShell
        shortcuts={shortcuts}
        sidebar={
          <AppSidebar
            things={things}
            navigateOnCreate={false}
            onIssueCreated={(created) => {
              mutateIssues((prev) => (prev ? [created, ...prev] : [created]), {
                revalidate: false,
              });
              if (hasQuery) {
                void mutateSearch();
              }
            }}
          />
        }
      >
        <SiteHeader
          title={hasQuery ? "Search issues" : "Issues"}
          eyebrow="Issues"
          subtitle={
            hasQuery
              ? `Semantic matches for "${activeQuery}"${includeClosed ? " across open and closed issues." : " across open issues."}`
              : "Incoming reports, bugs, and ideas waiting to be triaged."
          }
          meta={
            resultCount > 0 ? (
              <span className="app-chip tabular-nums">
                {resultCount}
              </span>
            ) : null
          }
          actions={
            <div className="flex w-full flex-wrap items-center gap-2">
              <div className="relative min-w-[min(18rem,100%)] flex-1 sm:max-w-md">
                <SearchIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  ref={searchInputRef}
                  aria-label="Search issues"
                  value={searchText}
                  onChange={(event) => setSearchText(event.currentTarget.value)}
                  onPaste={(event) => {
                    const pastedText = event.clipboardData.getData("text");
                    if (navigateToDirectIssue(pastedText)) {
                      event.preventDefault();
                    }
                  }}
                  onKeyDown={(event) => {
                    if (event.key !== "Enter") {
                      return;
                    }

                    if (navigateToDirectIssue(searchText)) {
                      event.preventDefault();
                    }
                  }}
                  placeholder="Search issues semantically..."
                  className="h-9 rounded-xl bg-background/85 pl-9 pr-10"
                />
                {searchText && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Clear search"
                    className="absolute right-1 top-1/2 -translate-y-1/2"
                    onClick={() => {
                      setSearchText("");
                      setIncludeClosed(false);
                    }}
                  >
                    <XIcon className="size-4" />
                  </Button>
                )}
              </div>
              <Label
                className={cn(
                  "gap-2 rounded-xl border border-border/60 bg-background/80 px-3 py-2 text-xs text-muted-foreground",
                  !hasQuery && "opacity-60"
                )}
              >
                <Switch
                  size="sm"
                  checked={includeClosed}
                  disabled={!hasQuery}
                  onCheckedChange={setIncludeClosed}
                />
                Include closed
              </Label>
              <button
                type="button"
                onClick={() => setFilterOpen((o) => !o)}
                title={filterOpen ? "Hide filters" : "Show filters"}
                className={cn(
                  "flex size-8 items-center justify-center rounded-lg border border-border/60 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                  filterOpen && "bg-muted text-foreground"
                )}
              >
                <FilterIcon className="size-4" />
              </button>
              <CompactToggle />
            </div>
          }
        />
        <div className="flex min-h-0 flex-1">
          <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
            <div className="@container/main flex min-h-0 flex-1 flex-col gap-2">
              <div className="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
              {error && (
                <div className="px-4 lg:px-6">
                  <div className="app-status-warning">
                    Issue backend unavailable: {error?.message}
                  </div>
                </div>
              )}

              {hasQuery && !loading && !error && (
                <div className="px-4 lg:px-6">
                  <div className="app-surface flex flex-col gap-3 p-4">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="min-w-0">
                        <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                          Semantic search
                        </p>
                        <p className="truncate text-sm font-medium text-foreground">
                          {searchResponse?.query.raw ?? activeQuery}
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          Showing up to {ISSUE_SEARCH_RESULT_LIMIT} semantic matches.
                        </p>
                      </div>
                      <span className="rounded-full border border-border/60 bg-muted/50 px-3 py-1 text-xs font-medium text-muted-foreground">
                        {resultCount} result{resultCount === 1 ? "" : "s"}
                      </span>
                    </div>
                    {searchResponse?.query.tags && searchResponse.query.tags.length > 0 && (
                      <div className="flex flex-wrap gap-1.5">
                        {searchResponse.query.tags.slice(0, 5).map((tag) => (
                          <TagBadge key={tag.tag} tag={tag.tag} />
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}

              {loading && (
                <div className="space-y-3 px-4 lg:px-6">
                  {Array.from({ length: 4 }).map((_, index) => (
                    <div
                      key={index}
                      className="app-surface animate-fade-in-up p-5"
                      style={{ animationDelay: `${index * 75}ms` }}
                    >
                      <div className="space-y-3">
                        <Skeleton className="h-4 w-3/4" />
                        <Skeleton className="h-4 w-5/6" />
                        <Skeleton className="h-4 w-2/3" />
                      </div>
                      <div className="mt-4 flex items-center gap-2">
                        <Skeleton className="h-6 w-16 rounded-full" />
                        <Skeleton className="h-6 w-14 rounded-full" />
                        <Skeleton className="ml-auto h-4 w-24" />
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {!loading && !hasQuery && unfilteredIssues.length === 0 && (
                <div className="app-surface mx-4 flex flex-col items-center gap-3 py-20 text-center lg:mx-6">
                  <div className="text-4xl opacity-20">~</div>
                  <p className="text-sm text-muted-foreground/60">
                    Nothing open right now. Drop something in.
                  </p>
                </div>
              )}

              {!loading && visibleIssues.length === 0 && unfilteredIssues.length > 0 && isFilterActive(filter) && (
                <div className="app-surface mx-4 flex flex-col items-center gap-3 py-20 text-center lg:mx-6">
                  <div className="text-4xl opacity-20">~</div>
                  <p className="text-sm text-muted-foreground/80">
                    No issues match the current filters.
                  </p>
                  <button
                    type="button"
                    onClick={() => setFilter(EMPTY_FILTER)}
                    className="text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground"
                  >
                    Clear filters
                  </button>
                </div>
              )}

              {!loading && hasQuery && visibleIssues.length === 0 && !isFilterActive(filter) && (
                <div className="app-surface mx-4 flex flex-col items-center gap-3 py-20 text-center lg:mx-6">
                  <div className="text-4xl opacity-20">?</div>
                  <p className="text-sm text-muted-foreground/80">
                    No semantic matches for "{activeQuery}".
                  </p>
                  <p className="max-w-md text-xs text-muted-foreground/60">
                    Try a broader phrase, different wording, or include closed issues.
                  </p>
                </div>
              )}

              {!hasQuery && visibleIssues.length > 0 && <IssueList issues={visibleIssues} />}
              {hasQuery && visibleIssues.length > 0 && (
                <SearchResultList issues={visibleIssues as SearchIssueRecord[]} />
              )}
              </div>
            </div>
          </div>
          {filterOpen && (
            <aside className="hidden w-56 shrink-0 overflow-y-auto md:block">
              <IssuesFilterSidebar
                issues={unfilteredIssues}
                filter={filter}
                onFilterChange={setFilter}
              />
            </aside>
          )}
        </div>
      </AppShell>
    </CompactModeProvider>
  );
}
