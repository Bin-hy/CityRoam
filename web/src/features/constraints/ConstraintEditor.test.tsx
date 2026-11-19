import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConstraintEditor, type EditableConstraint } from "./ConstraintEditor";

const items: EditableConstraint[] = [
  { key: "budget", label: "总预算", value: "1000", hardness: "hard" },
  { key: "pace", label: "节奏", value: "轻松", hardness: "soft" },
];

describe("ConstraintEditor", () => {
  it("groups constraints by hardness", () => {
    render(<ConstraintEditor items={items} onChange={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "硬约束" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "软约束" })).toBeInTheDocument();
    expect(screen.getByLabelText("总预算的值")).toHaveValue("1000");
  });

  it("toggles a hard constraint to soft", async () => {
    const onChange = vi.fn();
    render(<ConstraintEditor items={items} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "将总预算改为软约束" }));

    expect(onChange).toHaveBeenCalledWith([
      { key: "budget", label: "总预算", value: "1000", hardness: "soft" },
      { key: "pace", label: "节奏", value: "轻松", hardness: "soft" },
    ]);
  });

  it("edits a constraint value", async () => {
    function Harness() {
      const [state, setState] = useState(items);
      return <ConstraintEditor items={state} onChange={setState} />;
    }
    render(<Harness />);

    const input = screen.getByLabelText("节奏的值");
    await userEvent.clear(input);
    await userEvent.type(input, "紧凑");

    expect(input).toHaveValue("紧凑");
  });
});
