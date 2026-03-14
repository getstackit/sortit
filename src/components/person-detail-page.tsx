"use client";

import Link from "next/link";
import { SparklesIcon, UserIcon } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { DetailPageLayout, DetailPageGrid } from "@/components/detail-page-layout";
import { IssueListItem } from "@/components/issue-list-item";
import { SectionHeader } from "@/components/section-header";
import { SiteHeader } from "@/components/site-header";
import { TagBadge } from "@/components/tag-badge";
import { TagRelevanceBars } from "@/components/tag-relevance-bars";
import { usePersonDetail } from "@/hooks/use-people";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { scoreClasses } from "@/lib/format";
import type { PersonIssueRecommendation } from "@/lib/people";

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
        <Badge variant="outline">Factor {(recommendation.factorScore * 100).toFixed(0)}%</Badge>
        <Badge variant="outline">Semantic {(recommendation.semanticScore * 100).toFixed(0)}%</Badge>
        <Badge variant="outline" className="capitalize">{recommendation.source}</Badge>
      </div>

      {recommendation.sharedTags.length > 0 && (
        <div className="mt-4 flex flex-wrap gap-1.5">
          {recommendation.sharedTags.map((tag) => (
            <TagBadge key={tag} tag={tag} />
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
              <Badge variant="outline" className="tabular-nums">{data.issueCount} assigned</Badge>
              <Badge variant="outline" className="tabular-nums">{data.openIssueCount} open</Badge>
              <Badge variant="outline" className="tabular-nums">{data.closedIssueCount} closed</Badge>
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

      <DetailPageLayout
        loading={isLoading}
        error={error ? <>Failed to load person detail: {error.message}</> : undefined}
      >
        {data && (
          <DetailPageGrid
            sidebar={
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
            }
          >
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
              <SectionHeader
                eyebrow="Assigned Queue"
                title={`Issues currently attributed to ${data.person}`}
                count={data.assignedIssues.length}
              />

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
                <SectionHeader
                  eyebrow="Open Recommendations"
                  title="Profile-matched open issues"
                />
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
          </DetailPageGrid>
        )}
      </DetailPageLayout>
    </AppShell>
  );
}
