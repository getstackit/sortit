import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { AnalyticsPage } from "@/components/analytics-page";
import { useIssues } from "@/hooks/use-issues";
import type { IssueRecord } from "@/lib/issues";

vi.mock("next/link", () => ({
  default: ({
    children,
    href,
    ...props
  }: {
    children: ReactNode;
    href: string;
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock("@/components/app-shell", () => ({
  AppShell: ({
    children,
    sidebar,
  }: {
    children: ReactNode;
    sidebar: ReactNode;
  }) => (
    <div>
      <aside>{sidebar}</aside>
      <main>{children}</main>
    </div>
  ),
}));

vi.mock("@/components/app-sidebar", () => ({
  AppSidebar: () => <div>Sidebar</div>,
}));

vi.mock("@/components/site-header", () => ({
  SiteHeader: ({ title }: { title: string }) => (
    <header>
      <h1>{title}</h1>
    </header>
  ),
}));

vi.mock("@/components/closed-factor-attribution-chart", () => ({
  ClosedFactorAttributionChart: () => <div>Closed factor chart</div>,
}));

vi.mock("@/hooks/use-issues", () => ({
  useIssues: vi.fn(),
}));

function makeClosedIssue(id: string): IssueRecord {
  return {
    id,
    raw: `Closed issue ${id}`,
    tags: ["bug"],
    tagScores: [{ tag: "bug", relevance: 0.8 }],
    createdBy: "Casey",
    createdAt: new Date("2026-03-10T09:00:00Z").toISOString(),
    closedAt: new Date("2026-03-10T15:00:00Z").toISOString(),
    status: "closed",
  };
}

describe("AnalyticsPage", () => {
  beforeEach(() => {
    vi.mocked(useIssues).mockReturnValue({
      data: [makeClosedIssue("issue-1")],
      isLoading: false,
    } as ReturnType<typeof useIssues>);
  });

  it("renders the analytics page with closed factor timeline", () => {
    render(<AnalyticsPage />);

    expect(screen.getByText("Analytics")).toBeInTheDocument();
    expect(screen.getByText("Closed Factor Timeline")).toBeInTheDocument();
    expect(screen.getByText("Closed factor chart")).toBeInTheDocument();
  });

  it("shows loading state", () => {
    vi.mocked(useIssues).mockReturnValue({
      data: undefined,
      isLoading: true,
    } as ReturnType<typeof useIssues>);

    render(<AnalyticsPage />);

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("shows error state", () => {
    vi.mocked(useIssues).mockReturnValue({
      data: undefined,
      error: new Error("Network error"),
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<AnalyticsPage />);

    expect(screen.getByText(/Failed to load closed issue timeline/)).toBeInTheDocument();
  });
});
