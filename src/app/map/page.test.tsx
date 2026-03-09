import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import MapPage from "@/app/map/page";
import type { MapData } from "@/features/map/types";

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
  usePathname: () => "/map",
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/components/app-shell", () => ({
  AppShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/app-sidebar", () => ({
  AppSidebar: () => <div>Sidebar</div>,
}));

vi.mock("@/components/site-header", () => ({
  SiteHeader: ({ title }: { title: string }) => <header><h1>{title}</h1></header>,
}));

vi.mock("@/components/tag-relevance-bars", () => ({
  TagRelevanceBars: () => <div>TagBars</div>,
}));

vi.mock("@/components/ui/switch", () => ({
  Switch: (props: { checked: boolean; onCheckedChange: (v: boolean) => void }) => (
    <input
      type="checkbox"
      checked={props.checked}
      onChange={(e) => props.onCheckedChange(e.target.checked)}
    />
  ),
}));

const mockFetchMapData = vi.fn<() => Promise<MapData>>();
const mockFetchViewportEdges = vi.fn();
const mockCompareIssueEmbeddings = vi.fn();

vi.mock("@/features/map/api", () => ({
  fetchMapData: (...args: unknown[]) => mockFetchMapData(...args as []),
  fetchViewportEdges: (...args: unknown[]) => mockFetchViewportEdges(...args as []),
  compareIssueEmbeddings: (...args: unknown[]) => mockCompareIssueEmbeddings(...args as []),
}));

function makeMapData(overrides?: Partial<MapData>): MapData {
  return {
    issues: [
      { id: "issue-001", raw: "First issue", status: "open", x: 0.1, y: 0.1, tags: [{ tag: "bug", relevance: 0.9 }] },
      { id: "issue-002", raw: "Second issue", status: "open", x: 0.2, y: 0.2, tags: [{ tag: "feature", relevance: 0.8 }] },
      { id: "issue-003", raw: "Third issue", status: "open", x: 0.3, y: 0.3, tags: [{ tag: "bug", relevance: 0.7 }] },
      { id: "issue-004", raw: "Fourth issue", status: "open", x: 0.15, y: 0.15, tags: [{ tag: "bug", relevance: 0.6 }] },
      { id: "issue-005", raw: "Fifth issue", status: "open", x: 0.12, y: 0.12, tags: [{ tag: "bug", relevance: 0.5 }] },
      { id: "issue-006", raw: "Sixth issue", status: "open", x: 0.25, y: 0.25, tags: [{ tag: "feature", relevance: 0.7 }] },
      { id: "issue-007", raw: "Seventh issue", status: "open", x: 0.8, y: 0.8, tags: [{ tag: "ui", relevance: 0.9 }] },
      { id: "issue-008", raw: "Eighth issue", status: "open", x: 0.82, y: 0.82, tags: [{ tag: "ui", relevance: 0.8 }] },
      { id: "issue-009", raw: "Ninth issue", status: "open", x: 0.85, y: 0.85, tags: [{ tag: "ui", relevance: 0.7 }] },
      { id: "issue-010", raw: "Tenth issue", status: "open", x: 0.83, y: 0.81, tags: [{ tag: "ui", relevance: 0.6 }] },
      { id: "issue-011", raw: "Eleventh issue", status: "open", x: 0.84, y: 0.86, tags: [{ tag: "ui", relevance: 0.5 }] },
    ],
    edges: [],
    clusters: [
      {
        label: "Bug / Feature",
        centerX: 0.2,
        centerY: 0.2,
        radius: 0.1,
        color: "#ef4444",
        issueIds: ["issue-001", "issue-002", "issue-003", "issue-004", "issue-005", "issue-006"],
        topTag: "bug",
      },
      {
        label: "Ui",
        centerX: 0.83,
        centerY: 0.83,
        radius: 0.05,
        color: "#3b82f6",
        issueIds: ["issue-007", "issue-008", "issue-009", "issue-010", "issue-011"],
        topTag: "ui",
      },
    ],
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockFetchMapData.mockResolvedValue(makeMapData());
  mockFetchViewportEdges.mockResolvedValue({ edges: [] });

  Object.defineProperty(HTMLElement.prototype, "getBoundingClientRect", {
    configurable: true,
    value: () => ({
      width: 800,
      height: 600,
      top: 0,
      left: 0,
      bottom: 600,
      right: 800,
      x: 0,
      y: 0,
      toJSON: () => {},
    }),
  });

  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
});

describe("MapPage", () => {
  it("renders without crashing after data loads", async () => {
    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });
  });

  it("renders the SVG canvas with blob filter", async () => {
    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const filter = container.querySelector("#blob-soft");
    expect(filter).toBeInTheDocument();
  });

  it("renders blob paths for clusters with 5+ items", async () => {
    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const paths = container.querySelectorAll("path[filter='url(#blob-soft)']");
    expect(paths.length).toBe(2);
  });

  it("renders blob labels", async () => {
    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const svg = container.querySelector("svg")!;
    const texts = svg.querySelectorAll("text");
    const labels = Array.from(texts).map((t) => t.textContent);
    expect(labels).toContain("Bug / Feature");
    expect(labels).toContain("Ui");
  });

  it("does not render blobs for clusters with fewer than 5 items", async () => {
    const data = makeMapData({
      clusters: [
        {
          label: "Small",
          centerX: 0.5,
          centerY: 0.5,
          radius: 0.1,
          color: "#ef4444",
          issueIds: ["issue-001", "issue-002", "issue-003"],
          topTag: "bug",
        },
      ],
    });
    mockFetchMapData.mockResolvedValue(data);

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const paths = container.querySelectorAll("path[filter='url(#blob-soft)']");
    expect(paths.length).toBe(0);
  });

  it("handles clusters with null issueIds without crashing", async () => {
    const data = makeMapData({
      clusters: [
        {
          label: "Null IDs",
          centerX: 0.5,
          centerY: 0.5,
          radius: 0.1,
          color: "#ef4444",
          issueIds: null as unknown as string[],
          topTag: "bug",
        },
      ],
    });
    mockFetchMapData.mockResolvedValue(data);

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    // Should render without crashing, no blobs since issueIds is null
    const paths = container.querySelectorAll("path[filter='url(#blob-soft)']");
    expect(paths.length).toBe(0);
  });

  it("handles clusters with undefined issueIds without crashing", async () => {
    const data = makeMapData({
      clusters: [
        {
          label: "Missing IDs",
          centerX: 0.5,
          centerY: 0.5,
          radius: 0.1,
          color: "#ef4444",
          topTag: "bug",
        } as MapData["clusters"][0],
      ],
    });
    mockFetchMapData.mockResolvedValue(data);

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const paths = container.querySelectorAll("path[filter='url(#blob-soft)']");
    expect(paths.length).toBe(0);
  });

  it("handles legacy clusters without new fields (backward compat)", async () => {
    const data = makeMapData({
      clusters: [
        {
          label: "Legacy",
          centerX: 0.5,
          centerY: 0.5,
          radius: 0.1,
          color: "#ef4444",
        } as MapData["clusters"][0],
      ],
    });
    mockFetchMapData.mockResolvedValue(data);

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    // Should not crash — legacy clusters simply won't produce blobs
    const paths = container.querySelectorAll("path[filter='url(#blob-soft)']");
    expect(paths.length).toBe(0);
  });

  it("shows clear filter button when a blob is clicked", async () => {
    const user = userEvent.setup();
    const { container } = render(<MapPage />);

    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const blobGroup = container.querySelector("path[filter='url(#blob-soft)']")?.parentElement;
    expect(blobGroup).toBeInTheDocument();

    await user.click(blobGroup!);

    await waitFor(() => {
      expect(screen.getByText("Cluster members")).toBeInTheDocument();
    });
  });

  it("shows cluster sidebar details and top-tag drilldown when a blob is clicked", async () => {
    const user = userEvent.setup();
    const { container } = render(<MapPage />);

    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const blobGroup = container.querySelector("path[filter='url(#blob-soft)']")?.parentElement;
    expect(blobGroup).toBeInTheDocument();

    await user.click(blobGroup!);

    await waitFor(() => {
      expect(screen.getByText("Cluster")).toBeInTheDocument();
      expect(screen.getByText("Cluster members")).toBeInTheDocument();
      expect(screen.getByRole("link", { name: "View top tag" })).toHaveAttribute(
        "href",
        "/tags/bug"
      );
      expect(screen.getByText("First issue")).toBeInTheDocument();
    });
  });

  it("opens the issue sidebar when a cluster member is chosen from the blob sidebar", async () => {
    const user = userEvent.setup();
    const { container } = render(<MapPage />);

    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const blobGroup = container.querySelector("path[filter='url(#blob-soft)']")?.parentElement;
    expect(blobGroup).toBeInTheDocument();

    await user.click(blobGroup!);

    const clusterMember = await screen.findByRole("button", { name: /First issue/i });
    await user.click(clusterMember);

    await waitFor(() => {
      expect(screen.getByRole("link", { name: "View issue" })).toHaveAttribute(
        "href",
        "/issues/issue-001"
      );
    });
  });

  it("renders issue nodes inside the SVG", async () => {
    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const issueNodes = container.querySelectorAll("[data-issue]");
    expect(issueNodes.length).toBe(11);
  });

  it("handles empty clusters array", async () => {
    mockFetchMapData.mockResolvedValue(makeMapData({ clusters: [] }));

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const paths = container.querySelectorAll("path[filter='url(#blob-soft)']");
    expect(paths.length).toBe(0);
  });

  it("handles empty issues array", async () => {
    mockFetchMapData.mockResolvedValue(makeMapData({ issues: [], clusters: [] }));

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const issueNodes = container.querySelectorAll("[data-issue]");
    expect(issueNodes.length).toBe(0);
  });

  it("renders blobs via proximity fallback when issueIds is missing", async () => {
    // Simulate old backend format: clusters have center/radius but no issueIds
    // Issues at (0.1-0.2, 0.1-0.2) should fall within cluster radius 0.15 of center (0.15, 0.15)
    const data = makeMapData({
      clusters: [
        {
          label: "Proximity Cluster",
          centerX: 0.15,
          centerY: 0.15,
          radius: 0.15,
          color: "#ef4444",
        } as MapData["clusters"][0],
      ],
    });
    mockFetchMapData.mockResolvedValue(data);

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    // 6 issues are near (0.15, 0.15): issue-001 through issue-006
    // That's >= 5, so a blob should render
    const paths = container.querySelectorAll("path[filter='url(#blob-soft)']");
    expect(paths.length).toBe(1);
  });

  it("uses proximity fallback for filtering when issueIds is missing", async () => {
    const user = userEvent.setup();
    const data = makeMapData({
      clusters: [
        {
          label: "Proximity Cluster",
          centerX: 0.15,
          centerY: 0.15,
          radius: 0.15,
          color: "#ef4444",
        } as MapData["clusters"][0],
      ],
    });
    mockFetchMapData.mockResolvedValue(data);

    const { container } = render(<MapPage />);
    await waitFor(() => {
      expect(container.querySelector("svg")).toBeInTheDocument();
    });

    const blobGroup = container.querySelector("path[filter='url(#blob-soft)']")?.parentElement;
    expect(blobGroup).toBeInTheDocument();

    await user.click(blobGroup!);

    await waitFor(() => {
      expect(screen.getByText("Cluster members")).toBeInTheDocument();
    });
  });
});
