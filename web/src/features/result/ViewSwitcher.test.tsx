import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ViewSwitcher } from "./ViewSwitcher";

describe("ViewSwitcher", () => {
  it("marks the current view and switches on click", async () => {
    const onChange = vi.fn();
    render(<ViewSwitcher value="dossier" onChange={onChange} />);

    expect(screen.getByRole("tab", { name: "手册" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "手册" })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: "地图" })).toHaveAttribute("aria-selected", "false");

    await userEvent.click(screen.getByRole("tab", { name: "看板" }));
    expect(onChange).toHaveBeenCalledWith("board");
  });

  it("cycles views with arrow keys and moves focus", () => {
    const onChange = vi.fn();
    render(<ViewSwitcher value="dossier" onChange={onChange} />);

    fireEvent.keyDown(screen.getByRole("tab", { name: "手册" }), { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith("map");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "地图" }));

    fireEvent.keyDown(screen.getByRole("tab", { name: "地图" }), { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith("dossier");

    fireEvent.keyDown(screen.getByRole("tab", { name: "手册" }), { key: "End" });
    expect(onChange).toHaveBeenLastCalledWith("board");

    fireEvent.keyDown(screen.getByRole("tab", { name: "地图" }), { key: "Home" });
    expect(onChange).toHaveBeenLastCalledWith("dossier");
  });
});