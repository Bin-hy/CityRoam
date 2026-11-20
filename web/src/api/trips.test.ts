import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "./client";
import { cancelRun, replanTrip, resumeRun } from "./trips";

function jsonResponse(body: unknown, status = 202): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", "X-Request-ID": "rid" },
  });
}

describe("trip actions", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts a replan with full constraints", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ run_id: "run-2", status: "queued" }));
    vi.stubGlobal("fetch", fetchMock);

    await replanTrip("trip-1", {
      base_version: 2,
      constraints: { city: "beijing", days: 2, budget: { total_yuan: 2000, include_lodging: false } },
      client_request_id: "req-1",
    });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/trips/trip-1/replan");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toMatchObject({
      base_version: 2,
      constraints: { city: "beijing", days: 2, budget: { include_lodging: false } },
      client_request_id: "req-1",
    });
  });

  it("sends research choices through resume", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ run_id: "run-1", status: "queued" }));
    vi.stubGlobal("fetch", fetchMock);

    await resumeRun("run-1", {
      action: "resolve_research",
      research_choice: { option_id: "hotel-1", hotel: { name: "全季酒店", lat: 39.8, lng: 116.3 } },
    });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/runs/run-1/resume");
    expect(JSON.parse(init.body as string)).toMatchObject({
      action: "resolve_research",
      research_choice: { option_id: "hotel-1", hotel: { name: "全季酒店" } },
    });
  });

  it("sends revise feedback through resume", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ run_id: "run-1", status: "queued" }));
    vi.stubGlobal("fetch", fetchMock);

    await resumeRun("run-1", { action: "revise", input: "第二天不要安排爬山" });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/runs/run-1/resume");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      action: "revise",
      input: "第二天不要安排爬山",
    });
  });

  it("cancels a run via POST", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ run_id: "run 9", status: "cancelled" }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await cancelRun("run 9");

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/runs/run%209/cancel");
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();
    expect(result).toEqual({ run_id: "run 9", status: "cancelled" });
  });

  it("maps error envelopes to ApiError", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ error: { code: "invalid_input", message: "缺少修改建议", request_id: "rid-9" } }),
        { status: 400, headers: { "Content-Type": "application/json", "X-Request-ID": "rid-9" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const attempt = resumeRun("run-1", { action: "revise", input: "" });
    await expect(attempt).rejects.toMatchObject({
      name: "ApiError",
      status: 400,
      code: "invalid_input",
      message: "缺少修改建议",
      requestId: "rid-9",
    });
    await expect(attempt).rejects.toBeInstanceOf(ApiError);
  });
});
