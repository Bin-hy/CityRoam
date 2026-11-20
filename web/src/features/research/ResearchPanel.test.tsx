import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ResearchPanel } from "./ResearchPanel";
import type { ResearchBrief } from "./types";

const research: ResearchBrief = {
  status: "ok",
  accommodation: {
    mentioned: true,
    specified: ["全季酒店"],
    selected: {
      name: "全季酒店",
      address: "东城区某路 1 号",
      star: 4,
      score: 4.7,
      price_per_night_yuan: 480,
      price_estimated: true,
      provider: "飞猪",
      fetched_at: "2026-11-20T08:00:00Z",
      booking_url: "https://example.com/booking",
    },
    anchor_days: [0, 1],
    nights: 2,
    total_yuan: 960,
    price_estimated: true,
    include_in_budget: true,
  },
  guides: [
    {
      poi_id: "poi-0",
      weight: 1.4,
      mentions: 3,
      note: "傍晚人少，适合看日落",
      citations: [{ title: "北京两日", site: "小红书", url: "https://example.com/note", published_at: "2026-10-01" }],
    },
  ],
};

describe("ResearchPanel", () => {
  it("shows the selected hotel, price estimate and guide citations", () => {
    render(<ResearchPanel research={research} poiNames={{ "poi-0": "景山公园" }} />);

    expect(screen.getByText("全季酒店")).toBeInTheDocument();
    expect(screen.getByText("你指定的酒店")).toBeInTheDocument();
    expect(screen.getByText("东城区某路 1 号")).toBeInTheDocument();
    expect(screen.getByText(/¥480\/晚/)).toBeInTheDocument();
    expect(screen.getByText(/合计 ¥960/)).toBeInTheDocument();
    expect(screen.getByText("来源：飞猪")).toBeInTheDocument();
    expect(screen.getByText(/第 1 天、第 2 天/)).toBeInTheDocument();
    const booking = screen.getByRole("link", { name: "查看预订" });
    expect(booking).toHaveAttribute("rel", "noopener noreferrer");
    expect(booking).toHaveAttribute("target", "_blank");

    expect(screen.getByText("景山公园")).toBeInTheDocument();
    expect(screen.getByText("傍晚人少，适合看日落")).toBeInTheDocument();
    const citation = screen.getByRole("link", { name: /北京两日/ });
    expect(citation).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByText("外部摘要，仅供参考。")).toBeInTheDocument();
  });

  it("explains missing prices and empty guides", () => {
    render(
      <ResearchPanel
        research={{
          accommodation: { mentioned: true, selected: { name: "预算酒店", price_per_night_yuan: 0 } },
          guides: [],
        }}
      />,
    );

    expect(screen.getByText("未获取价格，未计入预算。")).toBeInTheDocument();
    expect(screen.getByText("未找到与候选点对应的攻略摘要。")).toBeInTheDocument();
  });

  it("marks degraded guide research as unavailable", () => {
    render(<ResearchPanel research={{ status: "degraded" }} />);
    expect(screen.getByText("攻略参考不可用。")).toBeInTheDocument();
  });

  it("hides recommendations when the hotel plugin is disabled", () => {
    render(
      <ResearchPanel
        research={research}
        plugins={{ hotels: { enabled: false, disabled_reason: "HOTEL_SEARCH_URL is empty" }, guides: { enabled: true } }}
      />,
    );

    expect(screen.getByText(/住宿插件未启用/)).toBeInTheDocument();
    expect(screen.queryByText("全季酒店")).not.toBeInTheDocument();
  });
});
