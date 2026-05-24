import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CommandPalette } from "@/components/command-palette";
import {
  clearRecentHistory,
  rememberRecentIssue,
  rememberRecentTag,
} from "@/hooks/use-recent-history";
import { useUnifiedSearch } from "@/hooks/use-search";

const push = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));

vi.mock("@base-ui/react/dialog", () => ({
  Dialog: {
    Root: ({
      children,
    }: {
      open: boolean;
      onOpenChange: (open: boolean) => void;
      children: ReactNode;
    }) => <div>{children}</div>,
    Portal: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    Backdrop: ({ className }: { className?: string }) => <div className={className} />,
    Viewport: ({ children, className }: { children: ReactNode; className?: string }) => (
      <div className={className}>{children}</div>
    ),
    Popup: ({ children, className }: { children: ReactNode; className?: string }) => (
      <div className={className}>{children}</div>
    ),
    Title: ({ children, className }: { children: ReactNode; className?: string }) => (
      <span className={className}>{children}</span>
    ),
    Description: ({ children, className }: { children: ReactNode; className?: string }) => (
      <span className={className}>{children}</span>
    ),
    Close: ({ children, className, render }: { children?: ReactNode; className?: string; render?: ReactNode }) => (
      <button className={className}>{render}{children}</button>
    ),
    Trigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  },
}));

vi.mock("@/hooks/use-search", () => ({
  UNIFIED_SEARCH_LIMIT: 8,
  useUnifiedSearch: vi.fn(),
}));

describe("CommandPalette", () => {
  beforeEach(() => {
    push.mockReset();
    clearRecentHistory();
    Element.prototype.scrollIntoView = vi.fn();
    vi.mocked(useUnifiedSearch).mockReturnValue({
      data: {
        query: {
          raw: "billing",
          tags: [],
        },
        issues: [],
        relatedTags: [
          {
            name: "billing",
            description: "Recurring payments and invoices",
            similarity: 0.94,
          },
        ],
      },
      isLoading: false,
    } as ReturnType<typeof useUnifiedSearch>);
  });

  it("routes tag results to the dedicated tag page", async () => {
    const user = userEvent.setup();

    render(<CommandPalette open onOpenChange={() => {}} />);

    const input = screen.getByPlaceholderText("Search issues and tags...");
    await user.type(input, "billing");
    await user.click(await screen.findByRole("option", { name: /billing/i }));

    expect(push).toHaveBeenCalledWith("/tags/billing");
  });

  it("navigates straight to an issue when a full issue id is pasted", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(<CommandPalette open onOpenChange={onOpenChange} />);

    const input = screen.getByPlaceholderText("Search issues and tags...");
    await user.click(input);
    await user.paste("01kkbqe0f7xz0s82nqbbg2fheh");

    expect(push).toHaveBeenCalledWith("/issues/01KKBQE0F7XZ0S82NQBBG2FHEH");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("shows recently viewed issues and tags when opened with no query", async () => {
    const user = userEvent.setup();

    rememberRecentIssue({
      id: "issue-123",
      raw: "Recent issue from detail page",
      status: "open",
      tags: ["frontend", "ux"],
    });
    rememberRecentTag({
      name: "billing",
      description: "Recurring payments and invoices",
    });

    render(<CommandPalette open onOpenChange={() => {}} />);

    expect(screen.getByText("Recently viewed")).toBeInTheDocument();
    expect(screen.getByText("Recent issue from detail page")).toBeInTheDocument();
    expect(screen.getAllByText("billing").length).toBeGreaterThan(0);

    await user.click(screen.getByRole("option", { name: /recent issue from detail page/i }));

    expect(push).toHaveBeenCalledWith("/issues/issue-123");
  });

  it("discloses that the palette is a truncated quick switcher", async () => {
    const user = userEvent.setup();

    render(<CommandPalette open onOpenChange={() => {}} />);

    await user.type(screen.getByPlaceholderText("Search issues and tags..."), "billing");

    expect(
      screen.getByText("Quick switcher showing up to 8 matches.")
    ).toBeInTheDocument();
  });

  it("keeps an explanatory empty state before anything has been viewed", () => {
    render(<CommandPalette open onOpenChange={() => {}} />);

    expect(screen.getByText("Search issues and tags...")).toBeInTheDocument();
    expect(
      screen.getByText("Recently viewed issues and tags will show up here.")
    ).toBeInTheDocument();
  });
});
