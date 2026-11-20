import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ResultModel, ResultSelection, ResultViewMode } from "./model";
import { ResultShell } from "./ResultShell";

vi.mock("./DossierView", () => ({
  DossierView: (props: {
    activeDay: number | null;
    onViewOnMap: (day: number) => void;
  }) => (
    <div data-testid="dossier" data-day={String(props.activeDay)}>
      <button type="button" onClick={() => props.onViewOnMap(1)}>
        看第二天地图
      </button>
    </div>
  ),
}));

vi.mock("./MapView", () => ({
  MapView: (props: { activeDay: number; readOnly?: boolean }) => (
    <div data-testid="map" data-day={String(props.activeDay)} data-readonly={String(!!props.readOnly)} />
  ),
}));

vi.mock("./BoardView", () => ({
  BoardView: (props: { activeDay: number | null; readOnly?: boolean }) => (
    <div data-testid="board" data-day={String(props.activeDay)} data-readonly={String(!!props.readOnly)} />
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

function Harness({
  initialView = "dossier",
  initialDay = null,
  initialSelection = null,
  resultModel = model,
  readOnly = false,
}: {
  initialView?: ResultViewMode;
  initialDay?: number | null;
  initialSelection?: ResultSelection;
  resultModel?: ResultModel | null;
  readOnly?: boolean;
}) {
  const [view, setView] = useState<ResultViewMode>(initialView);
  const [activeDay, setActiveDay] = useState<number | null>(initialDay);
  const [selection, setSelection] = useState<ResultSelection>(initialSelection);

  return (
    <ResultShell
      model={resultModel}
      view={view}
      onViewChange={setView}
      activeDay={activeDay}
      onActiveDayChange={setActiveDay}
      selection={selection}
      onSelect={setSelection}
      isDraft={false}
      versionSeq={2}
      readOnly={readOnly}
    />
  );
}

describe("ResultShell", () => {
  it("mounts the view chosen by props", () => {
    render(<Harness initialView="board" />);

    expect(screen.getByTestId("board")).toBeInTheDocument();
    expect(screen.queryByTestId("map")).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "看板" })).toHaveAttribute("aria-selected", "true");
  });

  it("keeps the active day and selection when switching views", async () => {
    const user = userEvent.setup();
    render(
      <Harness
        initialDay={1}
        initialSelection={{ kind: "stop", dayIndex: 1, stopID: "s-1" }}
      />,
    );

    await user.click(screen.getByRole("tab", { name: "地图" }));

    expect(screen.getByTestId("map")).toHaveAttribute("data-day", "1");
    expect(screen.getByRole("tab", { name: "D2 · 11 月 21 日" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("switches to the map for the day requested by the dossier", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(screen.getByRole("button", { name: "看第二天地图" }));

    expect(screen.getByTestId("map")).toHaveAttribute("data-day", "1");
    expect(screen.getByRole("tab", { name: "D2 · 11 月 21 日" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("falls back to the first day when opening the map without an active day", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(screen.getByRole("tab", { name: "地图" }));

    expect(screen.getByTestId("map")).toHaveAttribute("data-day", "0");
  });

  it("shows an empty state without mounting views when the model is missing", () => {
    render(<Harness resultModel={null} />);

    expect(screen.getByText("方案生成后会显示在这里。")).toBeInTheDocument();
    expect(screen.queryByTestId("dossier")).not.toBeInTheDocument();
    expect(screen.queryByTestId("map")).not.toBeInTheDocument();
    expect(screen.queryByTestId("board")).not.toBeInTheDocument();
  });
});