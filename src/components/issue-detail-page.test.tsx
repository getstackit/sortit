import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useParams } from "next/navigation";
import { SWRConfig } from "swr";
import { IssueDetailPage } from "@/components/issue-detail-page";
import {
  closeIssue,
  fetchIssue,
  fetchRevision,
  progressIssue,
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

vi.mock("next/navigation", () => ({
  useParams: vi.fn(),
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

vi.mock("@/components/auth-provider", () => ({
  useAuth: () => ({ user: { displayName: "TestUser" }, loading: false }),
}));

vi.mock("@/features/map/api", () => ({
  fetchMapData: vi.fn(),
}));

vi.mock("@/lib/issues", async () => {
  const actual = await vi.importActual<typeof import("@/lib/issues")>("@/lib/issues");
  return {
    ...actual,
    fetchIssue: vi.fn(),
    fetchRevision: vi.fn(),
    progressIssue: vi.fn(),
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

function renderIssueDetail(issueID: string) {
  return render(
    <SWRConfig
      value={{
        provider: () => new Map(),
        dedupingInterval: 0,
      }}
    >
      <IssueDetailPage issueID={issueID} />
    </SWRConfig>
  );
}

describe("IssueDetailPage", () => {
  const originalClipboard = Object.getOwnPropertyDescriptor(
    globalThis.navigator,
    "clipboard"
  );
  const writeTextMock = vi.fn<() => Promise<void>>();

  beforeEach(() => {
    vi.mocked(useParams).mockReturnValue({});
    vi.mocked(fetchIssue).mockReset();
    vi.mocked(fetchRevision).mockReset();
    vi.mocked(progressIssue).mockReset();
    vi.mocked(refineIssue).mockReset();
    vi.mocked(closeIssue).mockReset();
    vi.mocked(reopenIssue).mockReset();
    vi.mocked(fetchMapData).mockReset();
    writeTextMock.mockReset();
    writeTextMock.mockResolvedValue();
    vi.mocked(fetchRevision).mockResolvedValue(1);

    Object.defineProperty(globalThis.navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: writeTextMock,
      },
    });

    vi.mocked(fetchMapData).mockResolvedValue({
      issues: [],
      edges: [],
      clusters: [],
    });
  });

  afterEach(() => {
    if (originalClipboard) {
      Object.defineProperty(globalThis.navigator, "clipboard", originalClipboard);
      return;
    }

    Reflect.deleteProperty(globalThis.navigator, "clipboard");
  });

  it("renders the canonical summary separately from the discussion feed", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(makeIssue());

    renderIssueDetail("issue-123");

    expect(await screen.findByText("Canonical summary")).toBeInTheDocument();
    expect(
      screen.getByText("Discussion", {
        selector: "h3",
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

  it("falls back to the route param when the prop issue ID is undefined", async () => {
    vi.mocked(useParams).mockReturnValue({ id: "issue-route-fallback" });
    vi.mocked(fetchIssue).mockResolvedValue(makeIssue({ id: "issue-route-fallback" }));

    renderIssueDetail(undefined as unknown as string);

    await waitFor(() => {
      expect(vi.mocked(fetchIssue)).toHaveBeenCalledWith("issue-route-fallback");
    });
  });

  it("renders related issues and operation history when relationship data is present", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(
      makeIssue({
        links: [
          {
            id: "issue-link-1",
            type: "parent_of",
            sourceIssueId: "issue-123",
            targetIssueId: "issue-456",
            direction: "outgoing",
            createdBy: "Casey",
            createdAt: new Date("2026-03-08T12:15:00Z").toISOString(),
            note: "Split this into a concrete child issue.",
            operationId: "issue-op-000001",
            relatedIssue: {
              id: "issue-456",
              raw: "Add onboarding checklist",
              status: "open",
            },
          },
        ],
        operations: [
          {
            id: "issue-op-000001",
            kind: "split",
            createdBy: "Casey",
            createdAt: new Date("2026-03-08T12:15:00Z").toISOString(),
            note: "Split this into a concrete child issue.",
            participants: [
              {
                issueId: "issue-123",
                role: "source",
                issue: {
                  id: "issue-123",
                  raw: "Export fails in Safari after tapping share twice.",
                  status: "open",
                },
              },
              {
                issueId: "issue-456",
                role: "child:1",
                issue: {
                  id: "issue-456",
                  raw: "Add onboarding checklist",
                  status: "open",
                },
              },
            ],
          },
        ],
      })
    );

    renderIssueDetail("issue-123");

    expect(await screen.findByText("Related issues")).toBeInTheDocument();
    expect(screen.getByText("Parent of")).toBeInTheDocument();
    expect(screen.getByText("Add onboarding checklist")).toBeInTheDocument();
    expect(screen.getByText("Operation history")).toBeInTheDocument();
    expect(screen.getByText("Split")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /issue-456.*child:1/i })
    ).toBeInTheDocument();
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

    renderIssueDetail("issue-123");

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

  it("renders progress posts with distinct label", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(
      makeIssue({
        id: "issue-progress-label",
        discussion: [
          {
            id: "issue-pl-post-000001",
            issueId: "issue-progress-label",
            raw: "Export fails on iPad.",
            createdBy: "Casey",
            createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
            sequence: 1,
          },
          {
            id: "issue-pl-post-000002",
            issueId: "issue-progress-label",
            raw: "Started investigating the share handler.",
            createdBy: "Jordan",
            createdAt: new Date("2026-03-08T12:10:00Z").toISOString(),
            sequence: 2,
            kind: "progress",
          },
        ],
      })
    );

    renderIssueDetail("issue-progress-label");

    expect(await screen.findByText("Progress update")).toBeInTheDocument();
    expect(screen.queryByText("Refinement 1")).not.toBeInTheDocument();
  });

  it("posts a progress update", async () => {
    const initialIssue = makeIssue({ id: "issue-progress-post" });
    const updatedIssue = makeIssue({
      id: "issue-progress-post",
      discussion: [
        ...(initialIssue.discussion ?? []),
        {
          id: "issue-pp-post-000003",
          issueId: "issue-progress-post",
          raw: "Fixed the share handler crash.",
          createdBy: "You",
          createdAt: new Date("2026-03-08T12:20:00Z").toISOString(),
          sequence: 3,
          kind: "progress",
        },
      ],
    });

    vi.mocked(fetchIssue).mockResolvedValue(initialIssue);
    vi.mocked(progressIssue).mockResolvedValue(updatedIssue);

    renderIssueDetail("issue-progress-post");

    await screen.findByText("Canonical summary");

    await userEvent.click(screen.getByRole("button", { name: "Progress" }));

    await userEvent.type(
      screen.getByPlaceholderText("Report work done toward resolving this issue..."),
      "Fixed the share handler crash."
    );
    await userEvent.click(screen.getByRole("button", { name: "Post progress" }));

    await waitFor(() => {
      expect(progressIssue).toHaveBeenCalledWith("issue-progress-post", {
        raw: "Fixed the share handler crash.",
      });
    });

    expect(refineIssue).not.toHaveBeenCalled();
  });

  it("copies the canonical summary to the clipboard", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(
      makeIssue({
        id: "issue-copy-summary",
        raw: "Copy this canonical summary exactly.",
      })
    );

    renderIssueDetail("issue-copy-summary");

    await screen.findByText("Canonical summary");

    await userEvent.click(screen.getByRole("button", { name: "Copy summary" }));

    await waitFor(() => {
      expect(writeTextMock).toHaveBeenCalledWith("Copy this canonical summary exactly.");
    });

    expect(
      screen.getByRole("button", { name: "Copied summary" })
    ).toBeInTheDocument();
  });

  it("copies the current issue link to the clipboard", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(
      makeIssue({
        id: "issue-copy-link",
      })
    );
    window.history.replaceState({}, "", "/issues/issue-copy-link");

    renderIssueDetail("issue-copy-link");

    await screen.findByText("Canonical summary");

    await userEvent.click(screen.getByRole("button", { name: "Copy link" }));

    await waitFor(() => {
      expect(writeTextMock).toHaveBeenCalledWith(
        "http://localhost:3000/issues/issue-copy-link"
      );
    });

    expect(screen.getByRole("button", { name: "Copied link" })).toBeInTheDocument();
  });

  it("refinement numbering skips progress posts", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(
      makeIssue({
        id: "issue-numbering",
        discussion: [
          {
            id: "issue-num-post-000001",
            issueId: "issue-numbering",
            raw: "Export fails on iPad.",
            createdBy: "Casey",
            createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
            sequence: 1,
          },
          {
            id: "issue-num-post-000002",
            issueId: "issue-numbering",
            raw: "It only happens in Safari.",
            createdBy: "Jordan",
            createdAt: new Date("2026-03-08T12:10:00Z").toISOString(),
            sequence: 2,
            kind: "refinement",
          },
          {
            id: "issue-num-post-000003",
            issueId: "issue-numbering",
            raw: "Started investigating.",
            createdBy: "Jordan",
            createdAt: new Date("2026-03-08T12:15:00Z").toISOString(),
            sequence: 3,
            kind: "progress",
          },
          {
            id: "issue-num-post-000004",
            issueId: "issue-numbering",
            raw: "Also affects macOS Safari.",
            createdBy: "Casey",
            createdAt: new Date("2026-03-08T12:20:00Z").toISOString(),
            sequence: 4,
            kind: "refinement",
          },
        ],
      })
    );

    renderIssueDetail("issue-numbering");

    expect(await screen.findByText("Initial report")).toBeInTheDocument();
    expect(screen.getByText("Refinement 1")).toBeInTheDocument();
    expect(screen.getByText("Progress update")).toBeInTheDocument();
    expect(screen.getByText("Refinement 2")).toBeInTheDocument();
  });

  it("promotes related context in the side rail", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(makeIssue({ id: "issue-related-context" }));
    vi.mocked(fetchMapData).mockResolvedValue({
      issues: [
        {
          id: "issue-related-context",
          raw: "Current issue",
          status: "open",
          tags: [{ tag: "export", relevance: 0.9 }],
          x: 0.5,
          y: 0.5,
        },
        {
          id: "issue-open-neighbor",
          raw: "Open semantic neighbor",
          status: "open",
          tags: [{ tag: "export", relevance: 0.8 }],
          x: 0.58,
          y: 0.53,
        },
        {
          id: "issue-cluster-peer",
          raw: "Open cluster peer",
          status: "open",
          tags: [{ tag: "export", relevance: 0.72 }],
          x: 0.46,
          y: 0.57,
        },
      ],
      edges: [
        {
          source: "issue-related-context",
          target: "issue-open-neighbor",
          similarity: 0.81,
        },
      ],
      clusters: [
        {
          label: "Export cluster",
          centerX: 0.54,
          centerY: 0.52,
          radius: 0.15,
          color: "#2563eb",
        },
      ],
    });

    renderIssueDetail("issue-related-context");

    expect(await screen.findByText("Related context")).toBeInTheDocument();
    expect(screen.getByText("Issue neighborhood")).toBeInTheDocument();
    expect(screen.getByText("Semantic neighbors")).toBeInTheDocument();
    expect(screen.getByText("Same cluster")).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: "Issue neighborhood for issue-related-context" })
    ).toHaveAttribute("width", "100%");
    expect(
      screen.getByRole("img", { name: "Issue neighborhood for issue-related-context" })
    ).toHaveAttribute("height", "100%");
    expect(
      screen.getByRole("img", { name: "Issue neighborhood for issue-related-context" })
    ).toHaveAttribute("viewBox", "0 0 640 360");
  });

  it("sorts semantic and clustered neighbors with open issues first", async () => {
    vi.mocked(fetchIssue).mockResolvedValue(makeIssue({ id: "issue-sorting" }));
    vi.mocked(fetchMapData).mockResolvedValue({
      issues: [
        {
          id: "issue-sorting",
          raw: "Current issue",
          status: "open",
          tags: [{ tag: "export", relevance: 0.9 }],
          x: 0.5,
          y: 0.5,
        },
        {
          id: "issue-closed-neighbor",
          raw: "Closed semantic neighbor",
          status: "closed",
          tags: [{ tag: "export", relevance: 0.88 }],
          x: 0.55,
          y: 0.51,
        },
        {
          id: "issue-open-neighbor",
          raw: "Open semantic neighbor",
          status: "open",
          tags: [{ tag: "export", relevance: 0.82 }],
          x: 0.6,
          y: 0.52,
        },
      ],
      edges: [
        {
          source: "issue-sorting",
          target: "issue-closed-neighbor",
          similarity: 0.98,
        },
        {
          source: "issue-sorting",
          target: "issue-open-neighbor",
          similarity: 0.8,
        },
      ],
      clusters: [
        {
          label: "Export cluster",
          centerX: 0.55,
          centerY: 0.51,
          radius: 0.16,
          color: "#2563eb",
        },
      ],
    });

    renderIssueDetail("issue-sorting");

    await screen.findByText("Semantic neighbors");

    const openNeighbor = screen.getAllByText("Open semantic neighbor")[0];
    const closedNeighbor = screen.getAllByText("Closed semantic neighbor")[0];

    expect(
      openNeighbor.compareDocumentPosition(closedNeighbor) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });
});
