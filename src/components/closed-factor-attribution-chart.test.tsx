import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  buildClosedFactorAttributionTimeline,
  ClosedFactorAttributionChart,
} from "@/components/closed-factor-attribution-chart";
import type { IssueRecord } from "@/lib/issues";

function makeClosedIssue(
  id: string,
  closedAt: string,
  options: {
    tags?: string[];
    tagScores?: IssueRecord["tagScores"];
  } = {}
): IssueRecord {
  return {
    id,
    raw: `Closed issue ${id}`,
    tags: options.tags ?? [],
    tagScores: options.tagScores,
    createdBy: "Casey",
    createdAt: new Date("2026-03-10T09:00:00Z").toISOString(),
    closedAt,
    status: "closed",
  };
}

describe("buildClosedFactorAttributionTimeline", () => {
  it("uses hourly buckets for tightly clustered closures and tracks bucket counts", () => {
    const timeline = buildClosedFactorAttributionTimeline([
      makeClosedIssue("issue-1", "2026-03-10T10:10:00Z", {
        tagScores: [
          { tag: "bug", relevance: 0.8 },
          { tag: "infra", relevance: 0.2 },
        ],
      }),
      makeClosedIssue("issue-2", "2026-03-10T10:45:00Z", {
        tags: ["bug", "ops"],
      }),
      makeClosedIssue("issue-3", "2026-03-10T12:05:00Z", {
        tagScores: [{ tag: "ops", relevance: 1 }],
      }),
    ]);

    expect(timeline.granularity).toBe("hour");
    expect(timeline.totalClosed).toBe(3);
    expect(timeline.buckets).toHaveLength(3);
    expect(timeline.buckets[0].label).toContain("10 AM");
    expect(timeline.buckets[0].count).toBe(2);
    expect(timeline.buckets[0].values.bug).toBeCloseTo(1.3);
    expect(timeline.buckets[0].values.infra).toBeCloseTo(0.2);
    expect(timeline.buckets[0].values.ops).toBeCloseTo(0.5);
    expect(timeline.buckets[1].count).toBe(0);
    expect(timeline.buckets[2].count).toBe(1);
    expect(timeline.buckets[2].values.ops).toBeCloseTo(1);
  });

  it("uses daily buckets when issues span more than a week", () => {
    const timeline = buildClosedFactorAttributionTimeline([
      makeClosedIssue("issue-1", "2026-03-01T10:00:00Z", { tags: ["bug"] }),
      makeClosedIssue("issue-2", "2026-03-15T10:00:00Z", { tags: ["bug"] }),
    ]);

    expect(timeline.granularity).toBe("day");
    expect(timeline.buckets.length).toBe(15); // Mar 1 through Mar 15
    expect(timeline.buckets[0].label).toBe("Mar 1");
    expect(timeline.buckets[14].label).toBe("Mar 15");
  });

  it("uses weekly buckets when issues span more than 180 days", () => {
    const timeline = buildClosedFactorAttributionTimeline([
      makeClosedIssue("issue-1", "2025-06-01T10:00:00Z", { tags: ["bug"] }),
      makeClosedIssue("issue-2", "2026-03-10T10:00:00Z", { tags: ["bug"] }),
    ]);

    expect(timeline.granularity).toBe("week");
    // Should have ~40 weekly buckets, not ~6700 hourly buckets
    expect(timeline.buckets.length).toBeLessThan(60);
    expect(timeline.buckets[0].label).toMatch(/^w\/o /);
  });

  it("rolls the long tail into other", () => {
    const timeline = buildClosedFactorAttributionTimeline([
      makeClosedIssue("issue-1", "2026-03-10T10:00:00Z", { tags: ["a"] }),
      makeClosedIssue("issue-2", "2026-03-10T11:00:00Z", { tags: ["b"] }),
      makeClosedIssue("issue-3", "2026-03-10T12:00:00Z", { tags: ["c"] }),
      makeClosedIssue("issue-4", "2026-03-10T13:00:00Z", { tags: ["d"] }),
      makeClosedIssue("issue-5", "2026-03-10T14:00:00Z", { tags: ["e"] }),
      makeClosedIssue("issue-6", "2026-03-10T15:00:00Z", { tags: ["f"] }),
    ]);

    expect(timeline.series).toContain("Other");
    expect(timeline.totalsBySeries.Other).toBeCloseTo(1);
  });
});

describe("ClosedFactorAttributionChart", () => {
  it("renders the chart with axes and controls", () => {
    render(
      <ClosedFactorAttributionChart
        defaultPeriod="all"
        issues={[
          makeClosedIssue("issue-1", "2026-03-10T10:10:00Z", {
            tagScores: [{ tag: "bug", relevance: 1 }],
          }),
        ]}
      />
    );

    expect(screen.getByText("Closed factor attribution")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Closed factor attribution chart" })).toBeInTheDocument();
    expect(screen.getByText("1 closed tickets")).toBeInTheDocument();
    expect(screen.getByText("hour buckets")).toBeInTheDocument();
    // Y-axis label
    expect(screen.getByText("Issues closed")).toBeInTheDocument();
    // Mode toggles
    expect(screen.getByText("Per bucket")).toBeInTheDocument();
    expect(screen.getByText("Cumulative")).toBeInTheDocument();
  });

  it("renders period toggle buttons", () => {
    render(
      <ClosedFactorAttributionChart
        defaultPeriod="all"
        issues={[
          makeClosedIssue("issue-1", "2026-03-10T10:10:00Z", {
            tagScores: [{ tag: "bug", relevance: 1 }],
          }),
        ]}
      />
    );

    expect(screen.getByText("1 week")).toBeInTheDocument();
    expect(screen.getByText("1 month")).toBeInTheDocument();
    expect(screen.getByText("3 months")).toBeInTheDocument();
    expect(screen.getByText("All time")).toBeInTheDocument();
  });

  it("filters issues when period is changed", async () => {
    const now = new Date();
    const recent = new Date(now.getTime() - 3 * 24 * 60 * 60 * 1000).toISOString(); // 3 days ago
    const old = new Date(now.getTime() - 60 * 24 * 60 * 60 * 1000).toISOString(); // 60 days ago

    render(
      <ClosedFactorAttributionChart
        defaultPeriod="all"
        issues={[
          makeClosedIssue("recent", recent, { tagScores: [{ tag: "bug", relevance: 1 }] }),
          makeClosedIssue("old", old, { tagScores: [{ tag: "infra", relevance: 1 }] }),
        ]}
      />
    );

    // All time shows both
    expect(screen.getByText("2 closed tickets")).toBeInTheDocument();

    // Switch to 1 week — only the recent issue should remain
    await userEvent.click(screen.getByText("1 week"));
    expect(screen.getByText("1 closed tickets")).toBeInTheDocument();
  });

  it("switches to cumulative mode and updates y-axis label", async () => {
    render(
      <ClosedFactorAttributionChart
        defaultPeriod="all"
        issues={[
          makeClosedIssue("issue-1", "2026-03-10T10:10:00Z", {
            tagScores: [{ tag: "bug", relevance: 1 }],
          }),
          makeClosedIssue("issue-2", "2026-03-10T12:05:00Z", {
            tagScores: [{ tag: "ops", relevance: 1 }],
          }),
        ]}
      />
    );

    expect(screen.getByText("Issues closed")).toBeInTheDocument();

    await userEvent.click(screen.getByText("Cumulative"));
    expect(screen.getByText("Cumulative issues")).toBeInTheDocument();
  });
});
