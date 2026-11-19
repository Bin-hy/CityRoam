import { apiRequest } from "./client";
import type { Meta } from "./types";

export function getMeta(): Promise<Meta> {
  return apiRequest<Meta>("/api/meta");
}
