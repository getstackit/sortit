"use client";

import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { FilterToggleGroup } from "@/components/filter-toggle";
import { entityColors } from "@/lib/entity-colors";
import type { IssueRecord, IssueTagScore } from "@/lib/issues";

export type HistoryPeriod = "1w" | "1m" | "3m" | "6m" | "1y" | "all";

const HISTORY_PERIODS: readonly HistoryPeriod[] = ["1w", "1m", "3m", "6m", "1y", "all"] as const;

const PERIOD_LABELS: Record<HistoryPeriod, string> = {
  "1w": "1 week",
  "1m": "1 month",
  "3m": "3 months",
  "6m": "6 months",
  "1y": "1 year",
  all: "All time",
};

function periodCutoff(period: HistoryPeriod, now: Date): Date | null {
  if (period === "all") return null;
  const cutoff = new Date(now);
  switch (period) {
    case "1w":
      cutoff.setDate(cutoff.getDate() - 7);
      break;
    case "1m":
      cutoff.setMonth(cutoff.getMonth() - 1);
      break;
    case "3m":
      cutoff.setMonth(cutoff.getMonth() - 3);
      break;
    case "6m":
      cutoff.setMonth(cutoff.getMonth() - 6);
      break;
    case "1y":
      cutoff.setFullYear(cutoff.getFullYear() - 1);
      break;
  }
  return cutoff;
}

type TimelineGranularity = "hour" | "day" | "week";

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

const HOURS_MS = 3_600_000;
const DAYS_MS = 86_400_000;

function chooseGranularity(firstDate: Date, lastDate: Date): TimelineGranularity {
  const spanMs = lastDate.getTime() - firstDate.getTime();
  const spanDays = spanMs / DAYS_MS;
  if (spanDays <= 7) return "hour";
  if (spanDays <= 180) return "day";
  return "week";
}

function startOfGranularity(value: Date, granularity: TimelineGranularity): Date {
  switch (granularity) {
    case "hour":
      return new Date(Date.UTC(value.getUTCFullYear(), value.getUTCMonth(), value.getUTCDate(), value.getUTCHours()));
    case "day":
      return new Date(Date.UTC(value.getUTCFullYear(), value.getUTCMonth(), value.getUTCDate()));
    case "week": {
      const d = new Date(Date.UTC(value.getUTCFullYear(), value.getUTCMonth(), value.getUTCDate()));
      const day = d.getUTCDay();
      d.setUTCDate(d.getUTCDate() - day); // start of week = Sunday
      return d;
    }
  }
}

function bucketKey(value: Date): string {
  return value.toISOString();
}

function incrementGranularity(value: Date, granularity: TimelineGranularity): Date {
  switch (granularity) {
    case "hour":
      return new Date(value.getTime() + HOURS_MS);
    case "day":
      return new Date(value.getTime() + DAYS_MS);
    case "week":
      return new Date(value.getTime() + 7 * DAYS_MS);
  }
}

function formatBucketLabel(value: Date, granularity: TimelineGranularity): string {
  switch (granularity) {
    case "hour":
      return new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", hour: "numeric", timeZone: "UTC" }).format(value);
    case "day":
      return new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", timeZone: "UTC" }).format(value);
    case "week":
      return `w/o ${new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", timeZone: "UTC" }).format(value)}`;
  }
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
      granularity: "day",
      series: [],
      totalsBySeries: {},
      buckets: [],
      totalClosed: 0,
    };
  }

  const sortedDates = closedIssues.map((e) => e.closedAt).sort((a, b) => a.getTime() - b.getTime());
  const granularity = chooseGranularity(sortedDates[0], sortedDates[sortedDates.length - 1]);

  const bucketTotals = new Map<string, Map<string, number>>();
  const bucketCounts = new Map<string, number>();
  const bucketStarts = new Map<string, Date>();
  const totalsBySeries = new Map<string, number>();

  for (const { issue, closedAt } of closedIssues) {
    const start = startOfGranularity(closedAt, granularity);
    const key = bucketKey(start);
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
    current = incrementGranularity(current, granularity)
  ) {
    const key = bucketKey(current);
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
      label: formatBucketLabel(current, granularity),
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

function niceYTicks(max: number, targetCount = 5): number[] {
  if (max <= 0) return [0];
  const rough = max / targetCount;
  const magnitude = 10 ** Math.floor(Math.log10(rough));
  const candidates = [1, 2, 2.5, 5, 10];
  const step = magnitude * (candidates.find((c) => c * magnitude >= rough) ?? 10);
  const ticks: number[] = [];
  for (let v = 0; v <= max + step * 0.01; v += step) {
    ticks.push(Math.round(v * 1e6) / 1e6);
  }
  return ticks;
}

type ChartMode = "per-bucket" | "cumulative";

function buildCumulativeBuckets(
  buckets: TimelineBucket[],
  series: string[]
): TimelineBucket[] {
  const running: Record<string, number> = {};
  let runningCount = 0;
  for (const s of series) running[s] = 0;

  return buckets.map((bucket) => {
    const values: Record<string, number> = {};
    let total = 0;
    runningCount += bucket.count;
    for (const s of series) {
      running[s] += bucket.values[s] ?? 0;
      values[s] = running[s];
      total += running[s];
    }
    return { ...bucket, count: runningCount, total, values };
  });
}

export function ClosedFactorAttributionChart({
  issues,
  defaultPeriod = "1m",
}: {
  issues: IssueRecord[];
  defaultPeriod?: HistoryPeriod;
}) {
  const [period, setPeriod] = useState<HistoryPeriod>(defaultPeriod);
  const [mode, setMode] = useState<ChartMode>("per-bucket");

  const filteredIssues = useMemo(() => {
    const cutoff = periodCutoff(period, new Date());
    if (!cutoff) return issues;
    const cutoffTime = cutoff.getTime();
    return issues.filter((issue) => {
      if (!issue.closedAt) return false;
      return new Date(issue.closedAt).getTime() >= cutoffTime;
    });
  }, [issues, period]);

  const timeline = useMemo(() => buildClosedFactorAttributionTimeline(filteredIssues), [filteredIssues]);

  const displayBuckets = useMemo(
    () =>
      mode === "cumulative"
        ? buildCumulativeBuckets(timeline.buckets, timeline.series)
        : timeline.buckets,
    [timeline, mode]
  );

  if (timeline.totalClosed === 0) {
    return (
      <div className="rounded-[1.5rem] border border-dashed border-border/70 bg-muted/30 px-5 py-10 text-center text-sm text-muted-foreground">
        No closed issues with factor data yet.
      </div>
    );
  }

  const width = Math.max(720, displayBuckets.length * 84);
  const height = 360;
  const marginTop = 24;
  const marginRight = 18;
  const marginBottom = 80;
  const marginLeft = 56;
  const plotWidth = width - marginLeft - marginRight;
  const plotHeight = height - marginTop - marginBottom;
  const step = displayBuckets.length > 0 ? plotWidth / displayBuckets.length : plotWidth;
  const barWidth = Math.max(16, step - 20);
  const yMax = Math.max(1, ...displayBuckets.map((b) => b.count));
  const yTicks = niceYTicks(yMax);
  const yAxisMax = yTicks[yTicks.length - 1];
  const labelStep = Math.max(1, Math.ceil(displayBuckets.length / 8));
  const tickLen = 6;
  const xAxisY = marginTop + plotHeight;

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
        </div>
        <div className="flex flex-col items-end gap-2">
          <FilterToggleGroup
            options={HISTORY_PERIODS}
            value={period}
            onChange={setPeriod}
            formatLabel={(v) => PERIOD_LABELS[v]}
          />
          <FilterToggleGroup
            options={["per-bucket", "cumulative"] as const}
            value={mode}
            onChange={setMode}
            formatLabel={(v) => (v === "per-bucket" ? "Per bucket" : "Cumulative")}
          />
          <div className="flex flex-wrap gap-2 text-xs">
            <Badge variant="outline" className="tabular-nums">{timeline.totalClosed} closed tickets</Badge>
            <Badge variant="outline" className="capitalize">{timeline.granularity} buckets</Badge>
          </div>
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
          width={Math.max(width, 720)}
          height={height}
          className="chart-svg"
          style={
            {
              minWidth: "100%",
              "--axis-color": "var(--muted-foreground)",
              "--grid-color": "var(--border)",
              "--label-color": "var(--muted-foreground)",
              "--count-color": "var(--foreground)",
            } as React.CSSProperties
          }
        >
          {/* Y-axis label */}
          <text
            x={14}
            y={marginTop + plotHeight / 2}
            textAnchor="middle"
            fontSize="11"
            fontWeight="600"
            fill="var(--label-color)"
            transform={`rotate(-90, 14, ${marginTop + plotHeight / 2})`}
          >
            {mode === "cumulative" ? "Cumulative issues" : "Issues closed"}
          </text>

          {/* Y-axis line */}
          <line
            x1={marginLeft}
            y1={marginTop}
            x2={marginLeft}
            y2={xAxisY}
            stroke="var(--axis-color)"
            strokeWidth="1.5"
          />

          {/* X-axis line */}
          <line
            x1={marginLeft}
            y1={xAxisY}
            x2={marginLeft + plotWidth}
            y2={xAxisY}
            stroke="var(--axis-color)"
            strokeWidth="1.5"
          />

          {/* Y-axis grid lines and tick labels */}
          {yTicks.map((tick) => {
            const y = marginTop + plotHeight - (tick / yAxisMax) * plotHeight;
            return (
              <g key={tick}>
                {tick > 0 && (
                  <line
                    x1={marginLeft + 1}
                    y1={y}
                    x2={marginLeft + plotWidth}
                    y2={y}
                    stroke="var(--grid-color)"
                    strokeWidth="1"
                    strokeDasharray="4 3"
                  />
                )}
                <line
                  x1={marginLeft - tickLen}
                  y1={y}
                  x2={marginLeft}
                  y2={y}
                  stroke="var(--axis-color)"
                  strokeWidth="1.5"
                />
                <text
                  x={marginLeft - tickLen - 4}
                  y={y + 4}
                  textAnchor="end"
                  fontSize="11"
                  fill="var(--label-color)"
                >
                  {formatCount(tick)}
                </text>
              </g>
            );
          })}

          {/* Bars and X-axis labels */}
          {displayBuckets.map((bucket, index) => {
            const x = marginLeft + index * step + (step - barWidth) / 2;
            const barCenter = x + barWidth / 2;
            let offset = 0;

            return (
              <g key={bucket.key}>
                {timeline.series.map((series) => {
                  const value = bucket.values[series] ?? 0;
                  if (value <= 0) return null;

                  const barHeight = (value / yAxisMax) * plotHeight;
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
                  <text
                    x={barCenter}
                    y={marginTop + plotHeight - (bucket.count / yAxisMax) * plotHeight - 8}
                    textAnchor="middle"
                    fontSize="10"
                    fontWeight="700"
                    fill="var(--count-color)"
                  >
                    {bucket.count}
                  </text>
                )}

                {/* X-axis tick + label */}
                {index % labelStep === 0 && (
                  <>
                    <line
                      x1={barCenter}
                      y1={xAxisY}
                      x2={barCenter}
                      y2={xAxisY + tickLen}
                      stroke="var(--axis-color)"
                      strokeWidth="1.5"
                    />
                    <text
                      x={0}
                      y={0}
                      textAnchor="end"
                      fontSize="11"
                      fill="var(--label-color)"
                      transform={`translate(${barCenter}, ${xAxisY + tickLen + 6}) rotate(-40)`}
                    >
                      {bucket.label}
                    </text>
                  </>
                )}
              </g>
            );
          })}
        </svg>
      </div>
    </div>
  );
}
