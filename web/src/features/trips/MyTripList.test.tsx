import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TripSummary } from "@/api/types";
import { listTrips } from "@/api/trips";
import { useAuth } from "@/features/auth/useAuth";
import { MyTripList } from "./MyTripList";

vi.mock("@/api/trips", () => ({
  listTrips: vi.fn(),
}));

vi.mock("@/features/auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const listTripsMock = vi.mocked(listTrips);
const useAuthMock = vi.mocked(useAuth);

function authState(enabled: boolean, authenticated: boolean) {
  return {
    data: { enabled, authenticated, provider_name: "统一登录" },
    isPending: false,
    isError: false,
  } as unknown as ReturnType<typeof useAuth>;
}

const trips: TripSummary[] = [
  {
    id: "trip-1",
    city: "beijing",
    days: 2,
    status: "completed",
    created_at: "2026-11-19T10:00:00Z",
    updated_at: "2026-11-19T11:00:00Z",
    latest_run: { id: "run-1", status: "completed", stage: "present" },
    latest_version: { seq: 2, status: "accepted", created_at: "2026-11-19T11:00:00Z" },
  },
  {
    id: "trip-2",
    city: "beijing",
    days: 3,
    status: "planning",
    created_at: "2026-11-18T10:00:00Z",
    updated_at: "2026-11-18T10:00:00Z",
    latest_run: { id: "run-2", status: "awaiting_user", stage: "propose" },
  },
];

function renderList() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <MyTripList />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("MyTripList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthMock.mockReturnValue(authState(true, true));
    listTripsMock.mockResolvedValue({ trips });
  });

  it("renders nothing when auth is disabled", () => {
    useAuthMock.mockReturnValue(authState(false, false));
    renderList();

    expect(screen.queryByText("我的行程")).not.toBeInTheDocument();
    expect(listTripsMock).not.toHaveBeenCalled();
  });

  it("lists trips with status and latest version", async () => {
    renderList();

    expect(await screen.findByText("北京 · 2 天")).toBeInTheDocument();
    expect(screen.getByText(/版本 v2/)).toBeInTheDocument();
    expect(screen.getByText("已完成")).toBeInTheDocument();
    expect(screen.getByText("待确认")).toBeInTheDocument();

    const link = screen.getByRole("link", { name: /北京 · 2 天/ });
    expect(link).toHaveAttribute("href", "/trips/trip-1");
  });

  it("shows an empty state without trips", async () => {
    listTripsMock.mockResolvedValue({ trips: [] });
    renderList();

    expect(await screen.findByText("还没有行程")).toBeInTheDocument();
  });

  it("retries after a load error", async () => {
    listTripsMock.mockRejectedValueOnce(new Error("boom"));
    listTripsMock.mockResolvedValueOnce({ trips });
    renderList();

    expect(await screen.findByText("行程列表加载失败")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "重试" }));

    expect(await screen.findByText("北京 · 2 天")).toBeInTheDocument();
    expect(listTripsMock).toHaveBeenCalledTimes(2);
  });
});