"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { AvatarCircle } from "@/components/avatar-circle";
import { FilterToggleGroup } from "@/components/filter-toggle";
import { IssueListItem } from "@/components/issue-list-item";
import { SiteHeader } from "@/components/site-header";
import { TagBadge } from "@/components/tag-badge";
import { TagRelevanceBars } from "@/components/tag-relevance-bars";
import { useWorkCorrelations } from "@/hooks/use-people";
import { useIssues } from "@/hooks/use-issues";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { scoreColor } from "@/lib/format";
import type { IssueRecord } from "@/lib/issues";
import type { PeopleListStatus, PersonCorrelation, TagRelevance } from "@/lib/people";

function PersonProfileCard({
  person,
  issueCount,
  tagProfile,
  issues,
}: {
  person: string;
  issueCount: number;
  tagProfile: TagRelevance[];
  issues: IssueRecord[];
}) {
  return (
    <div className="app-surface rounded-[1.5rem] p-5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <AvatarCircle name={person} />
          <div>
            <Link
              href={`/people/${encodeURIComponent(person)}`}
              className="text-sm font-semibold transition-colors hover:text-violet-700"
            >
              {person}
            </Link>
            <p className="text-[11px] text-muted-foreground">
              {issueCount} issue{issueCount === 1 ? "" : "s"} assigned
            </p>
          </div>
        </div>
      </div>
      {tagProfile.length > 0 && (
        <div className="mt-4">
          <TagRelevanceBars tags={tagProfile} />
        </div>
      )}
      <div className="mt-5 space-y-2">
        <div className="flex items-center justify-between gap-2">
          <p className="text-[11px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
            Assigned issues
          </p>
          <Badge variant="outline">{issues.length}</Badge>
        </div>
        <div className="space-y-2">
          {issues.map((issue) => (
            <IssueListItem
              key={issue.id}
              issue={issue}
              tags={issue.tagScores ?? issue.tags}
              href={`/issues/${issue.id}`}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

function CorrelationCard({ correlation }: { correlation: PersonCorrelation }) {
  return (
    <div className="app-surface rounded-[1.5rem] p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <AvatarCircle name={correlation.personA} size="sm" />
          <span className="text-sm font-medium">{correlation.personA}</span>
          <span className="text-xs text-muted-foreground">&amp;</span>
          <AvatarCircle name={correlation.personB} size="sm" />
          <span className="text-sm font-medium">{correlation.personB}</span>
        </div>
        <span
          className={cn(
            "rounded-full px-2.5 py-1 text-xs font-semibold tabular-nums",
            scoreColor(correlation.combinedScore)
          )}
        >
          {(correlation.combinedScore * 100).toFixed(0)}% overlap
        </span>
      </div>

      <div className="mt-3 flex flex-wrap gap-3 text-[11px] text-muted-foreground">
        <span>
          Semantic: <span className="font-medium text-foreground">{(correlation.semanticScore * 100).toFixed(0)}%</span>
        </span>
        <span>
          Factor: <span className="font-medium text-foreground">{(correlation.factorScore * 100).toFixed(0)}%</span>
        </span>
        <span>
          {correlation.personA}: {correlation.personAIssueCount} issue{correlation.personAIssueCount === 1 ? "" : "s"}
        </span>
        <span>
          {correlation.personB}: {correlation.personBIssueCount} issue{correlation.personBIssueCount === 1 ? "" : "s"}
        </span>
      </div>

      {correlation.sharedTags.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {correlation.sharedTags.map((tag) => (
            <TagBadge key={tag} tag={tag} />
          ))}
        </div>
      )}
    </div>
  );
}

export function PeoplePage() {
  const [status, setStatus] = useState<PeopleListStatus>("open");
  const { data: correlationsData, error: correlationsError, isLoading: correlationsLoading } =
    useWorkCorrelations(status);
  const { data: allIssues, isLoading: issuesLoading } = useIssues(status);
  const people = useMemo(() => {
    if (!allIssues) return [];

    const byPerson = new Map<
      string,
      { name: string; count: number; tagSums: Map<string, number>; issues: IssueRecord[] }
    >();

    for (const issue of allIssues) {
      if (!issue.assignedTo) continue;

      const key = issue.assignedTo.toLowerCase();
      let entry = byPerson.get(key);
      if (!entry) {
        entry = { name: issue.assignedTo, count: 0, tagSums: new Map(), issues: [] };
        byPerson.set(key, entry);
      }
      entry.count++;
      entry.issues.push(issue);

      for (const ts of issue.tagScores ?? []) {
        entry.tagSums.set(ts.tag, (entry.tagSums.get(ts.tag) ?? 0) + ts.relevance);
      }
    }

    return Array.from(byPerson.values())
      .map((data) => {
        const profile: TagRelevance[] = [];
        for (const [tag, sum] of data.tagSums) {
          profile.push({ tag, relevance: Math.round((sum / data.count) * 100) / 100 });
        }
        profile.sort((a, b) => b.relevance - a.relevance || a.tag.localeCompare(b.tag));
        const sortedIssues = [...data.issues].sort((left, right) => {
          if (left.status !== right.status) {
            return left.status === "open" ? -1 : 1;
          }

          return new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime();
        });

        return {
          person: data.name,
          issueCount: data.count,
          tagProfile: profile,
          issues: sortedIssues,
        };
      })
      .sort((a, b) => b.issueCount - a.issueCount);
  }, [allIssues]);

  const correlations = correlationsData?.correlations ?? [];

  return (
    <AppShell sidebar={<AppSidebar showThingsSection={false} />}>
      <SiteHeader
        title="People"
        eyebrow="Analytics"
        subtitle="Work attribution and correlations"
      />

      <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
        <div className="flex w-full flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
          <FilterToggleGroup
            options={["all", "open", "closed"] as const}
            value={status}
            onChange={setStatus}
          />

          <section>
            <h2 className="text-lg font-semibold tracking-tight">People</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Factor attribution — where each person spends their time based on assigned issue tag scores.
            </p>

            {issuesLoading && (
              <div className="mt-4 text-sm text-muted-foreground">Loading...</div>
            )}

            {!issuesLoading && people.length === 0 && (
              <div className="mt-4 text-sm text-muted-foreground">
                No assigned issues found. Assign issues to people to see their profiles here.
              </div>
            )}

            {people.length > 0 && (
              <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                {people.map((p) => (
                  <PersonProfileCard
                    key={p.person}
                    person={p.person}
                    issueCount={p.issueCount}
                    tagProfile={p.tagProfile}
                    issues={p.issues}
                  />
                ))}
              </div>
            )}
          </section>

          <section>
            <h2 className="text-lg font-semibold tracking-tight">Work Correlations</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Pairs of people working on semantically similar or tag-overlapping issues.
            </p>

            {correlationsLoading && (
              <div className="mt-4 text-sm text-muted-foreground">Loading...</div>
            )}

            {correlationsError && (
              <div className="app-status-warning mt-4 text-sm">
                Failed to load correlations: {correlationsError.message}
              </div>
            )}

            {!correlationsLoading && !correlationsError && correlations.length === 0 && (
              <div className="mt-4 text-sm text-muted-foreground">
                Not enough assigned people to compute correlations. Assign issues to at least two people.
              </div>
            )}

            {correlations.length > 0 && (
              <div className="mt-4 grid gap-4">
                {correlations.map((c) => (
                  <CorrelationCard
                    key={`${c.personA}-${c.personB}`}
                    correlation={c}
                  />
                ))}
              </div>
            )}
          </section>
        </div>
      </div>
    </AppShell>
  );
}
