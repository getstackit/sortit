import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { PersonDetailPage } from "@/components/person-detail-page";
import { usePersonDetail } from "@/hooks/use-people";

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

vi.mock("@/components/tag-relevance-bars", () => ({
  TagRelevanceBars: () => <div>Tag bars</div>,
}));

vi.mock("@/hooks/use-people", () => ({
  usePersonDetail: vi.fn(),
}));

describe("PersonDetailPage", () => {
  beforeEach(() => {
    vi.mocked(usePersonDetail).mockReset();
  });

  it("renders the next issue and recommendations from the backend detail payload", () => {
    vi.mocked(usePersonDetail).mockReturnValue({
      data: {
        person: "Avery",
        issueCount: 2,
        openIssueCount: 1,
        closedIssueCount: 1,
        tagProfile: [{ tag: "auth", relevance: 0.88 }],
        assignedIssues: [
          {
            id: "issue-assigned",
            raw: "Fix auth redirect loop",
            tags: ["auth"],
            tagScores: [{ tag: "auth", relevance: 0.88 }],
            createdBy: "Casey",
            createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
            status: "open",
            assignedTo: "Avery",
          },
        ],
        nextIssue: {
          issue: {
            id: "issue-assigned",
            raw: "Fix auth redirect loop",
            tags: ["auth"],
            tagScores: [{ tag: "auth", relevance: 0.88 }],
            createdBy: "Casey",
            createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
            status: "open",
            assignedTo: "Avery",
          },
          source: "assigned",
          reason: "Oldest open issue already assigned to this person",
          combinedScore: 1,
          semanticScore: 1,
          factorScore: 1,
          sharedTags: ["auth"],
        },
        recommendedIssues: [
          {
            issue: {
              id: "issue-recommended",
              raw: "Invited users land on a blank auth handoff screen",
              tags: ["auth", "onboarding"],
              tagScores: [
                { tag: "auth", relevance: 0.83 },
                { tag: "onboarding", relevance: 0.62 },
              ],
              createdBy: "Jordan",
              createdAt: new Date("2026-03-09T12:00:00Z").toISOString(),
              status: "open",
            },
            source: "recommended",
            reason: "Matches this person's historical focus in auth, onboarding",
            combinedScore: 0.84,
            semanticScore: 0.76,
            factorScore: 0.88,
            sharedTags: ["auth", "onboarding"],
          },
        ],
      },
      error: undefined,
      isLoading: false,
    } as ReturnType<typeof usePersonDetail>);

    render(<PersonDetailPage person="Avery" />);

    expect(screen.getByText("Current Queue Head")).toBeInTheDocument();
    expect(screen.getAllByText("Fix auth redirect loop").length).toBeGreaterThan(0);
    expect(screen.getByText("Open Recommendations")).toBeInTheDocument();
    expect(screen.getByText("Invited users land on a blank auth handoff screen")).toBeInTheDocument();
    expect(screen.getByText("84% match")).toBeInTheDocument();
    expect(screen.getAllByText("auth").length).toBeGreaterThan(0);
  });
});
