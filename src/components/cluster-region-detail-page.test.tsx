import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";

import { ClusterRegionDetailPage } from "@/components/cluster-region-detail-page";
import { useIssues } from "@/hooks/use-issues";
import { useRegion } from "@/hooks/use-region";

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => "/regions/cluster/c-deadbeef",
}));

vi.mock("@/components/app-shell", () => ({
  AppShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/app-sidebar", () => ({
  AppSidebar: () => <div>Sidebar</div>,
}));

vi.mock("@/components/site-header", () => ({
  SiteHeader: ({ title, meta }: { title: string; meta?: ReactNode }) => (
    <header>
      <h1>{title}</h1>
      {meta}
    </header>
  ),
}));

vi.mock("@/components/issue-card", () => ({
  IssueCard: ({ issue }: { issue: { id: string; raw: string } }) => (
    <div data-testid={`issue-card-${issue.id}`}>{issue.raw}</div>
  ),
}));

vi.mock("@/hooks/use-region", () => ({
  useRegion: vi.fn(),
}));

vi.mock("@/hooks/use-issues", () => ({
  useIssues: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(useRegion).mockReset();
  vi.mocked(useIssues).mockReset();
});

function mockRegion(massOpen: number, massClosed: number) {
  vi.mocked(useRegion).mockReturnValue({
    data: {
      region: { key: { kind: "cluster", id: "c-deadbeef" }, label: "auth + tokens" },
      metrics: {
        key: { kind: "cluster", id: "c-deadbeef" },
        window: { label: "30d", start: "", end: "" },
        mass: massOpen + massClosed,
        massOpen,
        massClosed,
        ageBuckets: [
          { label: "<1w", count: 1 },
          { label: "1-4w", count: 2 },
          { label: "1-3m", count: 0 },
          { label: "3m+", count: 0 },
        ],
        growth: { count: 4, perDay: 0.13 },
        closure: { count: 2, perDay: 0.07 },
        churn: { count: 6, perDay: 0.2 },
      },
      window: { label: "30d", start: "", end: "" },
    },
    error: undefined,
    isLoading: false,
  } as ReturnType<typeof useRegion>);
}

describe("ClusterRegionDetailPage", () => {
  it("renders cluster metrics and member tag badges", () => {
    mockRegion(4, 3);
    vi.mocked(useIssues).mockReturnValue({
      data: [],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<ClusterRegionDetailPage id="c-deadbeef" />);

    expect(screen.getByText("auth + tokens")).toBeInTheDocument();
    expect(screen.getByText("Cluster metrics")).toBeInTheDocument();
    expect(screen.getByText("Open")).toBeInTheDocument();
    expect(screen.getByText("Top tags")).toBeInTheDocument();
    expect(screen.getByText("c-deadbeef")).toBeInTheDocument();
  });

  it("renders not-found state when the cluster has zero mass", () => {
    vi.mocked(useRegion).mockReturnValue({
      data: {
        region: { key: { kind: "cluster", id: "c-deadbeef" }, label: "c-deadbeef" },
        metrics: {
          key: { kind: "cluster", id: "c-deadbeef" },
          window: { label: "30d", start: "", end: "" },
          mass: 0,
          massOpen: 0,
          massClosed: 0,
        },
        window: { label: "30d", start: "", end: "" },
      },
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useRegion>);
    vi.mocked(useIssues).mockReturnValue({
      data: [],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<ClusterRegionDetailPage id="c-deadbeef" />);

    expect(screen.getByText(/Cluster not found/i)).toBeInTheDocument();
  });
});
