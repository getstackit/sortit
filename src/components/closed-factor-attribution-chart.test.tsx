import { render, screen } from "@testing-library/react";
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
  it("renders the bucket count guidance and chart", () => {
    render(
      <ClosedFactorAttributionChart
        issues={[
          makeClosedIssue("issue-1", "2026-03-10T10:10:00Z", {
            tagScores: [{ tag: "bug", relevance: 1 }],
          }),
        ]}
      />
    );

    expect(screen.getByText("Closed factor attribution")).toBeInTheDocument();
    expect(
      screen.getByText("Numbers above each bar show the closed-issue count for that bucket.")
    ).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Closed factor attribution chart" })).toBeInTheDocument();
    expect(screen.getByText("1 closed tickets")).toBeInTheDocument();
    expect(screen.getByText("hour buckets")).toBeInTheDocument();
  });
});
