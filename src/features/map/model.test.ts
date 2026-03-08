import { analyzeBatch, edgeRenderLimit } from "@/features/map/model";
import type { MapIssue } from "@/features/map/types";

const batchIssues: MapIssue[] = [
  {
    id: "issue-1",
    raw: "Safari export bug",
    status: "open",
    x: 0.1,
    y: 0.2,
    tags: [
      { tag: "bug", relevance: 0.9 },
      { tag: "export", relevance: 0.8 },
    ],
  },
  {
    id: "issue-2",
    raw: "PDF export fails",
    status: "open",
    x: 0.2,
    y: 0.3,
    tags: [
      { tag: "bug", relevance: 0.85 },
      { tag: "export", relevance: 0.75 },
      { tag: "ui", relevance: 0.1 },
    ],
  },
  {
    id: "issue-3",
    raw: "UI layout regression",
    status: "open",
    x: 0.4,
    y: 0.6,
    tags: [
      { tag: "ui", relevance: 0.8 },
      { tag: "bug", relevance: 0.3 },
    ],
  },
];

describe("map model", () => {
  it("summarizes shared and supporting tags for a selected batch", () => {
    const analysis = analyzeBatch(batchIssues);

    expect(analysis).not.toBeNull();
    expect(analysis?.sharedTags.map((tag) => tag.tag)).toContain("bug");
    expect(analysis?.supportingTags.map((tag) => tag.tag)).toContain("export");
    expect(analysis?.pairwise[0]).toMatchObject({
      sourceId: "issue-1",
      targetId: "issue-2",
    });
  });

  it("caps rendered edge counts within the ambient range", () => {
    expect(edgeRenderLimit(0)).toBe(0);
    expect(edgeRenderLimit(10)).toBe(24);
    expect(edgeRenderLimit(2000)).toBe(180);
  });
});
