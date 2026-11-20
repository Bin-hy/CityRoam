import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import type { SharedTrip } from "@/api/types";
import * as api from "@/api/trips";
import { SharePage } from "./SharePage";

vi.mock("@/api/trips", () => ({ getSharedTrip: vi.fn() }));

vi.mock("@/features/timeline/ItineraryTimeline", () => ({
  ItineraryTimeline: () => <div>readonly-timeline</div>,
}));

const getSharedTrip = vi.mocked(api.getSharedTrip);

const itinerary = {
  city: "beijing",
  days: [
    {
      date: "2026-11-20",
      start_time: "09:00",
      end_time: "18:00",
      day_cost_yuan: 60,
      blocks: [
        { id: "b1", type: "poi", title: "故宫", start: "09:00", end: "11:00", duration_min: 120, cost_yuan: 60 },
      ],
    },
  ],
  totals: { cost_yuan: 60, poi_count: 1, walking_km: 1.2 },
};

const shared: SharedTrip = {
  slug: "share-1",
  trip_id: "trip-1",
  created_at: "2026-11-19T10:00:00Z",
  version: {
    id: "v3",
    trip_id: "trip-1",
    run_id: "run-1",
    seq: 3,
    plan: itinerary as unknown as SharedTrip["version"]["plan"],
    constraints: {},
    violations: [],
    status: "accepted",
    created_at: "2026-11-19T10:00:00Z",
  },
};

function renderShare() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/share/share-1"]}>
        <Routes>
          <Route path="/share/:slug" element={<SharePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("SharePage", () => {
  const writeText = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    writeText.mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    getSharedTrip.mockResolvedValue(shared);
  });

  it("renders a read-only snapshot with source and AI labels", async () => {
    renderShare();

    expect(await screen.findByText("北京 · 1 天")).toBeInTheDocument();
    expect(screen.getByText("AI 生成")).toBeInTheDocument();
    expect(screen.getByText("只读")).toBeInTheDocument();
    expect(screen.getByText("readonly-timeline")).toBeInTheDocument();
    expect(screen.getByText("版本 v3")).toBeInTheDocument();
    expect(screen.getByText("行程 trip-1")).toBeInTheDocument();
    expect(getSharedTrip).toHaveBeenCalledWith("share-1");
    expect(screen.queryByRole("button", { name: /编辑|保存|调整/ })).not.toBeInTheDocument();
  });

  it("copies the itinerary as plain text", async () => {
    renderShare();
    const copy = await screen.findByRole("button", { name: /复制行程文本/ });

    await userEvent.click(copy);

    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
    const text = writeText.mock.calls[0][0] as string;
    expect(text).toContain("北京 1 天行程");
    expect(text).toContain("09:00-11:00 景点 故宫 ¥60");
    expect(text).toContain("总计：¥60 · 景点 1 个 · 步行 1.2 km");
    expect(screen.getByRole("button", { name: /已复制/ })).toBeInTheDocument();
  });

  it("shows sanitized research without internal fields", async () => {
    getSharedTrip.mockResolvedValue({
      ...shared,
      version: {
        ...shared.version,
        research: {
          accommodation: {
            mentioned: true,
            selected: { name: "全季酒店", price_per_night_yuan: 480, price_estimated: true },
            nights: 1,
            total_yuan: 480,
            include_in_budget: true,
          },
          guides: [
            {
              poi_id: "b1",
              weight: 1.2,
              mentions: 2,
              note: "早上人少",
              citations: [{ title: "故宫攻略", url: "https://example.com/tip", site: "攻略站" }],
            },
          ],
        },
      },
    });
    renderShare();

    expect(await screen.findByText("全季酒店")).toBeInTheDocument();
    expect(screen.getByText("早上人少")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /故宫攻略/ });
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByText("外部摘要，仅供参考。")).toBeInTheDocument();
    expect(screen.queryByText(/tool_calls/)).not.toBeInTheDocument();
  });

  it("shows a not-found state for revoked or unknown links", async () => {
    getSharedTrip.mockRejectedValue(new ApiError(404, "share_not_found", "分享链接不存在", "req-1"));
    renderShare();

    expect(await screen.findByText("分享不存在或已撤销")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "返回首页" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试" })).not.toBeInTheDocument();
  });
});
