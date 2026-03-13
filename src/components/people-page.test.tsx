import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PeoplePage } from "@/components/people-page";
import { useIssues } from "@/hooks/use-issues";
import { useWorkCorrelations } from "@/hooks/use-people";
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
  SiteHeader: ({ title }: { title: string }) => <header><h1>{title}</h1></header>,
}));

vi.mock("@/components/tag-relevance-bars", () => ({
  TagRelevanceBars: () => <div>Tag bars</div>,
}));

vi.mock("@/components/closed-factor-attribution-chart", () => ({
  ClosedFactorAttributionChart: () => <div>Closed factor chart</div>,
}));

vi.mock("@/hooks/use-issues", () => ({
  useIssues: vi.fn(),
}));

vi.mock("@/hooks/use-people", () => ({
  useWorkCorrelations: vi.fn(),
}));

function makeIssue(
  id: string,
  assignedTo: string,
  status: IssueRecord["status"]
): IssueRecord {
  return {
    id,
    raw: `${status} issue`,
    tags: ["bug"],
    tagScores: [{ tag: "bug", relevance: 0.8 }],
    createdBy: "Casey",
    createdAt: new Date("2026-03-09T12:00:00Z").toISOString(),
    closedAt: status === "closed" ? new Date("2026-03-09T15:00:00Z").toISOString() : null,
    assignedTo,
    status,
  };
}

describe("PeoplePage", () => {
  beforeEach(() => {
    vi.mocked(useIssues).mockReset();
    vi.mocked(useWorkCorrelations).mockReset();

    vi.mocked(useIssues).mockImplementation((status) => {
      const issuesByStatus: Record<string, IssueRecord[]> = {
        all: [
          makeIssue("issue-open", "Avery", "open"),
          makeIssue("issue-closed", "Avery", "closed"),
        ],
        open: [makeIssue("issue-open", "Avery", "open")],
        closed: [makeIssue("issue-closed", "Avery", "closed")],
      };

      return {
        data: issuesByStatus[status] ?? [],
        isLoading: false,
      } as ReturnType<typeof useIssues>;
    });

    vi.mocked(useWorkCorrelations).mockReturnValue({
      data: { correlations: [] },
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useWorkCorrelations>);
  });

  it("applies the selected status filter to the people profiles", async () => {
    const user = userEvent.setup();

    render(<PeoplePage />);

    expect(screen.getByText("1 issue assigned")).toBeInTheDocument();
    expect(screen.getByText("open issue")).toBeInTheDocument();
    expect(screen.queryByText("closed issue")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "All" }));

    await waitFor(() => {
      expect(screen.getByText("2 issues assigned")).toBeInTheDocument();
    });
    expect(screen.getByText("closed issue")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Closed" }));

    await waitFor(() => {
      expect(screen.getByText("1 issue assigned")).toBeInTheDocument();
    });
    expect(screen.queryByText("open issue")).not.toBeInTheDocument();
    expect(screen.getByText("closed issue")).toBeInTheDocument();
  });

  it("shows the assigned issues for each person", () => {
    render(<PeoplePage />);

    expect(screen.getByText("Assigned issues")).toBeInTheDocument();
    expect(screen.getByText("open issue")).toBeInTheDocument();
    expect(screen.queryByText("closed issue")).not.toBeInTheDocument();
  });

  it("shows the closed factor timeline section", () => {
    render(<PeoplePage />);

    expect(screen.getByText("Closed Factor Timeline")).toBeInTheDocument();
    expect(screen.getByText("Closed factor chart")).toBeInTheDocument();
  });
});
