import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { CreatedTrip } from "@/api/types";
import { CreateForm } from "./CreateForm";
import { composeInput, emptyAdvanced } from "./composeInput";

const created: CreatedTrip = {
  trip_id: "trip-1",
  run_id: "run-1",
  events_url: "/api/runs/run-1/events",
};

describe("CreateForm", () => {
  it("submits the composed input with an idempotency key", async () => {
    const onSubmit = vi.fn().mockResolvedValue(created);
    const onCreated = vi.fn();
    render(<CreateForm onSubmit={onSubmit} onCreated={onCreated} />);

    await userEvent.type(screen.getByLabelText("行程需求"), "北京两天，想看历史");
    await userEvent.click(screen.getByRole("button", { name: /高级选项/ }));
    await userEvent.selectOptions(screen.getByLabelText("天数"), "2");
    await userEvent.click(screen.getByRole("button", { name: "开始规划" }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const request = onSubmit.mock.calls[0][0];
    expect(request.city).toBe("beijing");
    expect(request.input).toContain("北京两天，想看历史");
    expect(request.input).toContain("共 2 天");
    expect(request.client_request_id).toMatch(/^[0-9a-z-]{8,}$/);
    expect(onCreated).toHaveBeenCalledWith(created);
  });

  it("reuses the request key when retrying without edits", async () => {
    const onSubmit = vi
      .fn()
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce(created);
    render(<CreateForm onSubmit={onSubmit} />);

    await userEvent.type(screen.getByLabelText("行程需求"), "北京一天");
    const submit = screen.getByRole("button", { name: "开始规划" });
    await userEvent.click(submit);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    await userEvent.click(submit);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));

    expect(onSubmit.mock.calls[0][0].client_request_id).toBe(onSubmit.mock.calls[1][0].client_request_id);
  });

  it("fills the input from an example", async () => {
    render(<CreateForm onSubmit={vi.fn().mockResolvedValue(created)} />);

    await userEvent.click(screen.getByRole("button", { name: /带爸妈/ }));

    expect((screen.getByLabelText("行程需求") as HTMLTextAreaElement).value).toContain("带爸妈");
  });

  it("rejects an empty request", async () => {
    const onSubmit = vi.fn();
    render(<CreateForm onSubmit={onSubmit} />);

    await userEvent.click(screen.getByRole("button", { name: "开始规划" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("请先描述");
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe("composeInput", () => {
  it("appends advanced options to the description", () => {
    const input = composeInput("北京两天", {
      ...emptyAdvanced,
      budgetYuan: "1000",
      pace: "relaxed",
      meals: true,
    });

    expect(input).toBe("北京两天。节奏轻松，总预算 1000 元，需要安排午餐和晚餐。");
  });
});
