import type { ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";

import { RegionsPage } from "@/components/regions-page";
import { useRegions } from "@/hooks/use-regions";

const searchParamsBag = { value: new URLSearchParams() };

vi.mock("next/navigation", () => ({
  usePathname: () => "/regions",
  useSearchParams: () => searchParamsBag.value,
}));

vi.mock("@/components/app-shell", () => ({
  AppShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/app-sidebar", () => ({
  AppSidebar: () => <div>Sidebar</div>,
}));

vi.mock("@/components/site-header", () => ({
  SiteHeader: ({
    title,
    actions,
    meta,
  }: {
    title: string;
    actions?: ReactNode;
    meta?: ReactNode;
  }) => (
    <header>
      <h1>{title}</h1>
      {meta}
      {actions}
    </header>
  ),
}));

vi.mock("@/hooks/use-regions", () => ({
  useRegions: vi.fn(),
}));

const replaceState = vi.fn();

function setLocation(search: string) {
  searchParamsBag.value = new URLSearchParams(search);
}

beforeEach(() => {
  setLocation("");
  vi.mocked(useRegions).mockReset();
  replaceState.mockReset();
  window.history.replaceState = replaceState;
});

function makeRegion(overrides: {
  id: string;
  mass?: number;
  massOpen?: number;
  massClosed?: number;
  growth?: number | null;
  closure?: number | null;
}) {
  return {
    region: { key: { kind: "tag" as const, id: overrides.id }, label: overrides.id },
    metrics: {
      key: { kind: "tag" as const, id: overrides.id },
      window: { label: "30d", start: "", end: "" },
      mass: overrides.mass ?? 1,
      massOpen: overrides.massOpen ?? 1,
      massClosed: overrides.massClosed ?? 0,
      ageBuckets: [
        { label: "<1w", count: 1 },
        { label: "1-4w", count: 0 },
        { label: "1-3m", count: 0 },
        { label: "3m+", count: 0 },
      ],
      growth: overrides.growth != null ? { count: 0, perDay: overrides.growth } : null,
      closure: overrides.closure != null ? { count: 0, perDay: overrides.closure } : null,
    },
  };
}

function mockRegions(
  regions: ReturnType<typeof makeRegion>[],
  orphans?: { total: number; open: number; fraction: number }
) {
  vi.mocked(useRegions).mockReturnValue({
    data: {
      regions,
      window: { label: "30d", start: "", end: "" },
      orphans: orphans ?? null,
    },
    error: undefined,
    isLoading: false,
  } as ReturnType<typeof useRegions>);
}

describe("RegionsPage", () => {
  it("renders region tiles", () => {
    mockRegions([
      makeRegion({ id: "auth", mass: 5, massOpen: 3, massClosed: 2 }),
      makeRegion({ id: "billing", mass: 2, massOpen: 1, massClosed: 1 }),
    ]);

    render(<RegionsPage />);

    expect(screen.getByText("auth")).toBeInTheDocument();
    expect(screen.getByText("billing")).toBeInTheDocument();
    expect(screen.getByText("2 regions")).toBeInTheDocument();
  });

  it("sorts by mass desc by default", () => {
    mockRegions([
      makeRegion({ id: "billing", mass: 2 }),
      makeRegion({ id: "auth", mass: 5 }),
    ]);

    render(<RegionsPage />);

    const tagBadges = screen.getAllByText(/auth|billing/);
    const ordering = tagBadges.map((el) => el.textContent).filter(Boolean) as string[];
    expect(ordering[0]).toBe("auth");
  });

  it("sorts by name when sort=name in URL", () => {
    setLocation("sort=name");
    mockRegions([
      makeRegion({ id: "billing", mass: 5 }),
      makeRegion({ id: "auth", mass: 2 }),
    ]);

    render(<RegionsPage />);

    const tagBadges = screen.getAllByText(/auth|billing/);
    const ordering = tagBadges.map((el) => el.textContent).filter(Boolean) as string[];
    expect(ordering[0]).toBe("auth");
  });

  it("shows empty state when no regions", () => {
    mockRegions([]);

    render(<RegionsPage />);

    expect(screen.getByText(/No regions yet/i)).toBeInTheDocument();
  });

  it("shows error state", () => {
    vi.mocked(useRegions).mockReturnValue({
      data: undefined,
      error: new Error("boom"),
      isLoading: false,
    } as ReturnType<typeof useRegions>);

    render(<RegionsPage />);

    expect(screen.getByText(/Failed to load regions/i)).toBeInTheDocument();
  });

  it("renders orphans card when corpus has dead zones", () => {
    mockRegions(
      [makeRegion({ id: "auth", mass: 5 })],
      { total: 7, open: 4, fraction: 0.12 }
    );

    render(<RegionsPage />);

    expect(screen.getByText(/Dead zones/i)).toBeInTheDocument();
    expect(screen.getByText(/7 issues don't fit any region/i)).toBeInTheDocument();
  });

  it("hides orphans card when zero", () => {
    mockRegions(
      [makeRegion({ id: "auth", mass: 5 })],
      { total: 0, open: 0, fraction: 0 }
    );

    render(<RegionsPage />);

    expect(screen.queryByText(/Dead zones/i)).not.toBeInTheDocument();
  });

  it("renders cartography when view=map in URL", () => {
    setLocation("view=map");
    mockRegions([
      makeRegion({ id: "auth", mass: 5, growth: 0.4, closure: 0.1 }),
    ]);

    render(<RegionsPage />);

    expect(screen.getByTestId("region-bubble-auth")).toBeInTheDocument();
  });

  it("updates URL when window selector changes", () => {
    mockRegions([makeRegion({ id: "auth" })]);

    render(<RegionsPage />);

    const sevenDay = screen.getAllByRole("button").find((el) => el.textContent === "7d");
    if (!sevenDay) throw new Error("7d toggle not found");
    fireEvent.click(sevenDay);

    expect(replaceState).toHaveBeenCalled();
    const lastCall = replaceState.mock.calls.at(-1);
    expect(lastCall?.[2]).toContain("window=7d");
  });
});
