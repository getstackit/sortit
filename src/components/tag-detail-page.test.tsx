import type { ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { TagDetailPage } from "@/components/tag-detail-page";
import { useIssues } from "@/hooks/use-issues";
import { useRegion } from "@/hooks/use-region";
import {
  clearRecentHistory,
  readRecentHistory,
} from "@/hooks/use-recent-history";
import { useTags } from "@/hooks/use-tags";
import type { IssueRecord } from "@/lib/issues";
import type { TagRecord } from "@/lib/tags";

const searchParamsBag = { value: new URLSearchParams() };
const routerReplace = vi.fn();

vi.mock("next/navigation", () => ({
  usePathname: () => "/tags/billing",
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

vi.mock("@/hooks/use-region", () => ({
  useRegion: vi.fn(() => ({ data: undefined, error: undefined, isLoading: true })),
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
    vi.mocked(useRegion).mockReset();
    vi.mocked(useRegion).mockReturnValue({
      data: undefined,
      error: undefined,
      isLoading: true,
    } as ReturnType<typeof useRegion>);
    searchParamsBag.value = new URLSearchParams();
    routerReplace.mockReset();
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

  it("renders region metrics with mass and age buckets when available", () => {
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
    vi.mocked(useRegion).mockReturnValue({
      data: {
        region: { key: { kind: "tag", id: "billing" }, label: "billing" },
        metrics: {
          key: { kind: "tag", id: "billing" },
          window: { label: "30d", start: "2026-04-25T12:00:00Z", end: "2026-05-25T12:00:00Z" },
          mass: 5,
          massOpen: 3,
          massClosed: 2,
          ageBuckets: [
            { label: "<1w", count: 1 },
            { label: "1-4w", count: 2 },
            { label: "1-3m", count: 0 },
            { label: "3m+", count: 0 },
          ],
          growth: { count: 12, perDay: 0.4 },
          closure: { count: 5, perDay: 0.17 },
        },
        window: { label: "30d", start: "2026-04-25T12:00:00Z", end: "2026-05-25T12:00:00Z" },
      },
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useRegion>);

    render(<TagDetailPage tagName="billing" />);

    expect(screen.getByText("Region metrics")).toBeInTheDocument();
    expect(screen.getByText("Open at floor")).toBeInTheDocument();
    expect(screen.getByText("Closed at floor")).toBeInTheDocument();
    expect(screen.getByText("Open issues by age")).toBeInTheDocument();
    expect(screen.getByText("<1w")).toBeInTheDocument();
    expect(screen.getByText("3m+")).toBeInTheDocument();
    expect(screen.getByText("Growth (30d)")).toBeInTheDocument();
    expect(screen.getByText("Closure (30d)")).toBeInTheDocument();
    expect(screen.getByText("0.40/day")).toBeInTheDocument();
    expect(screen.getByText("0.17/day")).toBeInTheDocument();
  });

  it("updates the URL when the region window selector changes", () => {
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
    vi.mocked(useRegion).mockReturnValue({
      data: {
        region: { key: { kind: "tag", id: "billing" }, label: "billing" },
        metrics: {
          key: { kind: "tag", id: "billing" },
          window: { label: "30d", start: "", end: "" },
          mass: 1,
          massOpen: 1,
          massClosed: 0,
        },
        window: { label: "30d", start: "", end: "" },
      },
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof useRegion>);

    render(<TagDetailPage tagName="billing" />);

    const sevenDay = screen.getAllByRole("button").find((el) => el.textContent === "7d");
    if (!sevenDay) throw new Error("7d toggle not found");
    fireEvent.click(sevenDay);

    expect(routerReplace).toHaveBeenCalled();
    const lastCall = routerReplace.mock.calls.at(-1);
    expect(lastCall?.[0]).toContain("regionWindow=7d");
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
