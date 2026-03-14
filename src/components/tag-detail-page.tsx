"use client";

import Link from "next/link";
import { useEffect, useMemo } from "react";
import { HashIcon, Link2Icon, SparklesIcon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { DetailPageLayout, DetailPageGrid } from "@/components/detail-page-layout";
import { IssueCard } from "@/components/issue-card";
import { SectionHeader } from "@/components/section-header";
import { SiteHeader } from "@/components/site-header";
import { TagBadge } from "@/components/tag-badge";
import { TagRelevanceBars } from "@/components/tag-relevance-bars";
import { useIssues } from "@/hooks/use-issues";
import { useTags } from "@/hooks/use-tags";
import { rememberRecentTag } from "@/hooks/use-recent-history";
import { entityStyle } from "@/lib/entity-colors";
import type { IssueRecord } from "@/lib/issues";
import { tagHref } from "@/lib/tags";

type RelatedTag = {
  tag: string;
  relevance: number;
  count: number;
};

type SemanticNeighbor = {
  name: string;
  description: string;
  similarity: number;
};

function normalizeTagName(value: string) {
  return value.trim().toLowerCase();
}

function cosineSimilarity(left: number[], right: number[]) {
  if (left.length === 0 || left.length !== right.length) {
    return 0;
  }

  let dotProduct = 0;
  let leftMagnitude = 0;
  let rightMagnitude = 0;
  for (let index = 0; index < left.length; index += 1) {
    dotProduct += left[index] * right[index];
    leftMagnitude += left[index] * left[index];
    rightMagnitude += right[index] * right[index];
  }

  if (leftMagnitude === 0 || rightMagnitude === 0) {
    return 0;
  }

  return dotProduct / (Math.sqrt(leftMagnitude) * Math.sqrt(rightMagnitude));
}

function issueTagRelevance(issue: IssueRecord, tagName: string) {
  const normalizedTag = normalizeTagName(tagName);
  const score = issue.tagScores?.find(
    (entry) => normalizeTagName(entry.tag) === normalizedTag
  );
  if (score) {
    return score.relevance;
  }

  return issue.tags.some((tag) => normalizeTagName(tag) === normalizedTag) ? 1 : 0;
}

function compareIssuesByTag(left: IssueRecord, right: IssueRecord, tagName: string) {
  if (left.status !== right.status) {
    return left.status === "open" ? -1 : 1;
  }

  const relevanceDiff = issueTagRelevance(right, tagName) - issueTagRelevance(left, tagName);
  if (relevanceDiff !== 0) {
    return relevanceDiff;
  }

  return new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime();
}

export function TagDetailPage({ tagName }: { tagName: string }) {
  const normalizedTag = normalizeTagName(tagName);
  const { data: tags = [], error: tagsError, isLoading: tagsLoading } = useTags();
  const {
    data: issues = [],
    error: issuesError,
    isLoading: issuesLoading,
  } = useIssues("all");

  const tag = useMemo(
    () => tags.find((entry) => normalizeTagName(entry.name) === normalizedTag) ?? null,
    [normalizedTag, tags]
  );

  const relatedIssues = useMemo(() => {
    return issues
      .filter((issue) => issueTagRelevance(issue, normalizedTag) > 0)
      .sort((left, right) => compareIssuesByTag(left, right, normalizedTag));
  }, [issues, normalizedTag]);

  const relatedTags = useMemo<RelatedTag[]>(() => {
    const aggregate = new Map<string, { total: number; count: number }>();

    for (const issue of relatedIssues) {
      const sourceTags =
        issue.tagScores && issue.tagScores.length > 0
          ? issue.tagScores.map((entry) => ({
              tag: entry.tag,
              relevance: entry.relevance,
            }))
          : issue.tags.map((entry) => ({ tag: entry, relevance: 1 }));

      for (const { tag: entryTag, relevance } of sourceTags) {
        const normalizedEntry = normalizeTagName(entryTag);
        if (normalizedEntry === normalizedTag) {
          continue;
        }

        const current = aggregate.get(entryTag) ?? { total: 0, count: 0 };
        current.total += relevance;
        current.count += 1;
        aggregate.set(entryTag, current);
      }
    }

    return Array.from(aggregate.entries())
      .map(([entryTag, stats]) => ({
        tag: entryTag,
        relevance: stats.total / stats.count,
        count: stats.count,
      }))
      .sort((left, right) => {
        if (right.relevance !== left.relevance) {
          return right.relevance - left.relevance;
        }
        if (right.count !== left.count) {
          return right.count - left.count;
        }
        return left.tag.localeCompare(right.tag);
      })
      .slice(0, 8);
  }, [normalizedTag, relatedIssues]);

  const semanticNeighbors = useMemo<SemanticNeighbor[]>(() => {
    if (!tag || tag.embedding.length < 2) {
      return [];
    }

    return tags
      .filter(
        (entry) =>
          normalizeTagName(entry.name) !== normalizedTag && entry.embedding.length === tag.embedding.length
      )
      .map((entry) => ({
        name: entry.name,
        description: entry.description ?? "",
        similarity: cosineSimilarity(tag.embedding, entry.embedding),
      }))
      .sort((left, right) => right.similarity - left.similarity)
      .slice(0, 6);
  }, [normalizedTag, tag, tags]);

  const issueCount = relatedIssues.length;
  const openCount = relatedIssues.filter((issue) => issue.status === "open").length;
  const closedCount = issueCount - openCount;
  const loading = tagsLoading || issuesLoading;
  const error = tagsError ?? issuesError;

  useEffect(() => {
    if (!tag) {
      return;
    }

    rememberRecentTag(tag);
  }, [tag]);

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title={tag?.name ?? tagName}
        eyebrow="Tag"
        subtitle={
          tag?.description ||
          "Issues, related tags, and semantic neighbors for this tag."
        }
        meta={
          !loading && tag ? (
            <>
              <span className="app-chip tabular-nums">{issueCount} issues</span>
              <span className="app-chip tabular-nums">{openCount} open</span>
              {closedCount > 0 && (
                <span className="app-chip tabular-nums">{closedCount} closed</span>
              )}
            </>
          ) : undefined
        }
        actions={
          <Link
            href="/tags"
            className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            Back to tag map
          </Link>
        }
      />

      <DetailPageLayout
        loading={loading}
        error={
          error
            ? <>Failed to load tag context: {error.message}</>
            : !loading && !error && !tag
              ? <>No tag exists for &ldquo;{tagName}&rdquo;.</>
              : undefined
        }
      >
        {tag && (
          <DetailPageGrid
            sidebar={
              <>
                <section id="related" className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-2">
                    <HashIcon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Related tags</h3>
                  </div>

                  {relatedTags.length > 0 ? (
                    <div className="mt-4 space-y-4">
                      <TagRelevanceBars
                        tags={relatedTags.map((entry) => ({
                          tag: entry.tag,
                          relevance: entry.relevance,
                        }))}
                      />
                      <div className="space-y-1.5">
                        {relatedTags.map((entry) => (
                          <Link
                            key={entry.tag}
                            href={tagHref(entry.tag)}
                            className="app-subtle-surface flex items-center justify-between gap-3 px-3 py-2 transition-colors hover:bg-accent/70"
                          >
                            <TagBadge tag={entry.tag} />
                            <span className="text-[11px] tabular-nums text-muted-foreground">
                              {entry.count} issues
                            </span>
                          </Link>
                        ))}
                      </div>
                    </div>
                  ) : (
                    <p className="mt-4 text-sm text-muted-foreground">
                      No strong co-occurring tags yet.
                    </p>
                  )}
                </section>

                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-2">
                    <Link2Icon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Semantic neighbors</h3>
                  </div>

                  {semanticNeighbors.length > 0 ? (
                    <div className="mt-4 space-y-1.5">
                      {semanticNeighbors.map((neighbor) => (
                        <Link
                          key={neighbor.name}
                          href={tagHref(neighbor.name)}
                          className="app-subtle-surface block px-3 py-2 transition-colors hover:bg-accent/70"
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <p className="text-sm font-medium">{neighbor.name}</p>
                              <p className="mt-1 text-xs text-muted-foreground">
                                {neighbor.description || "No description"}
                              </p>
                            </div>
                            <span className="text-xs font-medium tabular-nums text-muted-foreground">
                              {Math.round(neighbor.similarity * 100)}%
                            </span>
                          </div>
                        </Link>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 text-sm text-muted-foreground">
                      No tag embeddings are available for semantic comparison yet.
                    </p>
                  )}
                </section>

                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-2">
                    <SparklesIcon className="size-4 text-muted-foreground" />
                    <h3 className="text-sm font-semibold">Context</h3>
                  </div>
                  <div className="mt-4 space-y-2 text-sm text-muted-foreground">
                    <p>
                      Related tags are aggregated from issues that carry{" "}
                      <span className="font-medium text-foreground">{tag.name}</span>.
                    </p>
                    <p>
                      Semantic neighbors come from stored tag embeddings and are independent of issue co-occurrence.
                    </p>
                  </div>
                </section>
              </>
            }
          >
            <section className="app-surface rounded-[1.75rem] p-6">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="space-y-3">
                  <TagBadge tag={tag.name} className="inline-flex px-3 py-1 text-xs" />
                  <div>
                    <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-muted-foreground">
                      Canonical description
                    </p>
                    <p className="mt-2 max-w-3xl text-[15px] leading-7 text-foreground/90">
                      {tag.description || "No description has been recorded for this tag yet."}
                    </p>
                  </div>
                </div>
                <div className="grid min-w-[12rem] gap-3 sm:grid-cols-3 xl:grid-cols-1">
                  <div className="app-subtle-surface rounded-[1.25rem] px-4 py-3">
                    <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                      Coverage
                    </p>
                    <p className="mt-2 text-2xl font-semibold tabular-nums">
                      {issueCount}
                    </p>
                    <p className="text-[11px] text-muted-foreground">
                      issues tagged
                    </p>
                  </div>
                  <div className="app-subtle-surface rounded-[1.25rem] px-4 py-3">
                    <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                      Open
                    </p>
                    <p className="mt-2 text-2xl font-semibold tabular-nums">
                      {openCount}
                    </p>
                    <p className="text-[11px] text-muted-foreground">
                      currently active
                    </p>
                  </div>
                  <div className="app-subtle-surface rounded-[1.25rem] px-4 py-3">
                    <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
                      Neighbors
                    </p>
                    <p className="mt-2 text-2xl font-semibold tabular-nums">
                      {semanticNeighbors.length}
                    </p>
                    <p className="text-[11px] text-muted-foreground">
                      semantic matches
                    </p>
                  </div>
                </div>
              </div>
            </section>

            <section id="issues" className="app-surface rounded-[1.75rem] p-6">
              <SectionHeader
                eyebrow="Associated issues"
                title="All issues carrying this tag"
                count={`${issueCount} results`}
              />

              {relatedIssues.length === 0 ? (
                <p className="mt-5 text-sm text-muted-foreground">
                  No issues are currently associated with this tag.
                </p>
              ) : (
                <div className="mt-5 space-y-2">
                  {relatedIssues.map((issue) => (
                    <IssueCard
                      key={issue.id}
                      issue={issue}
                      href={`/issues/${issue.id}`}
                    />
                  ))}
                </div>
              )}
            </section>
          </DetailPageGrid>
        )}
      </DetailPageLayout>
    </AppShell>
  );
}
