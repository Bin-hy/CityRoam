import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import * as api from "@/api/trips";
import { ShareDialog } from "./ShareDialog";

vi.mock("@/api/trips", () => ({ createShare: vi.fn() }));

const createShare = vi.mocked(api.createShare);
const writeText = vi.fn();

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ShareDialog tripID="trip-1" />
    </QueryClientProvider>,
  );
}

describe("ShareDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    writeText.mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    createShare.mockResolvedValue({
      slug: "abc123",
      url: "/api/share/abc123",
      trip_id: "trip-1",
      version_seq: 2,
      created_at: "2026-11-19T10:00:00Z",
    });
  });

  it("creates the link on open and copies it", async () => {
    renderDialog();
    await userEvent.click(screen.getByRole("button", { name: "分享" }));

    expect(await screen.findByText("分享行程")).toBeInTheDocument();
    expect(createShare).toHaveBeenCalledWith("trip-1");
    const expected = new URL("/api/share/abc123", window.location.origin).toString();
    expect(screen.getByLabelText("分享链接")).toHaveValue(expected);

    await userEvent.click(screen.getByRole("button", { name: "复制链接" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(expected));
    expect(screen.getByRole("button", { name: "已复制" })).toBeInTheDocument();
  });

  it("explains when there is no accepted version to share", async () => {
    createShare.mockRejectedValue(new ApiError(409, "no_final_version", "行程尚未确认，不能分享", "req-1"));
    renderDialog();
    await userEvent.click(screen.getByRole("button", { name: "分享" }));

    expect(await screen.findByText("行程尚未确认，不能分享")).toBeInTheDocument();
  });
});
