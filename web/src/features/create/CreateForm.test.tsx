import { render, screen, waitFor, within } from "@testing-library/react";
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

async function openPreferences() {
  await userEvent.click(screen.getByRole("button", { name: /^偏好/ }));
  return screen.findByRole("dialog");
}

describe("CreateForm", () => {
  it("submits the composed input with an idempotency key", async () => {
    const onSubmit = vi.fn().mockResolvedValue(created);
    const onCreated = vi.fn();
    render(<CreateForm onSubmit={onSubmit} onCreated={onCreated} />);

    await userEvent.type(screen.getByLabelText("行程需求"), "北京两天，想看历史");
    const dialog = await openPreferences();
    await userEvent.selectOptions(within(dialog).getByLabelText("天数"), "2");
    await userEvent.click(within(dialog).getByRole("button", { name: "完成" }));
    expect(screen.getByRole("button", { name: /偏好 · 1 项/ })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "开始规划" }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const request = onSubmit.mock.calls[0][0];
    expect(request.city).toBe("beijing");
    expect(request.input).toContain("北京两天，想看历史");
    expect(request.input).toContain("共 2 天");
    expect(request.client_request_id).toMatch(/^[0-9a-z-]{8,}$/);
    expect(onCreated).toHaveBeenCalledWith(created);
  });

  it("reuses the request key and keeps the input when retrying without edits", async () => {
    const onSubmit = vi
      .fn()
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce(created);
    render(<CreateForm onSubmit={onSubmit} />);

    await userEvent.type(screen.getByLabelText("行程需求"), "北京一天");
    const submit = screen.getByRole("button", { name: "开始规划" });
    await userEvent.click(submit);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    expect(await screen.findByRole("alert")).toHaveTextContent("提交失败");
    expect(screen.getByLabelText("行程需求")).toHaveValue("北京一天");

    await userEvent.click(submit);
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(2));

    expect(onSubmit.mock.calls[0][0].client_request_id).toBe(onSubmit.mock.calls[1][0].client_request_id);
  });

  it("fills the input from an example", async () => {
    render(<CreateForm onSubmit={vi.fn().mockResolvedValue(created)} />);

    await userEvent.click(screen.getByRole("button", { name: /带爸妈/ }));

    expect((screen.getByLabelText("行程需求") as HTMLTextAreaElement).value).toContain("带爸妈");
  });

  it("appends preferences edited in the dialog", async () => {
    const onSubmit = vi.fn().mockResolvedValue(created);
    render(<CreateForm onSubmit={onSubmit} />);

    await userEvent.type(screen.getByLabelText("行程需求"), "北京两天");
    const dialog = await openPreferences();
    await userEvent.type(within(dialog).getByLabelText("总预算"), "1000");
    await userEvent.selectOptions(within(dialog).getByLabelText("节奏"), "relaxed");
    await userEvent.click(within(dialog).getByLabelText("安排午餐和晚餐"));
    await userEvent.click(within(dialog).getByRole("button", { name: "完成" }));
    await userEvent.click(screen.getByRole("button", { name: "开始规划" }));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const input = onSubmit.mock.calls[0][0].input as string;
    expect(input).toContain("节奏轻松");
    expect(input).toContain("总预算 1000 元");
    expect(input).toContain("需要安排午餐和晚餐");
  });

  it("keeps preferences when closing and resets them on clear", async () => {
    render(<CreateForm onSubmit={vi.fn().mockResolvedValue(created)} />);

    let dialog = await openPreferences();
    await userEvent.selectOptions(within(dialog).getByLabelText("天数"), "3");
    await userEvent.click(within(dialog).getByRole("button", { name: "完成" }));

    dialog = await openPreferences();
    expect(within(dialog).getByLabelText("天数")).toHaveValue("3");
    await userEvent.click(within(dialog).getByRole("button", { name: "清空偏好" }));
    expect(within(dialog).getByLabelText("天数")).toHaveValue("");
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
