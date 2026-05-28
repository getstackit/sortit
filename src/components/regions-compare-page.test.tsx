import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";

import { RegionsComparePage } from "@/components/regions-compare-page";
import { useRegion } from "@/hooks/use-region";

const searchParamsBag = { value: new URLSearchParams() };
const routerReplace = vi.fn();

vi.mock("next/navigation", () => ({
  usePathname: () => "/regions/compare",
  useSearchParams: () => searchParamsBag.value,
  useRouter: () => ({ replace: routerReplace }),
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
  }: {
    title: string;
    actions?: ReactNode;
  }) => (
    <header>
      <h1>{title}</h1>
      {actions}
    </header>
  ),
}));

vi.mock("@/hooks/use-region", () => ({
  useRegion: vi.fn(),
}));

function setLocation(search: string) {
  searchParamsBag.value = new URLSearchParams(search);
}

function mockRegion(id: string, mass: number, growthPerDay = 0.1) {
  return {
    data: {
      region: { key: { kind: "tag" as const, id }, label: id },
      metrics: {
        key: { kind: "tag" as const, id },
        window: { label: "30d", start: "", end: "" },
        mass,
        massOpen: mass,
        massClosed: 0,
        growth: { count: 0, perDay: growthPerDay },
        closure: { count: 0, perDay: 0 },
        churn: { count: 0, perDay: 0 },
      },
      window: { label: "30d", start: "", end: "" },
    },
    error: undefined,
    isLoading: false,
  } as ReturnType<typeof useRegion>;
}

beforeEach(() => {
  setLocation("");
  vi.mocked(useRegion).mockReset();
  routerReplace.mockReset();
});

describe("RegionsComparePage", () => {
  it("renders help when params are missing", () => {
    vi.mocked(useRegion).mockReturnValue({
      data: undefined,
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useRegion>);

    render(<RegionsComparePage />);

    expect(screen.getByText(/Pick two regions to compare/i)).toBeInTheDocument();
  });

  it("renders two columns plus a delta column when both keys parse", () => {
    setLocation("a=tag/auth&b=tag/billing");
    vi.mocked(useRegion)
      .mockReturnValueOnce(mockRegion("auth", 10, 0.4))
      .mockReturnValueOnce(mockRegion("billing", 4, 0.1));

    render(<RegionsComparePage />);

    expect(screen.getAllByText("auth").length).toBeGreaterThan(0);
    expect(screen.getAllByText("billing").length).toBeGreaterThan(0);
    expect(screen.getByText("Δ B − A")).toBeInTheDocument();
    // Mass delta is 4 - 10 = -6 (and Open delta matches since massClosed is 0 on both)
    expect(screen.getAllByText("-6").length).toBeGreaterThan(0);
  });

  it("rejects malformed keys", () => {
    setLocation("a=auth&b=tag/billing");
    vi.mocked(useRegion).mockReturnValue({
      data: undefined,
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useRegion>);

    render(<RegionsComparePage />);

    expect(screen.getByText(/Pick two regions to compare/i)).toBeInTheDocument();
  });
});
