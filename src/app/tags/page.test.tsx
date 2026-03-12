import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import TagsPage from "@/app/tags/page";
import { useIssues } from "@/hooks/use-issues";
import { fetchTags, type TagRecord } from "@/lib/tags";

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
  }: {
    title: string;
    subtitle?: string;
  }) => (
    <header>
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
    </header>
  ),
}));

vi.mock("@/components/ui/button", () => ({
  Button: ({
    children,
    onClick,
  }: {
    children: ReactNode;
    onClick?: () => void;
  }) => (
    <button type="button" onClick={onClick}>
      {children}
    </button>
  ),
}));

vi.mock("@/components/ui/skeleton", () => ({
  Skeleton: () => <div>Loading</div>,
}));

vi.mock("@/hooks/use-issues", () => ({
  useIssues: vi.fn(),
}));

vi.mock("@/lib/tags", async () => {
  const actual = await vi.importActual<typeof import("@/lib/tags")>("@/lib/tags");
  return {
    ...actual,
    fetchTags: vi.fn(),
  };
});

function makeTag(overrides: Partial<TagRecord>): TagRecord {
  return {
    name: "tag",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    embedding: [1, 0],
    ...overrides,
  };
}

describe("TagsPage", () => {
  beforeEach(() => {
    vi.mocked(fetchTags).mockReset();
    vi.mocked(useIssues).mockReturnValue({
      data: [],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);
  });

  it("defaults the inspector to the first embedded tag and avoids key warnings", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    vi.mocked(fetchTags).mockResolvedValue([
      makeTag({ name: "draft", embedding: [] }),
      makeTag({ name: "alpha", description: "Primary tag", embedding: [1, 0] }),
      makeTag({ name: "beta", description: "Neighbor tag", embedding: [0.8, 0.2] }),
    ]);

    render(<TagsPage />);

    expect(await screen.findByText("Nearest neighbors")).toBeInTheDocument();
    expect(
      screen.queryByText("Select a tag to inspect its neighborhood.")
    ).not.toBeInTheDocument();
    expect(screen.getByText("Primary tag")).toBeInTheDocument();
    expect(screen.getByText("Neighbor tag")).toBeInTheDocument();

    await waitFor(() => {
      const hasKeyWarning = consoleError.mock.calls.some((args) =>
        args.some(
          (arg) =>
            typeof arg === "string" &&
            arg.includes('Each child in a list should have a unique "key" prop')
        )
      );

      expect(hasKeyWarning).toBe(false);
    });

    consoleError.mockRestore();
  });

  it("shows specificity and merge insights for generic bucket tags", async () => {
    vi.mocked(fetchTags).mockResolvedValue([
      makeTag({ name: "backend", description: "Generic backend surface", embedding: [1, 0] }),
      makeTag({ name: "billing", description: "Invoices and account charges", embedding: [0.98, 0.02] }),
      makeTag({ name: "payments", description: "Payment processing", embedding: [0.97, 0.03] }),
      makeTag({ name: "back end", description: "Spacing variant", embedding: [1, 0] }),
    ]);
    vi.mocked(useIssues).mockReturnValue({
      data: [
        {
          id: "issue-1",
          raw: "Billing bug",
          tags: ["backend", "billing"],
          tagScores: [
            { tag: "backend", relevance: 0.9 },
            { tag: "billing", relevance: 0.8 },
          ],
          createdBy: "Jonathan Goldman",
          createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
          status: "open",
        },
      ],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<TagsPage />);

    expect(await screen.findByText("Specificity")).toBeInTheDocument();
    expect(screen.getByText("Generic bucket")).toBeInTheDocument();
    expect(screen.getAllByText("Invoices and account charges").length).toBeGreaterThan(0);
    expect(screen.getByText("Potential merges")).toBeInTheDocument();
    expect(screen.getAllByText("Name variant").length).toBeGreaterThan(0);
  });

  it("shows global consolidation candidates on the tag map", async () => {
    vi.mocked(fetchTags).mockResolvedValue([
      makeTag({
        name: "frontend",
        description: "Client-side runtime behavior",
        createdAt: new Date("2026-03-07T12:00:00Z").toISOString(),
        embedding: [1, 0],
      }),
      makeTag({
        name: "front end",
        description: "Spacing variant",
        createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
        embedding: [1, 0],
      }),
      makeTag({
        name: "billing",
        description: "Invoices and account charges",
        embedding: [0, 1],
      }),
    ]);
    vi.mocked(useIssues).mockReturnValue({
      data: [
        {
          id: "issue-1",
          raw: "Client issue",
          tags: ["frontend", "front end"],
          tagScores: [
            { tag: "frontend", relevance: 0.96 },
            { tag: "front end", relevance: 0.88 },
          ],
          createdBy: "Jonathan Goldman",
          createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
          status: "open",
        },
        {
          id: "issue-2",
          raw: "Another client issue",
          tags: ["frontend", "front end"],
          tagScores: [
            { tag: "frontend", relevance: 0.91 },
            { tag: "front end", relevance: 0.84 },
          ],
          createdBy: "Jonathan Goldman",
          createdAt: new Date("2026-03-09T12:00:00Z").toISOString(),
          status: "open",
        },
      ],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<TagsPage />);

    expect(await screen.findByText("Consolidation")).toBeInTheDocument();
    expect(screen.getByText(/Consolidate/i)).toBeInTheDocument();
    expect(screen.getByText("Name variant with overlapping issue coverage")).toBeInTheDocument();
    expect(screen.getByText("2 shared issues")).toBeInTheDocument();
  });
});
