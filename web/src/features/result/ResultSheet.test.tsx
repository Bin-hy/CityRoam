import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ResultModel, ResultSelection, ResultViewMode } from "./model";
import { ResultSheet } from "./ResultSheet";

vi.mock("./ResultShell", () => ({
  ResultShell: (props: {
    view: ResultViewMode;
    activeDay: number | null;
    hideToolbar?: boolean;
    readOnly?: boolean;
  }) => (
    <div
      data-testid="result-shell"
      data-view={props.view}
      data-day={String(props.activeDay)}
      data-hide-toolbar={String(!!props.hideToolbar)}
      data-readonly={String(!!props.readOnly)}
    />
  ),
}));

const model = {
  city: "北京",
  title: "北京 · 2 天",
  summary: "",
  overview: { days: 2, poiCount: 2, walkingKM: 1, pace: "relaxed" },
  budget: {
    tickets: 0,
    meals: 0,
    transit: 0,
    lodging: 0,
    total: 0,
    lodgingIncluded: false,
    estimated: false,
  },
  lodging: null,
  reminders: [],
  days: [
    { index: 0, date: "2026-11-20", label: "11 月 20 日", theme: "", start: "09:00", end: "10:00", costYuan: 0, stops: [], legs: [], gaps: [] },
    { index: 1, date: "2026-11-21", label: "11 月 21 日", theme: "", start: "09:00", end: "10:00", costYuan: 0, stops: [], legs: [], gaps: [] },
  ],
} as ResultModel;

const selection: ResultSelection = null;

function Harness({
  initialDay = null,
  initialView = "dossier",
  readOnly = false,
  onClose = () => {},
}: {
  initialDay?: number | null;
  initialView?: ResultViewMode;
  readOnly?: boolean;
  onClose?: (open: boolean) => void;
}) {
  const [open, setOpen] = useState(true);
  const [view, setView] = useState<ResultViewMode>(initialView);
  const [day, setDay] = useState<number | null>(initialDay);

  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        打开面板
      </button>
      <ResultSheet
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          onClose(next);
        }}
        model={model}
        view={view}
        onViewChange={setView}
        activeDay={day}
        onActiveDayChange={setDay}
        selection={selection}
        onSelect={vi.fn()}
        controls={<button type="button">预算</button>}
        isDraft={false}
        versionSeq={3}
        readOnly={readOnly}
      />
    </>
  );
}

describe("ResultSheet", () => {
  it("opens with the header controls and mounts the shared shell", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("北京 · 2 天")).toBeInTheDocument();
    expect(screen.getByRole("tablist", { name: "结果视图" })).toBeInTheDocument();
    expect(screen.getByTestId("result-shell")).toHaveAttribute("data-hide-toolbar", "true");

    await user.click(screen.getByRole("button", { name: "关闭" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("lands on the first day when opened without a valid day", () => {
    const onDayChange = vi.fn();
    render(
      <ResultSheet
        open
        onOpenChange={vi.fn()}
        model={model}
        view="dossier"
        onViewChange={vi.fn()}
        activeDay={null}
        onActiveDayChange={onDayChange}
        selection={null}
        onSelect={vi.fn()}
        isDraft={false}
        versionSeq={null}
      />,
    );

    expect(onDayChange).toHaveBeenCalledWith(0);
  });

  it("switches views while keeping the active day and hides day tabs on the board", async () => {
    const user = userEvent.setup();
    render(<Harness initialDay={1} />);

    await user.click(screen.getByRole("tab", { name: "地图" }));
    expect(screen.getByTestId("result-shell")).toHaveAttribute("data-view", "map");
    expect(screen.getByTestId("result-shell")).toHaveAttribute("data-day", "1");
    expect(screen.getByRole("tablist", { name: "行程天数" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "全部" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "看板" }));
    expect(screen.getByTestId("result-shell")).toHaveAttribute("data-view", "board");
    expect(screen.getByTestId("result-shell")).toHaveAttribute("data-day", "1");
    expect(screen.queryByRole("tablist", { name: "行程天数" })).not.toBeInTheDocument();
  });

  it("reports closing through the callback", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);

    await user.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalledWith(false);
  });

  it("hides the extra controls when read-only", () => {
    render(<Harness readOnly />);

    expect(screen.queryByRole("button", { name: "更多" })).not.toBeInTheDocument();
    expect(screen.getByTestId("result-shell")).toHaveAttribute("data-readonly", "true");
  });

  it("does not introduce a horizontal scrolling container", () => {
    render(<Harness />);

    expect(screen.getByRole("dialog").className).not.toMatch(/overflow-x-(auto|scroll)/);
  });
});