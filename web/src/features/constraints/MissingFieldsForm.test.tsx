import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MissingFieldsForm } from "./MissingFieldsForm";

describe("MissingFieldsForm", () => {
  it("renders nothing when nothing is missing", () => {
    const { container } = render(<MissingFieldsForm missing={[]} onSubmit={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("submits the missing days", async () => {
    const onSubmit = vi.fn();
    render(<MissingFieldsForm missing={["days"]} onSubmit={onSubmit} />);

    const days = screen.getByLabelText("天数");
    await userEvent.clear(days);
    await userEvent.type(days, "3");
    await userEvent.click(screen.getByRole("button", { name: "补充并继续" }));

    expect(onSubmit).toHaveBeenCalledWith("共 3 天");
  });

  it("blocks out-of-range days", async () => {
    const onSubmit = vi.fn();
    render(<MissingFieldsForm missing={["days"]} onSubmit={onSubmit} />);

    const days = screen.getByLabelText("天数");
    await userEvent.clear(days);
    await userEvent.type(days, "5");

    expect(await screen.findByText("天数需在 1–4 之间。")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "补充并继续" })).toBeDisabled();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
