import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IssuesBoard } from "@/components/issues-board";
import type { IssueRecord, IssueSearchResponse, SearchIssueRecord } from "@/lib/issues";
import { useIssueSearch, useIssues } from "@/hooks/use-issues";
import type { AppShortcutRegistration } from "@/components/app-shell";

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

vi.mock("motion/react", () => ({
  AnimatePresence: ({ children }: { children: ReactNode }) => <>{children}</>,
  motion: {
    div: ({ children, ...props }: { children: ReactNode }) => <div {...props}>{children}</div>,
  },
}));

vi.mock("next/navigation", () => ({
  usePathname: () => window.location.pathname,
  useSearchParams: () => new URLSearchParams(window.location.search),
}));

let appShellShortcuts: AppShortcutRegistration[] = [];

vi.mock("@/components/app-shell", () => ({
  AppShell: ({
    children,
    sidebar,
    shortcuts = [],
  }: {
    children: ReactNode;
    sidebar: ReactNode;
    shortcuts?: AppShortcutRegistration[];
  }) => {
    appShellShortcuts = shortcuts;

    return (
      <div>
        <aside>{sidebar}</aside>
        <div>
          {shortcuts.map((shortcut) => (
            <span key={shortcut.key}>{shortcut.description}</span>
          ))}
        </div>
        <main>{children}</main>
      </div>
    );
  },
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

vi.mock("@/hooks/use-issues", () => ({
  useIssues: vi.fn(),
  useIssueSearch: vi.fn(),
}));

function makeOpenIssue(raw: string): IssueRecord {
  return {
    id: "issue-open",
    raw,
    tags: ["bug"],
    createdBy: "Casey",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    status: "open",
  };
}

function makeSearchResponse(items: SearchIssueRecord[]): IssueSearchResponse {
  return {
    query: {
      raw: "front end changes",
      tags: [{ tag: "frontend", relevance: 0.95 }],
    },
    relatedIssues: items,
  };
}

describe("IssuesBoard", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/");
    appShellShortcuts = [];
    vi.mocked(useIssues).mockReset();
    vi.mocked(useIssueSearch).mockReset();

    vi.mocked(useIssues).mockReturnValue({
      data: [makeOpenIssue("Open issue from the normal list")],
      error: undefined,
      isLoading: false,
      mutate: vi.fn(),
      isValidating: false,
    } as ReturnType<typeof useIssues>);

    vi.mocked(useIssueSearch).mockImplementation((query, status) => {
      if (query === "front end changes" && status === "open") {
        return {
          data: makeSearchResponse([
            {
              id: "issue-search-open",
              raw: "Frontend polish for issue detail layout",
              status: "open",
              tags: [{ tag: "frontend", relevance: 0.94 }],
              semanticSimilarity: 0.82,
              factorSimilarity: 0.76,
              combinedSimilarity: 0.8,
              reason: "Shared factor relevance in frontend",
            },
          ]),
          error: undefined,
          isLoading: false,
          mutate: vi.fn(),
          isValidating: false,
        } as ReturnType<typeof useIssueSearch>;
      }

      if (query === "front end changes" && status === "all") {
        return {
          data: makeSearchResponse([
            {
              id: "issue-search-closed",
              raw: "Closed frontend follow-up cleanup",
              status: "closed",
              tags: [{ tag: "frontend", relevance: 0.91 }],
              semanticSimilarity: 0.8,
              factorSimilarity: 0.73,
              combinedSimilarity: 0.77,
              reason: "Semantically similar language suggests a shared root cause",
            },
          ]),
          error: undefined,
          isLoading: false,
          mutate: vi.fn(),
          isValidating: false,
        } as ReturnType<typeof useIssueSearch>;
      }

      return {
        data: undefined,
        error: undefined,
        isLoading: false,
        mutate: vi.fn(),
        isValidating: false,
      } as ReturnType<typeof useIssueSearch>;
    });
  });

  it("switches from the open list to semantic search results", async () => {
    render(<IssuesBoard />);

    expect(screen.getByText("Open issue from the normal list")).toBeInTheDocument();
    expect(screen.getByText("Focus semantic search")).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Search issues"), "front end changes");

    await waitFor(() => {
      expect(screen.getByText("Search issues")).toBeInTheDocument();
      expect(screen.getByText("Frontend polish for issue detail layout")).toBeInTheDocument();
    });

    expect(
      screen.getByText("Shared factor relevance in frontend")
    ).toBeInTheDocument();
    expect(screen.queryByText("Open issue from the normal list")).not.toBeInTheDocument();
    expect(window.location.search).toContain("q=front+end+changes");
  });

  it("can include closed issues in semantic search", async () => {
    render(<IssuesBoard />);

    await userEvent.type(screen.getByLabelText("Search issues"), "front end changes");
    await waitFor(() => {
      expect(screen.getByText("Frontend polish for issue detail layout")).toBeInTheDocument();
    });

    await userEvent.click(screen.getByLabelText("Include closed"));

    await waitFor(() => {
      expect(screen.getByText("Closed frontend follow-up cleanup")).toBeInTheDocument();
    });

    expect(screen.getByText("Closed")).toBeInTheDocument();
    expect(vi.mocked(useIssueSearch)).toHaveBeenCalledWith("front end changes", "all", 8);
    expect(window.location.search).toContain("status=all");
  });

  it("restores semantic search state from the URL on load", async () => {
    window.history.replaceState({}, "", "/?q=front%20end%20changes&status=all");

    render(<IssuesBoard />);

    expect(screen.getByDisplayValue("front end changes")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("Closed frontend follow-up cleanup")).toBeInTheDocument();
    });

    expect(vi.mocked(useIssueSearch)).toHaveBeenCalledWith("front end changes", "all", 8);
  });

  it("registers a slash shortcut to focus semantic search", async () => {
    render(<IssuesBoard />);

    const shortcut = appShellShortcuts.find((item) => item.key === "/");
    expect(shortcut?.description).toBe("Focus semantic search");

    const input = screen.getByLabelText("Search issues");
    expect(input).not.toHaveFocus();

    await shortcut?.action();

    expect(input).toHaveFocus();
  });
});
