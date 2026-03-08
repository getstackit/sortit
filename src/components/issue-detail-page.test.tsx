import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IssueDetailPage } from "@/components/issue-detail-page";
import {
  closeIssue,
  fetchIssue,
  refineIssue,
  reopenIssue,
  type IssueRecord,
} from "@/lib/issues";
import { fetchMapData } from "@/features/map/api";

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
  AppShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/app-sidebar", () => ({
  AppSidebar: () => <div>Sidebar</div>,
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

vi.mock("@/features/map/api", () => ({
  fetchMapData: vi.fn(),
}));

vi.mock("@/lib/issues", async () => {
  const actual = await vi.importActual<typeof import("@/lib/issues")>("@/lib/issues");
  return {
    ...actual,
    fetchIssue: vi.fn(),
    refineIssue: vi.fn(),
    closeIssue: vi.fn(),
    reopenIssue: vi.fn(),
  };
});

function makeIssue(overrides: Partial<IssueRecord> = {}): IssueRecord {
  return {
    id: "issue-123",
    raw: "Export fails in Safari after tapping share twice.",
    tags: ["export", "safari"],
    createdBy: "Casey",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    status: "open",
    discussion: [
      {
        id: "issue-123-post-000001",
        issueId: "issue-123",
        raw: "Export fails on iPad.",
        createdBy: "Casey",
        createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
        sequence: 1,
      },
      {
        id: "issue-123-post-000002",
        issueId: "issue-123",
        raw: "It only happens in Safari after tapping share twice.",
        createdBy: "Jordan",
        createdAt: new Date("2026-03-08T12:10:00Z").toISOString(),
        sequence: 2,
      },
    ],
    ...overrides,
  };
}

describe("IssueDetailPage", () => {
  beforeEach(() => {
    vi.mocked(fetchIssue).mockReset();
    vi.mocked(refineIssue).mockReset();
    vi.mocked(closeIssue).mockReset();
    vi.mocked(reopenIssue).mockReset();
    vi.mocked(fetchMapData).mockReset();

    vi.mocked(fetchMapData).mockResolvedValue({
      issues: [],
      edges: [],
      clusters: [],
    });
  });

  it("renders the canonical summary separately from the discussion feed", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(makeIssue());

    render(<IssueDetailPage issueID="issue-123" />);

    expect(await screen.findByText("Canonical summary")).toBeInTheDocument();
    expect(
      screen.getByText("Discussion", {
        selector: "p",
      })
    ).toBeInTheDocument();
    expect(
      screen.getByText("Export fails in Safari after tapping share twice.", {
        selector: "h2",
      })
    ).toBeInTheDocument();
    expect(screen.getByText("Export fails on iPad.")).toBeInTheDocument();
    expect(
      screen.getByText("It only happens in Safari after tapping share twice.")
    ).toBeInTheDocument();
    expect(screen.getByText("Initial report")).toBeInTheDocument();
    expect(screen.getByText("Refinement 1")).toBeInTheDocument();
  });

  it("posts a refinement and refreshes the canonical summary", async () => {
    const initialIssue = makeIssue();
    const refinedIssue = makeIssue({
      raw: "Export fails in Safari on iPad after tapping share twice and hangs forever.",
      tags: ["export", "safari", "performance"],
      discussion: [
        ...(initialIssue.discussion ?? []),
        {
          id: "issue-123-post-000003",
          issueId: "issue-123",
          raw: "It hangs forever after the second tap and never saves the file.",
          createdBy: "You",
          createdAt: new Date("2026-03-08T12:20:00Z").toISOString(),
          sequence: 3,
        },
      ],
    });

    vi.mocked(fetchIssue).mockResolvedValue(initialIssue);
    vi.mocked(refineIssue).mockResolvedValue(refinedIssue);

    render(<IssueDetailPage issueID="issue-123" />);

    await screen.findByText("Canonical summary");

    await userEvent.type(
      screen.getByPlaceholderText("Add more context, corrections, or feedback..."),
      "It hangs forever after the second tap and never saves the file."
    );
    await userEvent.click(screen.getByRole("button", { name: "Post refinement" }));

    await waitFor(() => {
      expect(refineIssue).toHaveBeenCalledWith("issue-123", {
        raw: "It hangs forever after the second tap and never saves the file.",
      });
    });

    expect(
      await screen.findAllByText(
        "Export fails in Safari on iPad after tapping share twice and hangs forever."
      )
    ).toHaveLength(2);
    expect(
      screen.getByText("It hangs forever after the second tap and never saves the file.")
    ).toBeInTheDocument();
    expect(screen.getByText("Refinement 2")).toBeInTheDocument();
  });
});
