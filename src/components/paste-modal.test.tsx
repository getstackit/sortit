import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PasteModal } from "@/components/paste-modal";
import { createIssue } from "@/lib/issues";

const push = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));

vi.mock("@/lib/issues", async () => {
  const actual = await vi.importActual<typeof import("@/lib/issues")>("@/lib/issues");
  return {
    ...actual,
    createIssue: vi.fn(),
  };
});

describe("PasteModal", () => {
  beforeEach(() => {
    push.mockReset();
    vi.mocked(createIssue).mockReset();
  });

  it("submits a new issue and navigates to its detail page", async () => {
    const onIssueCreated = vi.fn();
    const onOpenChange = vi.fn();
    vi.mocked(createIssue).mockResolvedValue({
      id: "issue-123",
      raw: "Crash on export",
      tags: ["bug"],
      createdBy: "test",
      createdAt: new Date("2026-03-08T12:00:00Z").toISOString(),
      status: "open",
    });

    render(
      <PasteModal
        open
        onOpenChange={onOpenChange}
        onIssueCreated={onIssueCreated}
      />
    );

    await userEvent.type(screen.getByPlaceholderText("paste anything... (markdown supported)"), "Crash on export");
    await userEvent.click(screen.getByRole("button", { name: "Submit" }));

    await waitFor(() => {
      expect(createIssue).toHaveBeenCalledWith({ raw: "Crash on export" });
      expect(onIssueCreated).toHaveBeenCalledWith(
        expect.objectContaining({ id: "issue-123" })
      );
      expect(onOpenChange).toHaveBeenCalledWith(false);
      expect(push).toHaveBeenCalledWith("/issues/issue-123");
    });
  });
});
