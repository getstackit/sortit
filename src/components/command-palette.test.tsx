import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CommandPalette } from "@/components/command-palette";
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
  },
}));

vi.mock("@/hooks/use-search", () => ({
  useUnifiedSearch: vi.fn(),
}));

describe("CommandPalette", () => {
  beforeEach(() => {
    push.mockReset();
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
    await user.click(await screen.findByRole("button", { name: /billing/i }));

    expect(push).toHaveBeenCalledWith("/tags/billing");
  });
});
