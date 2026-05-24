import {
  computeConvexHull,
  computeBlobPath,
  hashTagColor,
} from "@/features/map/model";

describe("computeConvexHull", () => {
  it("returns empty for fewer than 2 points", () => {
    expect(computeConvexHull([])).toEqual([]);
    expect(computeConvexHull([{ x: 1, y: 1 }])).toEqual([{ x: 1, y: 1 }]);
  });

  it("computes a triangle hull from 3 non-collinear points", () => {
    const hull = computeConvexHull([
      { x: 0, y: 0 },
      { x: 4, y: 0 },
      { x: 2, y: 3 },
    ]);
    expect(hull.length).toBe(3);
  });

  it("excludes interior points", () => {
    const hull = computeConvexHull([
      { x: 0, y: 0 },
      { x: 10, y: 0 },
      { x: 10, y: 10 },
      { x: 0, y: 10 },
      { x: 5, y: 5 }, // interior
    ]);
    expect(hull.length).toBe(4);
    expect(hull).not.toContainEqual({ x: 5, y: 5 });
  });

  it("handles collinear points", () => {
    const hull = computeConvexHull([
      { x: 0, y: 0 },
      { x: 1, y: 0 },
      { x: 2, y: 0 },
    ]);
    expect(hull.length).toBe(2);
  });

  it("handles duplicate points", () => {
    const hull = computeConvexHull([
      { x: 0, y: 0 },
      { x: 0, y: 0 },
      { x: 5, y: 0 },
      { x: 5, y: 5 },
    ]);
    expect(hull.length).toBeGreaterThanOrEqual(3);
  });
});

describe("computeBlobPath", () => {
  it("returns empty string for fewer than 3 points", () => {
    expect(computeBlobPath([], 10)).toBe("");
    expect(computeBlobPath([{ x: 0, y: 0 }], 10)).toBe("");
    expect(
      computeBlobPath(
        [
          { x: 0, y: 0 },
          { x: 1, y: 0 },
        ],
        10
      )
    ).toBe("");
  });

  it("produces a valid SVG path for a triangle", () => {
    const path = computeBlobPath(
      [
        { x: 100, y: 100 },
        { x: 200, y: 100 },
        { x: 150, y: 200 },
      ],
      20
    );
    expect(path).toMatch(/^M /);
    expect(path).toContain("C ");
    expect(path).toMatch(/Z$/);
  });

  it("produces a closed path (ends with Z)", () => {
    const path = computeBlobPath(
      [
        { x: 0, y: 0 },
        { x: 100, y: 0 },
        { x: 100, y: 100 },
        { x: 0, y: 100 },
        { x: 50, y: 50 },
      ],
      30
    );
    expect(path.endsWith("Z")).toBe(true);
  });

  it("applies padding to expand the shape outward", () => {
    const points = [
      { x: 100, y: 100 },
      { x: 200, y: 100 },
      { x: 150, y: 200 },
    ];
    const smallPadding = computeBlobPath(points, 5);
    const largePadding = computeBlobPath(points, 50);
    // Larger padding should produce different (larger) coordinates
    expect(smallPadding).not.toBe(largePadding);
  });

  it("returns empty for collinear points (no area hull)", () => {
    const path = computeBlobPath(
      [
        { x: 0, y: 0 },
        { x: 10, y: 0 },
        { x: 20, y: 0 },
      ],
      10
    );
    expect(path).toBe("");
  });
});

describe("hashTagColor", () => {
  it("returns a color string", () => {
    const color = hashTagColor("feature");
    expect(color).toMatch(/^#[0-9a-f]{6}$/);
  });

  it("returns the same color for the same tag", () => {
    expect(hashTagColor("bug")).toBe(hashTagColor("bug"));
    expect(hashTagColor("feature")).toBe(hashTagColor("feature"));
  });

  it("returns deterministic colors for known tags", () => {
    // Ensure different tags can map to different colors
    const colors = new Set([
      hashTagColor("bug"),
      hashTagColor("feature"),
      hashTagColor("ui"),
      hashTagColor("performance"),
      hashTagColor("search"),
    ]);
    // With 5 palette colors and 5 tags, we should get at least 2 distinct colors
    expect(colors.size).toBeGreaterThanOrEqual(2);
  });
});
