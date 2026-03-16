import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TokenSettingsPage } from "@/components/token-settings-page";
import {
  createAPIToken,
  listAPITokens,
  revokeAPIToken,
  type APITokenRecord,
} from "@/lib/auth";

vi.mock("@/components/auth-provider", () => ({
  useAuth: () => ({
    user: { id: "u1", login: "jonnii", displayName: "Jon", email: "jon@test.com" },
    loading: false,
  }),
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

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    listAPITokens: vi.fn(),
    createAPIToken: vi.fn(),
    revokeAPIToken: vi.fn(),
  };
});

const mockedListTokens = vi.mocked(listAPITokens);
const mockedCreateToken = vi.mocked(createAPIToken);
const mockedRevokeToken = vi.mocked(revokeAPIToken);

const testToken: APITokenRecord = {
  id: "tok-1",
  tokenPrefix: "splat_abc",
  createdAt: "2026-03-01T00:00:00Z",
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("TokenSettingsPage", () => {
  it("displays user info and loads tokens", async () => {
    mockedListTokens.mockResolvedValue([testToken]);

    render(<TokenSettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("Jon")).toBeInTheDocument();
    });
    expect(screen.getByText("@jonnii • jon@test.com")).toBeInTheDocument();
    expect(screen.getByText("splat_abc...")).toBeInTheDocument();
  });

  it("shows empty state when no tokens exist", async () => {
    mockedListTokens.mockResolvedValue([]);

    render(<TokenSettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("No tokens yet.")).toBeInTheDocument();
    });
  });

  it("shows error when token loading fails", async () => {
    mockedListTokens.mockRejectedValue(new Error("Network error"));

    render(<TokenSettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });

  it("creates a token and shows it", async () => {
    mockedListTokens.mockResolvedValue([]);
    mockedCreateToken.mockResolvedValue({
      token: "splat_full_secret_value",
      metadata: {
        id: "tok-2",
        tokenPrefix: "splat_ful",
        createdAt: "2026-03-15T00:00:00Z",
      },
    });

    const user = userEvent.setup();
    render(<TokenSettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("No tokens yet.")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Create token"));

    await waitFor(() => {
      expect(screen.getByText("splat_full_secret_value")).toBeInTheDocument();
    });
    expect(screen.getByText("Copy this now")).toBeInTheDocument();
    expect(screen.getByText("splat_ful...")).toBeInTheDocument();
  });

  it("revokes a token", async () => {
    mockedListTokens.mockResolvedValue([testToken]);
    mockedRevokeToken.mockResolvedValue({ status: "ok" } as never);

    const user = userEvent.setup();
    render(<TokenSettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("splat_abc...")).toBeInTheDocument();
    });

    // Click Revoke on the token row to open confirmation dialog
    await user.click(screen.getByText("Revoke"));

    // Confirm in the dialog (second "Revoke" button)
    const revokeButtons = screen.getAllByText("Revoke");
    await user.click(revokeButtons[revokeButtons.length - 1]);

    expect(mockedRevokeToken).toHaveBeenCalledWith("tok-1");

    await waitFor(() => {
      expect(screen.getByText("Revoked")).toBeInTheDocument();
    });
  });

  it("shows error when create fails", async () => {
    mockedListTokens.mockResolvedValue([]);
    mockedCreateToken.mockRejectedValue(new Error("Create failed"));

    const user = userEvent.setup();
    render(<TokenSettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("No tokens yet.")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Create token"));

    await waitFor(() => {
      expect(screen.getByText("Create failed")).toBeInTheDocument();
    });
  });

  it("shows error when revoke fails", async () => {
    mockedListTokens.mockResolvedValue([testToken]);
    mockedRevokeToken.mockRejectedValue(new Error("Revoke failed"));

    const user = userEvent.setup();
    render(<TokenSettingsPage />);

    await waitFor(() => {
      expect(screen.getByText("splat_abc...")).toBeInTheDocument();
    });

    // Click Revoke on the token row to open confirmation dialog
    await user.click(screen.getByText("Revoke"));

    // Confirm in the dialog (second "Revoke" button)
    const revokeButtons = screen.getAllByText("Revoke");
    await user.click(revokeButtons[revokeButtons.length - 1]);

    await waitFor(() => {
      expect(screen.getByText("Revoke failed")).toBeInTheDocument();
    });
  });
});
