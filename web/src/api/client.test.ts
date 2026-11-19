import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiRequest } from "./client";

function jsonResponse(body: unknown, status = 200, requestId = "server-request-id"): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", "X-Request-ID": requestId },
  });
}

describe("apiRequest", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("sends a request id and parses JSON", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ status: "ok" }));
    vi.stubGlobal("fetch", fetchMock);

    const data = await apiRequest<{ status: string }>("/api/meta");

    expect(data).toEqual({ status: "ok" });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/meta");
    expect(new Headers(init.headers).get("X-Request-ID")).toMatch(/^[0-9a-z-]+$/);
  });

  it("serializes JSON bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }, 202));
    vi.stubGlobal("fetch", fetchMock);

    await apiRequest("/api/trips", { method: "POST", body: { input: "北京一天" } });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ input: "北京一天" }));
    expect(new Headers(init.headers).get("Content-Type")).toBe("application/json");
  });

  it("raises the unified error envelope", async () => {
    const envelope = {
      error: { code: "invalid_request", message: "输入不合法", request_id: "server-request-id" },
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(envelope, 400)));

    const error = await apiRequest("/api/trips").catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(ApiError);
    const apiError = error as ApiError;
    expect(apiError.status).toBe(400);
    expect(apiError.code).toBe("invalid_request");
    expect(apiError.message).toBe("输入不合法");
    expect(apiError.requestId).toBe("server-request-id");
  });

  it("falls back when the error body is not JSON", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("boom", { status: 502, headers: { "X-Request-ID": "rid" } })),
    );

    const error = (await apiRequest("/api/trips").catch((cause: unknown) => cause)) as ApiError;

    expect(error).toBeInstanceOf(ApiError);
    expect(error.code).toBe("request_failed");
    expect(error.message).toBe("请求失败（502）");
    expect(error.requestId).toBe("rid");
  });

  it("returns undefined for empty responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));

    await expect(apiRequest("/api/trips/x", { method: "DELETE" })).resolves.toBeUndefined();
  });
});
