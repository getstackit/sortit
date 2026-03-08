import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AppShell } from "@/components/app-shell";

const push = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));

vi.mock("@/components/paste-modal", () => ({
  PasteModal: ({
    open,
    onOpenChange,
  }: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
  }) =>
    open ? (
      <div>
        <p>Paste anything</p>
        <button type="button" onClick={() => onOpenChange(false)}>
          Close composer
        </button>
      </div>
    ) : null,
}));

describe("AppShell shortcuts", () => {
  beforeEach(() => {
    push.mockReset();
    window.localStorage.clear();
  });

  it("opens the shortcut help with the global ? shortcut", async () => {
    render(<AppShell sidebar={<div>Sidebar</div>}>Body</AppShell>);

    fireEvent.keyDown(window, { key: "?" });

    await waitFor(() => {
      expect(screen.getByText("Keyboard shortcuts")).toBeInTheDocument();
    });
  });

  it("navigates with the G then M shortcut sequence", async () => {
    render(<AppShell sidebar={<div>Sidebar</div>}>Body</AppShell>);

    fireEvent.keyDown(window, { key: "g" });
    fireEvent.keyDown(window, { key: "m" });

    await waitFor(() => {
      expect(push).toHaveBeenCalledWith("/map");
    });
  });

  it("runs page shortcuts like slash when not typing", async () => {
    const focusSearch = vi.fn();

    render(
      <AppShell
        sidebar={<div>Sidebar</div>}
        shortcuts={[
          {
            key: "/",
            description: "Focus semantic search",
            action: focusSearch,
          },
        ]}
      >
        Body
      </AppShell>
    );

    fireEvent.keyDown(window, { key: "/" });

    await waitFor(() => {
      expect(focusSearch).toHaveBeenCalled();
    });
  });
});
