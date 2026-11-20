import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthMe } from "@/api/types";
import { logout } from "@/api/auth";
import { useAuth } from "@/features/auth/useAuth";
import { AppHeader } from "./AppHeader";

vi.mock("@/api/auth", () => ({
  logout: vi.fn(),
  loginURL: (returnTo: string) => `/api/auth/login?return_to=${encodeURIComponent(returnTo)}`,
}));

vi.mock("@/features/auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const logoutMock = vi.mocked(logout);
const useAuthMock = vi.mocked(useAuth);

function authState(me: AuthMe) {
  return { data: me, isPending: false, isError: false } as unknown as ReturnType<typeof useAuth>;
}

function renderHeader() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<AppHeader />} />
          <Route path="/login" element={<div>login-page</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("AppHeader", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    logoutMock.mockResolvedValue(undefined);
  });

  it("renders nothing when auth is disabled", () => {
    useAuthMock.mockReturnValue(authState({ enabled: false, authenticated: false, provider_name: "统一登录" }));
    renderHeader();

    expect(screen.queryByRole("button", { name: "退出登录" })).not.toBeInTheDocument();
  });

  it("shows the user name and admin badge", () => {
    useAuthMock.mockReturnValue(
      authState({
        enabled: true,
        authenticated: true,
        provider_name: "统一登录",
        user: { id: "u1", name: "张三", email: "zhang@example.com", role: "admin" },
      }),
    );
    renderHeader();

    expect(screen.getByText("张三")).toBeInTheDocument();
    expect(screen.getByText("管理员")).toBeInTheDocument();
  });

  it("logs out and returns to the login page", async () => {
    useAuthMock.mockReturnValue(
      authState({
        enabled: true,
        authenticated: true,
        provider_name: "统一登录",
        user: { id: "u1", name: "张三", role: "user" },
      }),
    );
    renderHeader();

    await userEvent.click(screen.getByRole("button", { name: "退出登录" }));

    expect(logoutMock).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("login-page")).toBeInTheDocument();
  });
});