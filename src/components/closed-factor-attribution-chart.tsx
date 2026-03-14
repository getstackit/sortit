"use client";

import { useMemo } from "react";
import { Badge } from "@/components/ui/badge";
import { entityColors } from "@/lib/entity-colors";
import type { IssueRecord, IssueTagScore } from "@/lib/issues";

type TimelineGranularity = "hour";

type TimelineBucket = {
  key: string;
  label: string;
  count: number;
  total: number;
  values: Record<string, number>;
};

export type ClosedFactorAttributionTimeline = {
  granularity: TimelineGranularity;
  series: string[];
  totalsBySeries: Record<string, number>;
  buckets: TimelineBucket[];
  totalClosed: number;
};

const MAX_SERIES = 5;
const OTHER_SERIES = "Other";
const UNATTRIBUTED_SERIES = "Unattributed";

function sortTagScores(tagScores: IssueTagScore[]): IssueTagScore[] {
  return [...tagScores].sort((left, right) => {
    if (right.relevance !== left.relevance) {
      return right.relevance - left.relevance;
    }
    return left.tag.localeCompare(right.tag);
  });
}

function normalizeContributions(issue: IssueRecord): Map<string, number> {
  const normalized = new Map<string, number>();
  const tagScores = sortTagScores(issue.tagScores ?? []).filter((tag) => tag.relevance > 0);

  if (tagScores.length > 0) {
    const total = tagScores.reduce((sum, tag) => sum + tag.relevance, 0);
    if (total > 0) {
      for (const tag of tagScores) {
        normalized.set(tag.tag, (normalized.get(tag.tag) ?? 0) + tag.relevance / total);
      }
      return normalized;
    }
  }

  const tags = Array.from(new Set((issue.tags ?? []).map((tag) => tag.trim()).filter(Boolean)));
  if (tags.length > 0) {
    const share = 1 / tags.length;
    for (const tag of tags) {
      normalized.set(tag, share);
    }
    return normalized;
  }

  normalized.set(UNATTRIBUTED_SERIES, 1);
  return normalized;
}

function startOfHour(value: Date): Date {
  return new Date(
    Date.UTC(
      value.getUTCFullYear(),
      value.getUTCMonth(),
      value.getUTCDate(),
      value.getUTCHours()
    )
  );
}

function startOfPeriod(value: Date): Date {
  return startOfHour(value);
}

function periodKey(value: Date): string {
  return value.toISOString();
}

function incrementPeriod(value: Date): Date {
  return new Date(
    Date.UTC(
      value.getUTCFullYear(),
      value.getUTCMonth(),
      value.getUTCDate(),
      value.getUTCHours() + 1
    )
  );
}

function formatBucketLabel(value: Date): string {
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    timeZone: "UTC",
  }).format(value);
}

export function buildClosedFactorAttributionTimeline(
  issues: IssueRecord[]
): ClosedFactorAttributionTimeline {
  const closedIssues = issues
    .filter((issue) => issue.status === "closed" && issue.closedAt)
    .map((issue) => {
      const closedAt = new Date(issue.closedAt as string);
      return Number.isNaN(closedAt.getTime()) ? null : { issue, closedAt };
    })
    .filter((entry): entry is { issue: IssueRecord; closedAt: Date } => entry !== null);

  if (closedIssues.length === 0) {
    return {
      granularity: "hour",
      series: [],
      totalsBySeries: {},
      buckets: [],
      totalClosed: 0,
    };
  }

  const granularity: TimelineGranularity = "hour";
  const bucketTotals = new Map<string, Map<string, number>>();
  const bucketCounts = new Map<string, number>();
  const bucketStarts = new Map<string, Date>();
  const totalsBySeries = new Map<string, number>();

  for (const { issue, closedAt } of closedIssues) {
    const start = startOfPeriod(closedAt);
    const key = periodKey(start);
    let bucket = bucketTotals.get(key);
    if (!bucket) {
      bucket = new Map<string, number>();
      bucketTotals.set(key, bucket);
      bucketStarts.set(key, start);
    }
    bucketCounts.set(key, (bucketCounts.get(key) ?? 0) + 1);

    for (const [series, value] of normalizeContributions(issue)) {
      bucket.set(series, (bucket.get(series) ?? 0) + value);
      totalsBySeries.set(series, (totalsBySeries.get(series) ?? 0) + value);
    }
  }

  const sortedSeries = Array.from(totalsBySeries.entries()).sort((left, right) => {
    if (right[1] !== left[1]) {
      return right[1] - left[1];
    }
    return left[0].localeCompare(right[0]);
  });

  const primarySeries = sortedSeries.slice(0, MAX_SERIES).map(([series]) => series);
  const extraSeries = sortedSeries.slice(MAX_SERIES).map(([series]) => series);
  const series = extraSeries.length > 0 ? [...primarySeries, OTHER_SERIES] : primarySeries;

  const allStarts = Array.from(bucketStarts.values()).sort((left, right) => left.getTime() - right.getTime());
  const firstStart = allStarts[0];
  const lastStart = allStarts[allStarts.length - 1];
  const buckets: TimelineBucket[] = [];

  for (
    let current = firstStart;
    current.getTime() <= lastStart.getTime();
    current = incrementPeriod(current)
  ) {
    const key = periodKey(current);
    const source = bucketTotals.get(key) ?? new Map<string, number>();
    const values: Record<string, number> = {};
    let total = 0;

    for (const name of primarySeries) {
      const value = source.get(name) ?? 0;
      values[name] = value;
      total += value;
    }

    if (extraSeries.length > 0) {
      const other = extraSeries.reduce((sum, name) => sum + (source.get(name) ?? 0), 0);
      values[OTHER_SERIES] = other;
      total += other;
    }

    buckets.push({
      key,
      label: formatBucketLabel(current),
      count: bucketCounts.get(key) ?? 0,
      total,
      values,
    });
  }

  const totalsRecord = Object.fromEntries(
    series.map((name) => {
      if (name === OTHER_SERIES) {
        return [name, extraSeries.reduce((sum, tag) => sum + (totalsBySeries.get(tag) ?? 0), 0)];
      }
      return [name, totalsBySeries.get(name) ?? 0];
    })
  );

  return {
    granularity,
    series,
    totalsBySeries: totalsRecord,
    buckets,
    totalClosed: closedIssues.length,
  };
}

function seriesColor(name: string): string {
  if (name === OTHER_SERIES) {
    return "hsl(215 16% 62%)";
  }
  if (name === UNATTRIBUTED_SERIES) {
    return "hsl(28 84% 60%)";
  }
  return entityColors(name).accent;
}

function formatCount(value: number): string {
  const rounded = Math.round(value * 10) / 10;
  return Number.isInteger(rounded) ? `${rounded}` : rounded.toFixed(1);
}

export function ClosedFactorAttributionChart({ issues }: { issues: IssueRecord[] }) {
  const timeline = useMemo(() => buildClosedFactorAttributionTimeline(issues), [issues]);

  if (timeline.totalClosed === 0) {
    return (
      <div className="rounded-[1.5rem] border border-dashed border-border/70 bg-muted/30 px-5 py-10 text-center text-sm text-muted-foreground">
        No closed issues with factor data yet.
      </div>
    );
  }

  const width = Math.max(720, timeline.buckets.length * 84);
  const height = 320;
  const marginTop = 20;
  const marginRight = 18;
  const marginBottom = 58;
  const marginLeft = 48;
  const plotWidth = width - marginLeft - marginRight;
  const plotHeight = height - marginTop - marginBottom;
  const step = timeline.buckets.length > 0 ? plotWidth / timeline.buckets.length : plotWidth;
  const barWidth = Math.max(16, step - 20);
  const yMax = Math.max(1, ...timeline.buckets.map((bucket) => bucket.count));
  const labelStep = Math.max(1, Math.ceil(timeline.buckets.length / 6));
  const gridTicks = [0, yMax / 2, yMax];

  return (
    <div className="rounded-[1.6rem] border border-border/70 bg-[linear-gradient(180deg,color-mix(in_oklab,var(--background)_96%,white_4%)_0%,color-mix(in_oklab,var(--background)_92%,var(--gradient-end)_8%)_100%)] p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
            Closed factor attribution
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Each closed ticket contributes one total unit, split across its factor scores and grouped by close date.
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            Numbers above each bar show the closed-issue count for that bucket.
          </p>
        </div>
        <div className="flex flex-wrap gap-2 text-xs">
          <Badge variant="outline" className="tabular-nums">{timeline.totalClosed} closed tickets</Badge>
          <Badge variant="outline" className="capitalize">{timeline.granularity} buckets</Badge>
        </div>
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {timeline.series.map((series) => (
          <div
            key={series}
            className="inline-flex items-center gap-2 rounded-full border border-border/60 bg-background/80 px-3 py-1 text-xs text-muted-foreground"
          >
            <span
              aria-hidden="true"
              className="size-2.5 rounded-full"
              style={{ backgroundColor: seriesColor(series) }}
            />
            <span className="font-medium text-foreground">{series}</span>
            <span className="tabular-nums">{formatCount(timeline.totalsBySeries[series] ?? 0)}</span>
          </div>
        ))}
      </div>

      <div className="mt-5 overflow-x-auto">
        <svg
          role="img"
          aria-label="Closed factor attribution chart"
          viewBox={`0 0 ${width} ${height}`}
          className="min-w-full"
        >
          {gridTicks.map((tick) => {
            const y = marginTop + plotHeight - (tick / yMax) * plotHeight;
            return (
              <g key={tick}>
                <line
                  x1={marginLeft}
                  y1={y}
                  x2={marginLeft + plotWidth}
                  y2={y}
                  stroke="rgba(100, 116, 139, 0.2)"
                  strokeWidth="1"
                />
                <text
                  x={marginLeft - 10}
                  y={y + 4}
                  textAnchor="end"
                  fontSize="11"
                  fill="rgba(100, 116, 139, 0.9)"
                >
                  {formatCount(tick)}
                </text>
              </g>
            );
          })}

          {timeline.buckets.map((bucket, index) => {
            const x = marginLeft + index * step + (step - barWidth) / 2;
            let offset = 0;

            return (
              <g key={bucket.key}>
                {timeline.series.map((series) => {
                  const value = bucket.values[series] ?? 0;
                  if (value <= 0) return null;

                  const barHeight = (value / yMax) * plotHeight;
                  const y = marginTop + plotHeight - offset - barHeight;
                  offset += barHeight;

                  return (
                    <rect
                      key={series}
                      x={x}
                      y={y}
                      width={barWidth}
                      height={barHeight}
                      rx={Math.min(6, barWidth / 2)}
                      fill={seriesColor(series)}
                      opacity={0.92}
                    >
                      <title>{`${bucket.label}: ${series} ${formatCount(value)}`}</title>
                    </rect>
                  );
                })}

                {bucket.total > 0 && (
                  <>
                    <text
                      x={x + barWidth / 2}
                      y={marginTop + plotHeight - (bucket.count / yMax) * plotHeight - 9}
                      textAnchor="middle"
                      fontSize="11"
                      fontWeight="700"
                      fill="rgba(15, 23, 42, 0.92)"
                    >
                      {bucket.count}
                    </text>
                    <title>{`${bucket.label}: ${bucket.count} closed issue${bucket.count === 1 ? "" : "s"}`}</title>
                  </>
                )}

                {index % labelStep === 0 && (
                  <text
                    x={x + barWidth / 2}
                    y={height - 18}
                    textAnchor="middle"
                    fontSize="11"
                    fill="rgba(100, 116, 139, 0.95)"
                  >
                    {bucket.label}
                  </text>
                )}
              </g>
            );
          })}
        </svg>
      </div>
    </div>
  );
}
