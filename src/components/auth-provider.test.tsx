import { render, screen, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SWRConfig } from "swr";
import { AuthProvider, useAuth } from "@/components/auth-provider";
import { AUTH_UNAUTHORIZED_EVENT } from "@/lib/http";
import { fetchSession, logoutSession, type SessionUser } from "@/lib/auth";

vi.mock("@/lib/auth", () => ({
  fetchSession: vi.fn(),
  logoutSession: vi.fn(),
}));

const mockedFetchSession = vi.mocked(fetchSession);
const mockedLogoutSession = vi.mocked(logoutSession);

function AuthConsumer() {
  const { user, loading, logout } = useAuth();

  if (loading) return <div>Loading...</div>;
  if (!user) return <div>No user</div>;

  return (
    <div>
      <span>User: {user.displayName}</span>
      <button onClick={() => void logout()}>Logout</button>
    </div>
  );
}

function renderWithProvider(ui: React.ReactElement = <AuthConsumer />) {
  return render(
    <SWRConfig value={{ dedupingInterval: 0, provider: () => new Map() }}>
      <AuthProvider>{ui}</AuthProvider>
    </SWRConfig>
  );
}

const testUser: SessionUser = {
  id: "u1",
  login: "testuser",
  displayName: "Test User",
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("AuthProvider", () => {
  it("fetches session and provides user", async () => {
    mockedFetchSession.mockResolvedValue(testUser);

    renderWithProvider();

    expect(screen.getByText("Loading...")).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText("User: Test User")).toBeInTheDocument();
    });
  });

  it("shows no user when session returns null", async () => {
    mockedFetchSession.mockResolvedValue(null);

    renderWithProvider();

    await waitFor(() => {
      expect(screen.getByText("No user")).toBeInTheDocument();
    });
  });

  it("clears user on AUTH_UNAUTHORIZED_EVENT", async () => {
    mockedFetchSession.mockResolvedValue(testUser);

    renderWithProvider();

    await waitFor(() => {
      expect(screen.getByText("User: Test User")).toBeInTheDocument();
    });

    act(() => {
      window.dispatchEvent(new CustomEvent(AUTH_UNAUTHORIZED_EVENT));
    });

    await waitFor(() => {
      expect(screen.getByText("No user")).toBeInTheDocument();
    });
  });

  it("calls logoutSession and clears user on logout", async () => {
    mockedFetchSession.mockResolvedValue(testUser);
    mockedLogoutSession.mockResolvedValue({ status: "ok" } as never);

    const user = userEvent.setup();
    renderWithProvider();

    await waitFor(() => {
      expect(screen.getByText("User: Test User")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Logout"));

    expect(mockedLogoutSession).toHaveBeenCalledTimes(1);

    await waitFor(() => {
      expect(screen.getByText("No user")).toBeInTheDocument();
    });
  });
});

describe("useAuth", () => {
  it("throws when used outside AuthProvider", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});

    expect(() => render(<AuthConsumer />)).toThrow(
      "useAuth must be used within AuthProvider"
    );

    spy.mockRestore();
  });
});
