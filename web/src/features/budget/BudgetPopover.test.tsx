import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ResultBudget } from "@/features/result/model";
import { BudgetPopover } from "./BudgetPopover";

const budget: ResultBudget = {
  tickets: 100,
  meals: 50,
  transit: 30,
  lodging: 400,
  total: 580,
  lodgingIncluded: true,
  estimated: true,
};

describe("BudgetPopover", () => {
  it("shows the total trigger and breakdown", async () => {
    render(<BudgetPopover budget={budget} />);
    await userEvent.click(screen.getByRole("button", { name: /总计 ¥580/ }));
    expect(screen.getByText("预算明细")).toBeInTheDocument();
    expect(screen.getByText("门票").nextSibling).toHaveTextContent("¥100");
    expect(screen.getByText("住宿").nextSibling).toHaveTextContent("¥400");
    expect(screen.getByText(/含估算交通/)).toBeInTheDocument();
    expect(screen.getByText(/住宿价格为估算/)).toBeInTheDocument();
  });

  it("omits the lodging row and note when lodging is zero", async () => {
    render(<BudgetPopover budget={{ ...budget, lodging: 0, total: 180 }} />);
    await userEvent.click(screen.getByRole("button", { name: /总计/ }));

    expect(screen.queryByText("住宿")).not.toBeInTheDocument();
    expect(screen.queryByText(/住宿价格为估算/)).not.toBeInTheDocument();
  });

  it("toggles the lodging scope", async () => {
    const onToggleLodging = vi.fn();
    render(<BudgetPopover budget={budget} includeLodging onToggleLodging={onToggleLodging} />);
    await userEvent.click(screen.getByRole("button", { name: /总计/ }));
    await userEvent.click(screen.getByRole("button", { name: "不计住宿" }));
    expect(onToggleLodging).toHaveBeenCalledWith(false);
  });

  it("offers to include lodging when it is currently excluded", async () => {
    const onToggleLodging = vi.fn();
    render(
      <BudgetPopover
        budget={{ ...budget, lodgingIncluded: false }}
        includeLodging={false}
        onToggleLodging={onToggleLodging}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /总计/ }));
    await userEvent.click(screen.getByRole("button", { name: "计入住宿" }));
    expect(onToggleLodging).toHaveBeenCalledWith(true);
  });

  it("hides the scope toggle in read-only mode", async () => {
    render(<BudgetPopover budget={budget} readOnly onToggleLodging={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: /总计/ }));
    expect(screen.queryByRole("button", { name: "不计住宿" })).not.toBeInTheDocument();
  });

  it("shows a disabled trigger without a budget", () => {
    render(<BudgetPopover budget={null} />);
    expect(screen.getByRole("button", { name: "暂无预算" })).toBeDisabled();
  });
});