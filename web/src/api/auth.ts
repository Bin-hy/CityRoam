import { API_BASE, apiRequest } from "./client";
import type { AuthLogout, AuthMe } from "./types";

export function getMe(): Promise<AuthMe> {
  return apiRequest<AuthMe>("/api/auth/me");
}

export function logout(): Promise<AuthLogout | void> {
  return apiRequest<AuthLogout | void>("/api/auth/logout", { method: "POST" });
}

export function loginURL(returnTo = "/"): string {
  return `${API_BASE}/api/auth/login?return_to=${encodeURIComponent(returnTo)}`;
}