export function newClientRequestID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `req-${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`;
}
