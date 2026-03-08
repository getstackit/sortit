import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import TagsPage from "@/app/tags/page";
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
});
