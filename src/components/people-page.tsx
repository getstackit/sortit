"use client";

import { useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { AppSidebar } from "@/components/app-sidebar";
import { SiteHeader } from "@/components/site-header";
import { TagRelevanceBars } from "@/components/tag-relevance-bars";
import { useWorkCorrelations } from "@/hooks/use-people";
import { useIssues } from "@/hooks/use-issues";
import { entityStyle } from "@/lib/entity-colors";
import { cn } from "@/lib/utils";
import type { PeopleListStatus, PersonCorrelation, TagRelevance } from "@/lib/people";

function scoreColor(score: number) {
  if (score >= 0.7) return "text-emerald-700 bg-emerald-100";
  if (score >= 0.4) return "text-amber-700 bg-amber-100";
  return "text-slate-600 bg-slate-100";
}

function PersonProfileCard({
  person,
  issueCount,
  tagProfile,
}: {
  person: string;
  issueCount: number;
  tagProfile: TagRelevance[];
}) {
  return (
    <div className="app-surface rounded-[1.5rem] p-5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="flex size-8 items-center justify-center rounded-full bg-violet-100 text-sm font-semibold text-violet-700">
            {person.charAt(0).toUpperCase()}
          </span>
          <div>
            <p className="text-sm font-semibold">{person}</p>
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
    </div>
  );
}

function CorrelationCard({ correlation }: { correlation: PersonCorrelation }) {
  return (
    <div className="app-surface rounded-[1.5rem] p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="flex size-7 items-center justify-center rounded-full bg-violet-100 text-xs font-semibold text-violet-700">
            {correlation.personA.charAt(0).toUpperCase()}
          </span>
          <span className="text-sm font-medium">{correlation.personA}</span>
          <span className="text-xs text-muted-foreground">&amp;</span>
          <span className="flex size-7 items-center justify-center rounded-full bg-violet-100 text-xs font-semibold text-violet-700">
            {correlation.personB.charAt(0).toUpperCase()}
          </span>
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
    </div>
  );
}

export function PeoplePage() {
  const [status, setStatus] = useState<PeopleListStatus>("all");
  const { data: correlationsData, error: correlationsError, isLoading: correlationsLoading } =
    useWorkCorrelations(status);
  const { data: allIssues, isLoading: issuesLoading } = useIssues(status);

  const people = useMemo(() => {
    if (!allIssues) return [];

    const byPerson = new Map<string, { name: string; count: number; tagSums: Map<string, number> }>();

    for (const issue of allIssues) {
      if (!issue.assignedTo) continue;

      const key = issue.assignedTo.toLowerCase();
      let entry = byPerson.get(key);
      if (!entry) {
        entry = { name: issue.assignedTo, count: 0, tagSums: new Map() };
        byPerson.set(key, entry);
      }
      entry.count++;

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

        return { person: data.name, issueCount: data.count, tagProfile: profile };
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
          <div className="flex items-center gap-2">
            {(["all", "open", "closed"] as const).map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => setStatus(s)}
                className={cn(
                  "rounded-full px-3 py-1 text-xs font-medium transition-colors",
                  status === s
                    ? "bg-foreground text-background"
                    : "bg-muted text-muted-foreground hover:bg-muted/80"
                )}
              >
                {s.charAt(0).toUpperCase() + s.slice(1)}
              </button>
            ))}
          </div>

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
