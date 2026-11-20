import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes, useLocation, useSearchParams } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthMe } from "@/api/types";
import { getMe } from "@/api/auth";
import { AuthGate } from "./AuthGate";
import { LoginPage, loginErrorMessage, sanitizeReturnTo } from "./LoginPage";

vi.mock("@/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/auth")>();
  return { ...actual, getMe: vi.fn() };
});

const getMeMock = vi.mocked(getMe);

function authMe(overrides: Partial<AuthMe> = {}): AuthMe {
  return { enabled: true, authenticated: false, provider_name: "统一登录", ...overrides };
}

function renderAt(path: string, element: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>{element}</MemoryRouter>
    </QueryClientProvider>,
  );
}

function LocationProbe() {
  const location = useLocation();
  return <div>at:{`${location.pathname}${location.search}`}</div>;
}

function LoginProbe() {
  const [params] = useSearchParams();
  return <div>login:{params.get("return_to")}</div>;
}

describe("AuthGate", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders children when auth is disabled", async () => {
    getMeMock.mockResolvedValue(authMe({ enabled: false }));
    renderAt(
      "/",
      <Routes>
        <Route
          path="/"
          element={
            <AuthGate>
              <div>open-content</div>
            </AuthGate>
          }
        />
      </Routes>,
    );

    expect(await screen.findByText("open-content")).toBeInTheDocument();
  });

  it("redirects unauthenticated visitors to login with the current path", async () => {
    getMeMock.mockResolvedValue(authMe());
    renderAt(
      "/trips/t1?tab=map",
      <Routes>
        <Route
          path="/trips/:tripId"
          element={
            <AuthGate>
              <div>protected-content</div>
            </AuthGate>
          }
        />
        <Route path="/login" element={<LoginProbe />} />
      </Routes>,
    );

    expect(await screen.findByText("login:/trips/t1?tab=map")).toBeInTheDocument();
    expect(screen.queryByText("protected-content")).not.toBeInTheDocument();
  });

  it("renders children for an authenticated user", async () => {
    getMeMock.mockResolvedValue(
      authMe({ authenticated: true, user: { id: "u1", role: "user" } }),
    );
    renderAt(
      "/",
      <Routes>
        <Route
          path="/"
          element={
            <AuthGate>
              <div>protected-content</div>
            </AuthGate>
          }
        />
      </Routes>,
    );

    expect(await screen.findByText("protected-content")).toBeInTheDocument();
  });
});

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the provider and a login link that preserves return_to", async () => {
    getMeMock.mockResolvedValue(authMe());
    renderAt("/login?return_to=%2Ftrips%2Ft1", <LoginPage />);

    expect(await screen.findByText(/使用 统一登录 验证身份/)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /使用 统一登录 登录/ });
    expect(link).toHaveAttribute("href", "/api/auth/login?return_to=%2Ftrips%2Ft1");
  });

  it("maps callback error codes to readable messages", async () => {
    getMeMock.mockResolvedValue(authMe());
    renderAt("/login?error=login_state_invalid", <LoginPage />);

    expect(await screen.findByText("登录状态已失效，请重新发起登录。")).toBeInTheDocument();
  });

  it("sends authenticated visitors back to the requested page", async () => {
    getMeMock.mockResolvedValue(
      authMe({ authenticated: true, user: { id: "u1", role: "user" } }),
    );
    renderAt(
      "/login?return_to=%2Ftrips%2Ft1",
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/trips/:tripId" element={<LocationProbe />} />
      </Routes>,
    );

    expect(await screen.findByText("at:/trips/t1")).toBeInTheDocument();
  });

  it("rejects unsafe return targets", () => {
    expect(sanitizeReturnTo("//evil.example")).toBe("/");
    expect(sanitizeReturnTo("https://evil.example")).toBe("/");
    expect(sanitizeReturnTo("/trips/t1")).toBe("/trips/t1");
    expect(loginErrorMessage("unknown")).toBe("登录未完成，请重试。");
    expect(loginErrorMessage(null)).toBeNull();
  });
});