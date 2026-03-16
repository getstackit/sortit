import { render, screen } from "@testing-library/react";
import { usePathname } from "next/navigation";
import { AuthGate } from "@/components/auth-gate";
import { useAuth } from "@/components/auth-provider";

vi.mock("next/navigation", () => ({
  usePathname: vi.fn(),
}));

vi.mock("@/components/auth-provider", () => ({
  useAuth: vi.fn(),
}));

vi.mock("@/components/login-screen", () => ({
  LoginScreen: () => <div data-testid="login-screen">Login Screen</div>,
}));

const mockedUsePathname = vi.mocked(usePathname);
const mockedUseAuth = vi.mocked(useAuth);

beforeEach(() => {
  mockedUsePathname.mockReturnValue("/");
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("AuthGate", () => {
  it("shows loading indicator when session is loading", () => {
    mockedUseAuth.mockReturnValue({
      user: null,
      loading: true,
      refresh: vi.fn(),
      logout: vi.fn(),
    });

    render(<AuthGate>Protected</AuthGate>);

    expect(screen.getByText("Loading session...")).toBeInTheDocument();
    expect(screen.queryByText("Protected")).not.toBeInTheDocument();
  });

  it("shows login screen when unauthenticated", () => {
    mockedUseAuth.mockReturnValue({
      user: null,
      loading: false,
      refresh: vi.fn(),
      logout: vi.fn(),
    });

    render(<AuthGate>Protected</AuthGate>);

    expect(screen.getByTestId("login-screen")).toBeInTheDocument();
    expect(screen.queryByText("Protected")).not.toBeInTheDocument();
  });

  it("renders children when authenticated", () => {
    mockedUseAuth.mockReturnValue({
      user: { id: "1", login: "test", displayName: "Test" },
      loading: false,
      refresh: vi.fn(),
      logout: vi.fn(),
    });

    render(<AuthGate>Protected</AuthGate>);

    expect(screen.getByText("Protected")).toBeInTheDocument();
  });

  it("bypasses auth gate on /auth/cli-login", () => {
    mockedUsePathname.mockReturnValue("/auth/cli-login");
    mockedUseAuth.mockReturnValue({
      user: null,
      loading: true,
      refresh: vi.fn(),
      logout: vi.fn(),
    });

    render(<AuthGate>CLI Login Page</AuthGate>);

    expect(screen.getByText("CLI Login Page")).toBeInTheDocument();
    expect(screen.queryByText("Loading session...")).not.toBeInTheDocument();
  });
});
