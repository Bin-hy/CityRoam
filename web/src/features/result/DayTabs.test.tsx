import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ResultDay } from "./model";
import { DayTabs } from "./DayTabs";

const days: ResultDay[] = [
  {
    index: 0,
    date: "2026-11-21",
    label: "11 月 21 日 周六",
    theme: "故宫",
    start: "09:00",
    end: "18:00",
    costYuan: 320,
    stops: [],
    legs: [],
    gaps: [],
  },
  {
    index: 1,
    date: "2026-11-22",
    label: "11 月 22 日 周日",
    theme: "天坛",
    start: "09:00",
    end: "17:00",
    costYuan: 0,
    stops: [],
    legs: [],
    gaps: [],
  },
];

describe("DayTabs", () => {
  it("renders all days with dates and costs, hiding zero cost", () => {
    render(<DayTabs days={days} activeDay={null} onChange={vi.fn()} />);

    expect(screen.getByRole("tab", { name: "全部" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: /D1 · 11 月 21 日 周六/ })).toHaveTextContent("¥320");
    expect(screen.getByRole("tab", { name: /D2/ })).not.toHaveTextContent("¥0");
    expect(screen.getByRole("tab", { name: /D1/ })).toHaveAttribute("aria-controls", "result-view");
  });

  it("switches with clicks and wraps the keyboard across all days", async () => {
    const onChange = vi.fn();
    render(<DayTabs days={days} activeDay={null} onChange={onChange} />);

    await userEvent.click(screen.getByRole("tab", { name: /D2/ }));
    expect(onChange).toHaveBeenCalledWith(1);

    fireEvent.keyDown(screen.getByRole("tab", { name: "全部" }), { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith(0);

    fireEvent.keyDown(screen.getByRole("tab", { name: "全部" }), { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith(1);

    fireEvent.keyDown(screen.getByRole("tab", { name: "全部" }), { key: "End" });
    expect(onChange).toHaveBeenLastCalledWith(1);
  });

  it("omits the all tab and cycles concrete days when allowAll is false", () => {
    const onChange = vi.fn();
    render(<DayTabs days={days} activeDay={null} onChange={onChange} allowAll={false} />);

    expect(screen.queryByRole("tab", { name: "全部" })).not.toBeInTheDocument();
    const first = screen.getByRole("tab", { name: /D1/ });
    expect(first).toHaveAttribute("tabindex", "0");

    fireEvent.keyDown(first, { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith(1);

    fireEvent.keyDown(first, { key: "Home" });
    expect(onChange).toHaveBeenLastCalledWith(0);

    fireEvent.keyDown(first, { key: "End" });
    expect(onChange).toHaveBeenLastCalledWith(1);
  });

  it("keeps a single selected tab in the tab order", () => {
    render(<DayTabs days={days} activeDay={0} onChange={vi.fn()} allowAll={false} />);

    expect(screen.getByRole("tab", { name: /D1/ })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: /D1/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: /D2/ })).toHaveAttribute("tabindex", "-1");
  });
});