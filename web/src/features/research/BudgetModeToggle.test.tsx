import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import * as api from "@/api/trips";
import { BudgetModeToggle } from "./BudgetModeToggle";

vi.mock("@/api/trips", () => ({ replanTrip: vi.fn() }));

const replanTrip = vi.mocked(api.replanTrip);

function renderToggle(overrides: Partial<Parameters<typeof BudgetModeToggle>[0]> = {}) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const props = {
    tripID: "trip-1",
    baseVersion: 2,
    constraints: { city: "beijing", days: 2, budget: { total_yuan: 2000, hard: false } },
    lodgingMentioned: true,
    hasLodging: true,
    includeLodging: true,
    onRunStarted: vi.fn(),
    onReplanned: vi.fn(),
    ...overrides,
  };
  render(
    <QueryClientProvider client={client}>
      <BudgetModeToggle {...props} />
    </QueryClientProvider>,
  );
  return props;
}

describe("BudgetModeToggle", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    replanTrip.mockResolvedValue({ run_id: "run-9", status: "queued" });
  });

  it("replans with the toggled budget scope", async () => {
    const props = renderToggle();
    const toggle = screen.getByRole("switch", { name: "住宿计入预算" });
    expect(toggle).toHaveAttribute("aria-checked", "true");

    await userEvent.click(toggle);

    await waitFor(() => expect(replanTrip).toHaveBeenCalledTimes(1));
    const [tripID, body] = replanTrip.mock.calls[0];
    expect(tripID).toBe("trip-1");
    expect(body.base_version).toBe(2);
    expect(body.client_request_id).toBeTruthy();
    expect(body.constraints).toMatchObject({
      city: "beijing",
      days: 2,
      budget: { total_yuan: 2000, hard: false, include_lodging: false },
    });
    await waitFor(() => expect(props.onRunStarted).toHaveBeenCalledWith("run-9"));
    expect(props.onReplanned).toHaveBeenCalled();
  });

  it("rolls back and reports failures", async () => {
    replanTrip.mockRejectedValue(new ApiError(409, "request_id_conflict", "幂等键已被使用", "req-1"));
    renderToggle();

    await userEvent.click(screen.getByRole("switch", { name: "住宿计入预算" }));

    expect(await screen.findByText("幂等键已被使用")).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "住宿计入预算" })).toHaveAttribute("aria-checked", "true");
  });

  it("stays hidden without any lodging context", () => {
    const client = new QueryClient();
    const { container } = render(
      <QueryClientProvider client={client}>
        <BudgetModeToggle
          tripID="trip-1"
          baseVersion={2}
          constraints={{ city: "beijing", days: 2 }}
          lodgingMentioned={false}
          hasLodging={false}
          includeLodging
          onRunStarted={vi.fn()}
        />
      </QueryClientProvider>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
