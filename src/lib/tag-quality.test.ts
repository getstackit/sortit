import {
  buildConsolidationCandidates,
  buildMergeCandidates,
  buildSpecificTagSuggestions,
  isGenericBucketTag,
} from "@/lib/tag-quality";
import type { IssueRecord } from "@/lib/issues";
import type { TagRecord } from "@/lib/tags";

function makeTag(overrides: Partial<TagRecord>): TagRecord {
  return {
    name: "tag",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    embedding: [1, 0],
    ...overrides,
  };
}

function makeIssue(overrides: Partial<IssueRecord>): IssueRecord {
  return {
    id: "issue-1",
    raw: "Issue",
    tags: [],
    createdBy: "Jonathan Goldman",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    status: "open",
    ...overrides,
  };
}

describe("tag quality helpers", () => {
  it("recognizes generic bucket tags", () => {
    expect(isGenericBucketTag("backend")).toBe(true);
    expect(isGenericBucketTag("Billing")).toBe(false);
  });

  it("finds more specific co-occurring tags for generic buckets", () => {
    const suggestions = buildSpecificTagSuggestions(
      "backend",
      [
        makeTag({ name: "backend" }),
        makeTag({ name: "billing", description: "Invoices and account charges" }),
        makeTag({ name: "payments", description: "Payment processing" }),
      ],
      [
        makeIssue({
          id: "issue-1",
          tags: ["backend", "billing"],
          tagScores: [
            { tag: "backend", relevance: 0.9 },
            { tag: "billing", relevance: 0.8 },
          ],
        }),
        makeIssue({
          id: "issue-2",
          tags: ["backend", "payments"],
          tagScores: [
            { tag: "backend", relevance: 0.85 },
            { tag: "payments", relevance: 0.7 },
          ],
        }),
      ]
    );

    expect(suggestions.map((entry) => entry.name)).toEqual(["billing", "payments"]);
  });

  it("finds merge candidates from name variants and semantic overlap", () => {
    const candidates = buildMergeCandidates(
      makeTag({ name: "frontend", embedding: [1, 0] }),
      [
        makeTag({ name: "front end", embedding: [1, 0] }),
        makeTag({ name: "ui", embedding: [0.6, 0.4] }),
        makeTag({ name: "client shell", embedding: [0.96, 0.04] }),
      ]
    );

    expect(candidates[0]?.name).toBe("front end");
    expect(candidates[0]?.reason).toBe("Name variant");
    expect(candidates.some((candidate) => candidate.name === "client shell")).toBe(true);
  });

  it("finds consolidation candidates from lexical variants and shared issue coverage", () => {
    const candidates = buildConsolidationCandidates(
      [
        makeTag({
          name: "frontend",
          description: "Client-side runtime behavior",
          createdAt: new Date("2026-03-07T12:00:00Z").toISOString(),
          embedding: [1, 0],
        }),
        makeTag({
          name: "front end",
          description: "Spacing variant",
          createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
          embedding: [1, 0],
        }),
        makeTag({
          name: "billing",
          description: "Invoices and account charges",
          embedding: [0, 1],
        }),
      ],
      [
        makeIssue({
          id: "issue-1",
          tags: ["frontend", "front end"],
          tagScores: [
            { tag: "frontend", relevance: 0.95 },
            { tag: "front end", relevance: 0.88 },
          ],
        }),
        makeIssue({
          id: "issue-2",
          tags: ["frontend", "front end"],
          tagScores: [
            { tag: "frontend", relevance: 0.91 },
            { tag: "front end", relevance: 0.82 },
          ],
        }),
      ]
    );

    expect(candidates).toHaveLength(1);
    expect(candidates[0]?.canonicalName).toBe("frontend");
    expect(candidates[0]?.aliasName).toBe("front end");
    expect(candidates[0]?.reason).toContain("Name variant");
    expect(candidates[0]?.sharedIssueCount).toBe(2);
  });
});
