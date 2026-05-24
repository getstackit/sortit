import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { TagDetailPage } from "@/components/tag-detail-page";
import { useIssues } from "@/hooks/use-issues";
import {
  clearRecentHistory,
  readRecentHistory,
} from "@/hooks/use-recent-history";
import { useTags } from "@/hooks/use-tags";
import type { IssueRecord } from "@/lib/issues";
import type { TagRecord } from "@/lib/tags";

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
  }: {
    title: string;
    subtitle?: string;
    meta?: ReactNode;
  }) => (
    <header>
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
      {meta}
    </header>
  ),
}));

vi.mock("@/components/issue-card", () => ({
  IssueCard: ({ issue }: { issue: IssueRecord }) => <div>{issue.raw}</div>,
}));

vi.mock("@/components/ui/skeleton", () => ({
  Skeleton: () => <div>Loading</div>,
}));

vi.mock("@/hooks/use-tags", () => ({
  useTags: vi.fn(),
}));

vi.mock("@/hooks/use-issues", () => ({
  useIssues: vi.fn(),
}));

function makeTag(overrides: Partial<TagRecord>): TagRecord {
  return {
    name: "billing",
    description: "Recurring payments and invoices",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    embedding: [1, 0],
    ...overrides,
  };
}

function makeIssue(overrides: Partial<IssueRecord>): IssueRecord {
  return {
    id: "issue-1",
    raw: "Billing issue",
    tags: ["billing"],
    tagScores: [{ tag: "billing", relevance: 0.9 }],
    createdBy: "Casey",
    createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
    status: "open",
    ...overrides,
  };
}

describe("TagDetailPage", () => {
  beforeEach(() => {
    vi.mocked(useTags).mockReset();
    vi.mocked(useIssues).mockReset();
    clearRecentHistory();
  });

  it("renders associated issues, related tags, and semantic neighbors", () => {
    vi.mocked(useTags).mockReturnValue({
      data: [
        makeTag({ name: "billing" }),
        makeTag({
          name: "payments",
          description: "Captures and refunds",
          embedding: [0.92, 0.08],
        }),
        makeTag({
          name: "invoices",
          description: "Invoice lifecycle",
          embedding: [0.84, 0.16],
        }),
      ],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useTags>);

    vi.mocked(useIssues).mockReturnValue({
      data: [
        makeIssue({
          id: "issue-1",
          raw: "Billing export fails for enterprise invoices",
          tags: ["billing", "payments"],
          tagScores: [
            { tag: "billing", relevance: 0.9 },
            { tag: "payments", relevance: 0.7 },
          ],
        }),
        makeIssue({
          id: "issue-2",
          raw: "Invoice retries should respect billing schedules",
          tags: ["billing", "invoices"],
          tagScores: [
            { tag: "billing", relevance: 0.8 },
            { tag: "invoices", relevance: 0.75 },
          ],
          status: "closed",
        }),
      ],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<TagDetailPage tagName="billing" />);

    expect(screen.getByRole("heading", { name: "billing" })).toBeInTheDocument();
    expect(screen.getByText("Billing export fails for enterprise invoices")).toBeInTheDocument();
    expect(screen.getByText("Invoice retries should respect billing schedules")).toBeInTheDocument();
    expect(screen.getAllByText("payments").length).toBeGreaterThan(0);
    expect(screen.getAllByText("invoices").length).toBeGreaterThan(0);
    expect(screen.getByText("Captures and refunds")).toBeInTheDocument();
  });

  it("renders a not-found state when the tag does not exist", () => {
    vi.mocked(useTags).mockReturnValue({
      data: [makeTag({ name: "payments" })],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useTags>);

    vi.mocked(useIssues).mockReturnValue({
      data: [],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<TagDetailPage tagName="billing" />);

    expect(screen.getByText(/No tag exists for/i)).toBeInTheDocument();
  });

  it("records the viewed tag in recent history", () => {
    vi.mocked(useTags).mockReturnValue({
      data: [makeTag({ name: "billing" })],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useTags>);

    vi.mocked(useIssues).mockReturnValue({
      data: [],
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useIssues>);

    render(<TagDetailPage tagName="billing" />);

    expect(readRecentHistory()).toEqual([
      expect.objectContaining({
        kind: "tag",
        name: "billing",
        description: "Recurring payments and invoices",
      }),
    ]);
  });
});
