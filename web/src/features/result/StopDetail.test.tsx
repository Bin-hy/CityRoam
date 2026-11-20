import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ResultDay, ResultStop } from "./model";
import { GuideSection, StopDetail } from "./StopDetail";

const day: ResultDay = {
  index: 0,
  date: "2026-11-20",
  label: "11 月 20 日 周五",
  theme: "故宫",
  start: "09:00",
  end: "18:00",
  costYuan: 60,
  stops: [],
  legs: [],
  gaps: [],
};

function stop(overrides: Partial<ResultStop> = {}): ResultStop {
  return {
    id: "b-1",
    dayIndex: 0,
    order: 0,
    type: "poi",
    typeLabel: "景点",
    title: "故宫",
    start: "09:00",
    end: "11:00",
    durationMin: 120,
    costYuan: 60,
    lat: 39.9163,
    lng: 116.3972,
    reason: "中轴线起点，上午人少。",
    guides: [],
    ...overrides,
  };
}

describe("StopDetail", () => {
  it("renders stop facts and the reason", () => {
    render(<StopDetail day={day} stop={stop()} />);

    expect(screen.getByRole("heading", { name: "故宫" })).toBeInTheDocument();
    expect(screen.getByText("120 分钟")).toBeInTheDocument();
    expect(screen.getByText("¥60")).toBeInTheDocument();
    expect(screen.getByText("中轴线起点，上午人少。")).toBeInTheDocument();
  });

  it("states missing reason and guides instead of leaving blanks", () => {
    render(<StopDetail day={day} stop={stop({ reason: null, guides: [] })} />);

    expect(screen.getByText("本次未生成额外说明。")).toBeInTheDocument();
    expect(screen.getByText("暂无攻略提及。")).toBeInTheDocument();
  });

  it("renders markup-like text as plain text", () => {
    render(<StopDetail day={day} stop={stop({ title: "<b>故宫</b>", reason: "<i>不要斜体</i>" })} />);

    expect(screen.getByText("<b>故宫</b>")).toBeInTheDocument();
    expect(screen.getByText("<i>不要斜体</i>")).toBeInTheDocument();
    expect(document.querySelector("b")).toBeNull();
    expect(document.querySelector("i")).toBeNull();
  });

  it("only links http(s) citations and shows site and date", () => {
    render(
      <GuideSection
        guides={[
          {
            poiID: "p1",
            note: "开门就进。",
            citations: [
              {
                title: "故宫攻略",
                url: "https://example.com/guide",
                site: "示例旅行",
                publishedAt: "2026-10-01T00:00:00Z",
              },
              { title: "坏链接", url: "javascript:alert(1)" },
            ],
          },
        ]}
      />,
    );

    const link = screen.getByRole("link", { name: "故宫攻略" });
    expect(link).toHaveAttribute("href", "https://example.com/guide");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByText(/示例旅行/)).toBeInTheDocument();
    expect(screen.getByText("坏链接")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "坏链接" })).not.toBeInTheDocument();
  });

  it("passes the day index to the adjust callback", async () => {
    const onAdjustDay = vi.fn();
    render(<StopDetail day={{ ...day, index: 1 }} stop={stop({ dayIndex: 1 })} onAdjustDay={onAdjustDay} />);

    await userEvent.click(screen.getByRole("button", { name: "调整这一天" }));
    expect(onAdjustDay).toHaveBeenCalledWith(1);
  });

  it("hides the adjust action in read-only mode", () => {
    render(<StopDetail day={day} stop={stop()} readOnly onAdjustDay={vi.fn()} />);

    expect(screen.queryByRole("button", { name: "调整这一天" })).not.toBeInTheDocument();
  });
});