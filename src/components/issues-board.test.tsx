import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { IssuesBoard } from "@/components/issues-board";
import type { IssueRecord } from "@/lib/issues";
import { useIssues } from "@/hooks/use-issues";

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

vi.mock("next/navigation", () => ({
  usePathname: () => window.location.pathname,
  useSearchParams: () => new URLSearchParams(window.location.search),
  useRouter: () => ({ push: vi.fn() }),
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

vi.mock("@/components/issue-card", () => ({
  IssueCard: ({ issue }: { issue: IssueRecord }) => <div>{issue.raw}</div>,
}));

vi.mock("@/components/site-header", () => ({
  SiteHeader: ({
    title,
    subtitle,
    meta,
    actions,
  }: {
    title: string;
    subtitle?: string;
    meta?: ReactNode;
    actions?: ReactNode;
  }) => (
    <header>
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
      {meta}
      {actions}
    </header>
  ),
}));

vi.mock("@/hooks/use-compact-mode", () => ({
  CompactModeProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  useCompactMode: () => ({
    isCompact: false,
    toggleCompact: vi.fn(),
  }),
}));

vi.mock("@/components/issues-filter-sidebar", () => ({
  IssuesFilterSidebar: () => <div>Filters</div>,
  applyFilter: (issues: IssueRecord[]) => issues,
  isFilterActive: () => false,
  EMPTY_FILTER: { tags: [], assignees: [], includeClosed: false },
}));

vi.mock("@/hooks/use-issues", () => ({
  useIssues: vi.fn(),
}));

function makeOpenIssue(raw: string, id = "issue-open"): IssueRecord {
  return {
    id,
    raw,
    tags: ["bug"],
    createdBy: "Casey",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    status: "open",
  };
}

describe("IssuesBoard", () => {
  beforeEach(() => {
    vi.mocked(useIssues).mockReset();

    vi.mocked(useIssues).mockReturnValue({
      data: [makeOpenIssue("Open issue from the normal list")],
      error: undefined,
      isLoading: false,
      mutate: vi.fn(),
      isValidating: false,
    } as ReturnType<typeof useIssues>);
  });

  it("renders the issues content inside its own scroll container", () => {
    const { container } = render(<IssuesBoard />);

    const scrollRegion = container.querySelector(".app-scrollarea.overflow-y-auto");
    expect(scrollRegion).not.toBeNull();
  });

  it("renders issues from the useIssues hook", () => {
    render(<IssuesBoard />);

    expect(screen.getByText("Open issue from the normal list")).toBeInTheDocument();
  });

  it("shows the issue count badge", () => {
    render(<IssuesBoard />);

    expect(screen.getByText("1")).toBeInTheDocument();
  });

  it("shows an empty state when no issues exist", () => {
    vi.mocked(useIssues).mockReturnValue({
      data: [],
      error: undefined,
      isLoading: false,
      mutate: vi.fn(),
      isValidating: false,
    } as ReturnType<typeof useIssues>);

    render(<IssuesBoard />);

    expect(screen.getByText("Nothing open right now. Drop something in.")).toBeInTheDocument();
  });

  it("shows loading skeletons while issues are being fetched", () => {
    vi.mocked(useIssues).mockReturnValue({
      data: undefined,
      error: undefined,
      isLoading: true,
      mutate: vi.fn(),
      isValidating: false,
    } as ReturnType<typeof useIssues>);

    const { container } = render(<IssuesBoard />);

    const skeletons = container.querySelectorAll("[data-slot='skeleton']");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("displays an error message when the backend is unavailable", () => {
    vi.mocked(useIssues).mockReturnValue({
      data: undefined,
      error: new Error("Connection refused"),
      isLoading: false,
      mutate: vi.fn(),
      isValidating: false,
    } as ReturnType<typeof useIssues>);

    render(<IssuesBoard />);

    expect(screen.getByText(/Connection refused/)).toBeInTheDocument();
  });
});
