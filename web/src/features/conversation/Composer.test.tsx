import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Composer, type ComposerMode } from "./Composer";
import { ToolCard } from "./ToolCard";
import { DecisionCard } from "./DecisionCard";
import { PlanCard } from "./PlanCard";
import type { DecisionMessage, PlanMessage, ToolMessage } from "./deriveConversation";

function renderComposer(mode: ComposerMode, overrides: Partial<React.ComponentProps<typeof Composer>> = {}) {
  const onSubmit = vi.fn(async () => true);
  const onStop = vi.fn();
  render(<Composer mode={mode} onSubmit={onSubmit} onStop={onStop} {...overrides} />);
  return { onSubmit, onStop };
}

describe("Composer modes", () => {
  it("keeps editable modes enabled with the fixed send label", () => {
    const modes: ComposerMode[] = [
      { kind: "initial" },
      { kind: "supplement", missing: ["city", "days"] },
      { kind: "revise", rejected: true },
      { kind: "adjust", baseVersion: 2 },
    ];
    for (const mode of modes) {
      const { unmount } = render(
        <Composer mode={mode} onSubmit={vi.fn(async () => true)} />,
      );
      expect(screen.getByLabelText("行程输入")).toBeEnabled();
      expect(screen.getByRole("button", { name: "发送" })).toBeInTheDocument();
      unmount();
    }
  });

  it("locks the input while a research choice is open", () => {
    renderComposer({ kind: "locked", reason: "research_choice" });
    expect(screen.getByLabelText("行程输入")).toBeDisabled();
    expect(screen.getByRole("button", { name: "发送" })).toBeDisabled();
    expect(screen.getByText(/请在上方选择住宿/)).toBeInTheDocument();
  });

  it("shows the running stage and a stop button", async () => {
    const { onStop } = renderComposer({ kind: "running" }, { stage: "research" });
    expect(screen.getByRole("status")).toHaveTextContent("正在研究攻略与住宿…");
    expect(screen.queryByLabelText("行程输入")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "停止" }));
    expect(onStop).toHaveBeenCalled();
  });

  it("disables with a reason", () => {
    renderComposer({ kind: "disabled", reason: "运行已完成，请新建行程。" });
    expect(screen.getByLabelText("行程输入")).toBeDisabled();
    expect(screen.getByText("运行已完成，请新建行程。")).toBeInTheDocument();
  });
});

describe("Composer input behaviour", () => {
  it("sends on Enter and clears after success", async () => {
    const { onSubmit } = renderComposer({ kind: "initial" });
    const input = screen.getByLabelText("行程输入");
    await userEvent.type(input, "杭州 2 天");
    await userEvent.keyboard("{Enter}");
    expect(onSubmit).toHaveBeenCalledWith("杭州 2 天");
    expect(input).toHaveValue("");
  });

  it("inserts a newline on Shift+Enter and does not send", async () => {
    const { onSubmit } = renderComposer({ kind: "initial" });
    const input = screen.getByLabelText("行程输入");
    await userEvent.type(input, "第一行");
    await userEvent.keyboard("{Shift>}{Enter}{/Shift}");
    await userEvent.type(input, "第二行");
    expect(input).toHaveValue("第一行\n第二行");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("does not send empty input or while pending", async () => {
    const { onSubmit } = renderComposer({ kind: "initial" }, { pending: "adjust" });
    const input = screen.getByLabelText("行程输入");
    expect(screen.getByRole("button", { name: "发送中…" })).toBeDisabled();
    await userEvent.type(input, "杭州");
    await userEvent.keyboard("{Enter}");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("keeps the input when submit fails", async () => {
    const onSubmit = vi.fn(async () => false);
    render(<Composer mode={{ kind: "adjust", baseVersion: 1 }} onSubmit={onSubmit} />);
    const input = screen.getByLabelText("行程输入");
    await userEvent.type(input, "第二天轻松点");
    await userEvent.keyboard("{Enter}");
    expect(input).toHaveValue("第二天轻松点");
  });

  it("applies prefill and focuses on token change", () => {
    const { rerender } = render(
      <Composer mode={{ kind: "adjust", baseVersion: 1 }} onSubmit={vi.fn(async () => true)} prefill="调整第 2 天：" focusToken={1} />,
    );
    const input = screen.getByLabelText("行程输入");
    expect(input).toHaveValue("调整第 2 天：");
    expect(input).toHaveFocus();
    rerender(
      <Composer mode={{ kind: "adjust", baseVersion: 1 }} onSubmit={vi.fn(async () => true)} prefill="调整第 3 天：" focusToken={2} />,
    );
    expect(input).toHaveValue("调整第 3 天：");
  });
});

const tool: ToolMessage = {
  kind: "tool",
  id: "run-1:6:tool:get_weather:1",
  seq: 6,
  callKey: "get_weather:1",
  name: "get_weather",
  provider: "amap",
  query: "杭州 · 11/21 起 · 2 天",
  status: "ok",
  latencyMs: 120,
  resultCount: 2,
  preview: [
    { title: "11/21", subtitle: "晴 12–20°C" },
    { title: "11/22", subtitle: "多云 10–18°C" },
  ],
};

describe("ToolCard", () => {
  it("stays collapsed until expanded", async () => {
    render(<ToolCard message={tool} />);
    const trigger = screen.getByRole("button", { name: /天气查询/ });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("11/21")).not.toBeInTheDocument();
    await userEvent.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("11/21")).toBeInTheDocument();
    expect(screen.queryAllByText(/\/晚/)).toHaveLength(0);
  });

  it("shows failure messaging", async () => {
    render(
      <ToolCard
        message={{
          ...tool,
          status: "error",
          resultCount: undefined,
          preview: [],
          error: { code: "tool_failed", message: "外部工具调用失败，不影响继续。" },
        }}
      />,
    );
    expect(screen.getByText("失败")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /天气查询/ }));
    expect(screen.getByText("外部工具调用失败，不影响继续。")).toBeInTheDocument();
  });
});

const baseDecision: DecisionMessage = {
  kind: "decision",
  id: "run-1:2:decision:missing",
  seq: 2,
  state: "open",
  payload: { decision: "missing", missing: ["city", "days"] },
};

function renderDecision(message: DecisionMessage, overrides: Partial<React.ComponentProps<typeof DecisionCard>> = {}) {
  const callbacks = {
    onSupplement: vi.fn(),
    onSelectResearch: vi.fn(),
    onAccept: vi.fn(),
    onReject: vi.fn(),
    onCancel: vi.fn(),
  };
  render(<DecisionCard message={message} {...callbacks} {...overrides} />);
  return callbacks;
}

describe("DecisionCard", () => {
  it("submits missing fields", async () => {
    const callbacks = renderDecision(baseDecision);
    await userEvent.click(screen.getByRole("button", { name: "补充并继续" }));
    expect(callbacks.onSupplement).toHaveBeenCalledWith("城市是北京，共 2 天");
  });

  it("collapses a resolved card into a summary", () => {
    renderDecision({ ...baseDecision, state: "resolved", summary: "已补充信息。" });
    expect(screen.getByText("已补充信息。")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("selects a lodging option with a safe external link", async () => {
    const callbacks = renderDecision({
      kind: "decision",
      id: "run-1:3:decision:lodging",
      seq: 3,
      state: "open",
      payload: {
        decision: "lodging",
        question: {
          code: "lodging_choice",
          message: "请选择住宿",
          options: [
            {
              id: "o1",
              label: "全季西湖店",
              hotel: {
                name: "全季西湖店",
                address: "西湖区",
                price_per_night_yuan: 480,
                score: 8.8,
                booking_url: "https://example.com/hotel",
              },
            },
          ],
        },
      },
    });
    const link = screen.getByRole("link", { name: "查看预订" });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    await userEvent.click(screen.getByRole("button", { name: "选择 全季西湖店" }));
    expect(callbacks.onSelectResearch).toHaveBeenCalledWith({
      option_id: "o1",
      hotel: expect.objectContaining({ name: "全季西湖店" }),
    });
  });

  it("accepts and rejects proposals with pending labels", async () => {
    const callbacks = renderDecision(
      {
        kind: "decision",
        id: "run-1:4:decision:proposal",
        seq: 4,
        state: "open",
        payload: {
          decision: "proposal",
          proposal: {
            baseVersion: 1,
            scope: "day",
            dayIndex: 1,
            summary: "把灵隐寺挪到上午",
            impact: "步行减少 1.2km",
            warnings: ["预约时间需确认"],
          },
        },
      },
      { pending: "accept" },
    );
    expect(screen.getByText("把灵隐寺挪到上午")).toBeInTheDocument();
    expect(screen.getByText(/保留当前版本/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "接受中…" })).toBeDisabled();
    expect(callbacks.onAccept).not.toHaveBeenCalled();
  });

  it("renders confirm actions", async () => {
    const callbacks = renderDecision({
      kind: "decision",
      id: "run-1:5:decision:confirm",
      seq: 5,
      state: "open",
      payload: { decision: "confirm" },
    });
    await userEvent.click(screen.getByRole("button", { name: "接受方案" }));
    await userEvent.click(screen.getByRole("button", { name: "拒绝" }));
    await userEvent.click(screen.getByRole("button", { name: "取消" }));
    expect(callbacks.onAccept).toHaveBeenCalled();
    expect(callbacks.onReject).toHaveBeenCalled();
    expect(callbacks.onCancel).toHaveBeenCalled();
  });
});

const planMessage: PlanMessage = {
  kind: "plan",
  id: "run-1:10:plan",
  seq: 10,
  versionSeq: 3,
  stats: { days: 2, pois: 5, totalYuan: 1280, walkingKm: 4.2 },
  isDraft: true,
};

describe("PlanCard", () => {
  it("marks drafts and opens the canvas", async () => {
    const onOpenCanvas = vi.fn();
    render(<PlanCard message={planMessage} onOpenCanvas={onOpenCanvas} />);
    expect(screen.getByText("草稿预览 · 待确认")).toBeInTheDocument();
    expect(screen.getByText("¥1,280")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "查看方案" }));
    expect(onOpenCanvas).toHaveBeenCalled();
  });

  it("shows the version for a settled plan", () => {
    render(<PlanCard message={{ ...planMessage, isDraft: false }} />);
    expect(screen.getByText("版本 v3")).toBeInTheDocument();
  });
});
