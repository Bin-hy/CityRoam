import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useMeta } from "./useMeta";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

describe("useMeta", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads service metadata", async () => {
    const meta = {
      version: "0.1.0",
      cities: [{ id: "beijing", name: "北京" }],
      defaults: { days: 2, pace: "normal", currency: "CNY", budget_level: 4 },
      limits: { input_max_chars: 2000, rate_limit_per_hour: 10 },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(meta), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const { result } = renderHook(() => useMeta(), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(meta);
  });
});
