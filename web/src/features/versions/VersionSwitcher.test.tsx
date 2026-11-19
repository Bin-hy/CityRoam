import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { PlanVersion } from "@/api/types";
import { VersionSwitcher } from "./VersionSwitcher";

function version(seq: number, status: PlanVersion["status"]): PlanVersion {
  return {
    id: `v${seq}`,
    trip_id: "trip-1",
    run_id: "run-1",
    seq,
    plan: {},
    constraints: {},
    violations: [],
    status,
    created_at: "2026-11-19T10:00:00Z",
  };
}

const versions = [version(3, "accepted"), version(2, "superseded"), version(1, "superseded")];

describe("VersionSwitcher", () => {
  it("switches the selected version and restores only after confirmation", async () => {
    const onSelect = vi.fn();
    const onRestore = vi.fn();
    render(
      <VersionSwitcher
        versions={versions}
        selectedSeq={2}
        currentSeq={3}
        onSelect={onSelect}
        onRestore={onRestore}
      />,
    );

    await userEvent.selectOptions(screen.getByLabelText("版本"), "1");
    expect(onSelect).toHaveBeenCalledWith(1);

    await userEvent.click(screen.getByRole("button", { name: "回退到此版本" }));
    expect(screen.getByText("回退到 v2")).toBeInTheDocument();
    expect(onRestore).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "确认回退" }));
    expect(onRestore).toHaveBeenCalledWith(2);
  });

  it("cancels the rollback without calling the API", async () => {
    const onRestore = vi.fn();
    render(
      <VersionSwitcher
        versions={versions}
        selectedSeq={1}
        currentSeq={3}
        onSelect={() => undefined}
        onRestore={onRestore}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "回退到此版本" }));
    await userEvent.click(screen.getByRole("button", { name: "取消" }));

    expect(onRestore).not.toHaveBeenCalled();
  });

  it("hides rollback for the current version", () => {
    render(
      <VersionSwitcher
        versions={versions}
        selectedSeq={3}
        currentSeq={3}
        onSelect={() => undefined}
        onRestore={() => undefined}
      />,
    );

    expect(screen.queryByRole("button", { name: "回退到此版本" })).not.toBeInTheDocument();
  });

  it("shows restore errors", () => {
    render(
      <VersionSwitcher
        versions={versions}
        selectedSeq={2}
        currentSeq={3}
        onSelect={() => undefined}
        onRestore={() => undefined}
        error="版本已是当前版本"
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("版本已是当前版本");
  });
});
