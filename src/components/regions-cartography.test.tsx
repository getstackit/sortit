import { render, screen } from "@testing-library/react";

import { RegionsCartography } from "@/components/regions-cartography";
import type { RegionWithMetrics } from "@/lib/regions";

function makeRegion(overrides: {
  id: string;
  mass?: number;
  growth?: number | null;
  closure?: number | null;
}): RegionWithMetrics {
  return {
    region: { key: { kind: "tag", id: overrides.id }, label: overrides.id },
    metrics: {
      key: { kind: "tag", id: overrides.id },
      window: { label: "30d", start: "", end: "" },
      mass: overrides.mass ?? 1,
      massOpen: overrides.mass ?? 1,
      massClosed: 0,
      growth: overrides.growth != null ? { count: 0, perDay: overrides.growth } : null,
      closure: overrides.closure != null ? { count: 0, perDay: overrides.closure } : null,
    },
  };
}

describe("RegionsCartography", () => {
  it("renders one bubble per region with link to tag detail", () => {
    render(
      <RegionsCartography
        windowLabel="30d"
        regions={[
          makeRegion({ id: "auth", mass: 5, growth: 0.4, closure: 0.1 }),
          makeRegion({ id: "billing", mass: 3, growth: 0.1, closure: 0.5 }),
        ]}
      />
    );

    expect(screen.getByTestId("region-bubble-auth")).toBeInTheDocument();
    expect(screen.getByTestId("region-bubble-billing")).toBeInTheDocument();
    expect(screen.getByTestId("region-bubble-auth").getAttribute("href")).toBe("/tags/auth");
  });

  it("colors growing regions green and shrinking regions red", () => {
    render(
      <RegionsCartography
        windowLabel="30d"
        regions={[
          makeRegion({ id: "growing", mass: 5, growth: 0.6, closure: 0.1 }),
          makeRegion({ id: "shrinking", mass: 5, growth: 0.1, closure: 0.6 }),
        ]}
      />
    );

    const growing = screen.getByTestId("region-bubble-growing");
    const shrinking = screen.getByTestId("region-bubble-shrinking");
    expect(growing.getAttribute("data-fill")).toBe("#16a34a");
    expect(shrinking.getAttribute("data-fill")).toBe("#dc2626");
  });

  it("falls back when no flow data is available (window=all)", () => {
    render(
      <RegionsCartography
        windowLabel="all"
        regions={[makeRegion({ id: "auth", mass: 5, growth: null, closure: null })]}
      />
    );

    expect(screen.getByText(/Cartography needs a finite window/i)).toBeInTheDocument();
    expect(screen.queryByTestId("region-bubble-auth")).not.toBeInTheDocument();
  });

  it("positions larger-mass regions further right", () => {
    render(
      <RegionsCartography
        windowLabel="30d"
        regions={[
          makeRegion({ id: "small", mass: 2, growth: 0.1, closure: 0.05 }),
          makeRegion({ id: "large", mass: 100, growth: 0.1, closure: 0.05 }),
        ]}
      />
    );

    const small = screen.getByTestId("region-bubble-small").querySelector("circle");
    const large = screen.getByTestId("region-bubble-large").querySelector("circle");
    if (!small || !large) throw new Error("missing bubbles");
    expect(parseFloat(large.getAttribute("cx") ?? "0")).toBeGreaterThan(
      parseFloat(small.getAttribute("cx") ?? "0")
    );
  });
});
