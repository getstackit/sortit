"use client";

import Link from "next/link";
import { SparklesIcon, UserIcon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { IssueListItem } from "@/components/issue-list-item";
import { SiteHeader } from "@/components/site-header";
import { TagRelevanceBars } from "@/components/tag-relevance-bars";
import { Skeleton } from "@/components/ui/skeleton";
import { usePersonDetail } from "@/hooks/use-people";
import { entityStyle } from "@/lib/entity-colors";
import { cn } from "@/lib/utils";
import type { PersonIssueRecommendation } from "@/lib/people";

function scoreClasses(score: number) {
  if (score >= 0.65) return "bg-emerald-100 text-emerald-700";
  if (score >= 0.35) return "bg-amber-100 text-amber-700";
  return "bg-slate-100 text-slate-700";
}

function RecommendationCard({
  title,
  recommendation,
}: {
  title: string;
  recommendation: PersonIssueRecommendation;
}) {
  return (
    <section className="app-surface rounded-[1.5rem] p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
            {title}
          </p>
          <p className="mt-2 text-sm text-foreground/85">{recommendation.reason}</p>
        </div>
        <span
          className={cn(
            "rounded-full px-2.5 py-1 text-xs font-semibold tabular-nums",
            scoreClasses(recommendation.combinedScore)
          )}
        >
          {(recommendation.combinedScore * 100).toFixed(0)}% match
        </span>
      </div>

      <div className="mt-4">
        <IssueListItem
          issue={recommendation.issue}
          tags={recommendation.issue.tagScores ?? recommendation.issue.tags}
          href={`/issues/${recommendation.issue.id}`}
          maxLabelLength={96}
          className="rounded-xl border border-border/70 bg-background/65 px-3 py-3"
        />
      </div>

      <div className="mt-4 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
        <span className="app-chip">Factor {(recommendation.factorScore * 100).toFixed(0)}%</span>
        <span className="app-chip">Semantic {(recommendation.semanticScore * 100).toFixed(0)}%</span>
        <span className="app-chip capitalize">{recommendation.source}</span>
      </div>

      {recommendation.sharedTags.length > 0 && (
        <div className="mt-4 flex flex-wrap gap-1.5">
          {recommendation.sharedTags.map((tag) => (
            <span
              key={tag}
              className="rounded-full border px-2 py-0.5 text-[11px] font-medium"
              style={entityStyle(tag)}
            >
              {tag}
            </span>
          ))}
        </div>
      )}
    </section>
  );
}

export function PersonDetailPage({ person }: { person: string }) {
  const { data, error, isLoading } = usePersonDetail(person);

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title={person}
        eyebrow="Person"
        subtitle="Assigned queue, factor profile, and next-issue recommendations"
        meta={
          data ? (
            <>
              <span className="app-chip tabular-nums">{data.issueCount} assigned</span>
              <span className="app-chip tabular-nums">{data.openIssueCount} open</span>
              <span className="app-chip tabular-nums">{data.closedIssueCount} closed</span>
            </>
          ) : undefined
        }
        actions={
          <Link
            href="/people"
            className="rounded-md border border-border px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            Back to people
          </Link>
        }
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          {isLoading && (
            <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_21rem]">
              <div className="space-y-6">
                <Skeleton className="h-56 rounded-[1.5rem]" />
                <Skeleton className="h-72 rounded-[1.5rem]" />
              </div>
              <div className="space-y-4">
                <Skeleton className="h-48 rounded-[1.5rem]" />
                <Skeleton className="h-64 rounded-[1.5rem]" />
              </div>
            </div>
          )}

          {!isLoading && error && (
            <div className="app-status-warning">
              Failed to load person detail: {error.message}
            </div>
          )}

          {!isLoading && !error && data && (
            <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_21rem] 2xl:grid-cols-[minmax(0,1.1fr)_24rem]">
              <div className="space-y-6">
                {data.nextIssue && (
                  <RecommendationCard
                    title={
                      data.nextIssue.source === "assigned"
                        ? "Current Queue Head"
                        : "Suggested Next Issue"
                    }
                    recommendation={data.nextIssue}
                  />
                )}

                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                        Assigned Queue
                      </p>
                      <h2 className="mt-2 text-lg font-semibold tracking-tight">
                        Issues currently attributed to {data.person}
                      </h2>
                    </div>
                    <span className="app-chip tabular-nums">{data.assignedIssues.length}</span>
                  </div>

                  {data.assignedIssues.length > 0 ? (
                    <div className="mt-4 space-y-2">
                      {data.assignedIssues.map((issue) => (
                        <IssueListItem
                          key={issue.id}
                          issue={issue}
                          tags={issue.tagScores ?? issue.tags}
                          href={`/issues/${issue.id}`}
                          maxLabelLength={96}
                        />
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 text-sm text-muted-foreground">
                      No issues are currently assigned to this person.
                    </p>
                  )}
                </section>

                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-2">
                    <SparklesIcon className="size-4 text-amber-500" />
                    <div>
                      <p className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
                        Open Recommendations
                      </p>
                      <h2 className="mt-1 text-lg font-semibold tracking-tight">
                        Profile-matched open issues
                      </h2>
                    </div>
                  </div>

                  {data.recommendedIssues.length > 0 ? (
                    <div className="mt-4 space-y-4">
                      {data.recommendedIssues.map((recommendation) => (
                        <RecommendationCard
                          key={recommendation.issue.id}
                          title="Recommendation"
                          recommendation={recommendation}
                        />
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 text-sm text-muted-foreground">
                      No strong open issue matches were found for this person yet.
                    </p>
                  )}
                </section>
              </div>

              <aside className="space-y-4">
                <section className="app-surface rounded-[1.5rem] p-5">
                  <div className="flex items-center gap-3">
                    <span className="flex size-10 items-center justify-center rounded-full bg-violet-100 text-violet-700">
                      <UserIcon className="size-4" />
                    </span>
                    <div>
                      <p className="text-sm font-semibold">{data.person}</p>
                      <p className="text-[11px] text-muted-foreground">
                        Historical factor profile from assigned issues
                      </p>
                    </div>
                  </div>
                  <div className="mt-4">
                    <TagRelevanceBars tags={data.tagProfile} />
                  </div>
                </section>
              </aside>
            </div>
          )}
        </div>
      </div>
    </AppShell>
  );
}
