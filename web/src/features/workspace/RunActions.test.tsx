import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import * as api from "@/api/trips";
import { RunActions } from "./RunActions";

vi.mock("@/api/trips", () => ({
  resumeRun: vi.fn(),
  createAdjustment: vi.fn(),
}));

const resumeRun = vi.mocked(api.resumeRun);
const createAdjustment = vi.mocked(api.createAdjustment);

function renderActions(overrides: Partial<ComponentProps<typeof RunActions>> = {}) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const props: ComponentProps<typeof RunActions> = {
    tripID: "trip-1",
    runID: "run-1",
    status: "awaiting_user",
    missing: [],
    proposal: null,
    baseVersion: 3,
    onRunStarted: vi.fn(),
    onActionComplete: vi.fn(),
    ...overrides,
  };
  render(
    <QueryClientProvider client={client}>
      <RunActions {...props} />
    </QueryClientProvider>,
  );
  return props;
}

describe("RunActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resumeRun.mockResolvedValue({ run_id: "run-1", status: "queued" });
    createAdjustment.mockResolvedValue({ run_id: "run-2", status: "queued" });
  });

  it("supplements missing constraints", async () => {
    const props = renderActions({ missing: ["days"] });
    expect(screen.getByText("还差一点信息")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "补充并继续" }));

    await waitFor(() =>
      expect(resumeRun).toHaveBeenCalledWith("run-1", { action: "provide_constraints", input: "共 2 天" }),
    );
    await waitFor(() => expect(props.onActionComplete).toHaveBeenCalled());
  });

  it("accepts a pending proposal", async () => {
    const props = renderActions({
      proposal: { summary: "第二天改为博物馆", impact: "费用 200→220 元", warnings: ["周一闭馆"] },
    });

    expect(screen.getByText("调整提案")).toBeInTheDocument();
    expect(screen.getByText("第二天改为博物馆")).toBeInTheDocument();
    expect(screen.getByText("周一闭馆")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "接受调整" }));
    await waitFor(() => expect(resumeRun).toHaveBeenCalledWith("run-1", { action: "accept" }));
    await waitFor(() => expect(props.onActionComplete).toHaveBeenCalled());
  });

  it("accepts the initial plan when nothing is missing", async () => {
    renderActions();
    expect(screen.getByText("等待你的确认")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "接受方案" }));
    await waitFor(() => expect(resumeRun).toHaveBeenCalledWith("run-1", { action: "accept" }));
  });

  it("submits an adjustment and follows the new run", async () => {
    const props = renderActions({ status: "completed" });

    await userEvent.type(screen.getByLabelText("调整需求"), "第二天下午轻松一点");
    await userEvent.click(screen.getByRole("button", { name: "提交调整" }));

    await waitFor(() =>
      expect(createAdjustment).toHaveBeenCalledWith("trip-1", {
        input: "第二天下午轻松一点",
        base_version: 3,
      }),
    );
    await waitFor(() => expect(props.onRunStarted).toHaveBeenCalledWith("run-2"));
  });

  it("resolves a hotel question with server options", async () => {
    const props = renderActions({
      researchQuestion: {
        code: "hotel_unconfirmed",
        message: "未找到指定酒店",
        options: [
          {
            id: "hotel-1",
            label: "全季酒店",
            hotel: { name: "全季酒店", price_per_night_yuan: 480, booking_url: "https://example.com/book" },
          },
          { id: "continue_without_lodging", label: "继续但不计住宿" },
        ],
      },
    });

    expect(screen.getByText("确认住宿")).toBeInTheDocument();
    expect(screen.getAllByText("全季酒店").length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole("button", { name: "选择 继续但不计住宿" }));
    await waitFor(() =>
      expect(resumeRun).toHaveBeenCalledWith("run-1", {
        action: "resolve_research",
        research_choice: { option_id: "continue_without_lodging" },
      }),
    );
    await waitFor(() => expect(props.onActionComplete).toHaveBeenCalled());
  });

  it("passes the selected server hotel through unchanged", async () => {
    const hotel = { name: "全季酒店", lat: 39.8, lng: 116.3, price_per_night_yuan: 480 };
    renderActions({
      researchQuestion: {
        code: "hotel_unconfirmed",
        message: "未找到指定酒店",
        options: [{ id: "hotel-1", label: "全季酒店", hotel }],
      },
    });

    await userEvent.click(screen.getByRole("button", { name: "选择 全季酒店" }));

    await waitFor(() =>
      expect(resumeRun).toHaveBeenCalledWith("run-1", {
        action: "resolve_research",
        research_choice: { option_id: "hotel-1", hotel },
      }),
    );
  });

  it("renders nothing while the agent is still working", () => {
    const { container } = render(<RunActions status="running" tripID="trip-1" runID="run-1" missing={[]} proposal={null} baseVersion={3} onRunStarted={vi.fn()} onActionComplete={vi.fn()} />, {
      wrapper: ({ children }) => <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>,
    });
    expect(container).toBeEmptyDOMElement();
  });
});
