import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { pathToFileURL } from "node:url";

const MAX_BODY = 64 * 1024;
const MAX_OUTPUT = 256 * 1024;
const DEFAULT_LIMIT = 5;
const sorts = new Set(["price_asc", "price_desc", "rate_desc", "distance_asc", "no_rank"]);

export function configFromEnv(overrides = {}) {
  return {
    port: Number(process.env.PORT || 8090),
    token: process.env.HOTEL_TOOL_TOKEN || "",
    flyaiBin: process.env.FLYAI_BIN || "flyai",
    cliTimeoutMs: Number(process.env.FLYAI_TIMEOUT_MS || 9000),
    healthTimeoutMs: Number(process.env.FLYAI_HEALTH_TIMEOUT_MS || 1500),
    ...overrides,
  };
}

export function createHotelServer(overrides = {}) {
  const config = configFromEnv(overrides);
  return createServer((req, res) => {
    handle(req, res, config).catch(() => {
      respond(res, 500, { error: { code: "internal_error", message: "sidecar failed" } });
    });
  });
}

async function handle(req, res, config) {
  const url = new URL(req.url ?? "/", "http://hotel-tool");
  if (req.method === "GET" && url.pathname === "/healthz") {
    const ok = await probe(config);
    respond(res, ok ? 200 : 503, ok ? { status: "ok", flyai: "available" } : { status: "unavailable" });
    return;
  }
  if (req.method !== "POST" || url.pathname !== "/search") {
    respond(res, 404, { error: { code: "not_found", message: "route not found" } });
    return;
  }
  if (!authorized(req, config.token)) {
    respond(res, 401, { error: { code: "unauthorized", message: "invalid bearer token" } });
    return;
  }
  if (!isJSON(req.headers["content-type"])) {
    respond(res, 415, { error: { code: "unsupported_media_type", message: "content type must be application/json" } });
    return;
  }

  let body;
  try {
    body = await readJSON(req);
  } catch (error) {
    const status = error?.status ?? 400;
    respond(res, status, { error: { code: "invalid_request", message: error?.message ?? "invalid request" } });
    return;
  }

  const parsed = parseSearch(body);
  if (parsed.error) {
    respond(res, 400, { error: { code: "invalid_request", message: parsed.error } });
    return;
  }

  let result;
  try {
    result = await runSearch(config, parsed.value);
  } catch (error) {
    if (error?.timeout) {
      respond(res, 504, { error: { code: "upstream_timeout", message: "hotel search timed out" } });
      return;
    }
    respond(res, 502, { error: { code: "upstream_failure", message: "hotel search failed" } });
    return;
  }

  respond(res, 200, {
    provider: "fliggy",
    fetched_at: new Date().toISOString(),
    hotels: result,
  });
}

async function probe(config) {
  return new Promise((resolve) => {
    const child = spawn(config.flyaiBin, ["--help"], { shell: false, stdio: ["ignore", "ignore", "ignore"] });
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      resolve(false);
    }, config.healthTimeoutMs);
    child.on("error", () => {
      clearTimeout(timer);
      resolve(false);
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      resolve(code === 0);
    });
  });
}

function authorized(req, token) {
  if (!token) return true;
  return req.headers.authorization === `Bearer ${token}`;
}

function isJSON(value) {
  return typeof value === "string" && value.split(";")[0].trim().toLowerCase() === "application/json";
}

function readJSON(req) {
  return new Promise((resolve, reject) => {
    let size = 0;
    let rejected = false;
    const chunks = [];
    req.on("data", (chunk) => {
      if (rejected) return;
      size += chunk.length;
      if (size > MAX_BODY) {
        rejected = true;
        const error = new Error("request body too large");
        error.status = 413;
        reject(error);
        return;
      }
      chunks.push(chunk);
    });
    req.on("end", () => {
      if (rejected) return;
      try {
        const parsed = JSON.parse(Buffer.concat(chunks).toString("utf8"));
        if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
          throw new Error("request body must be a JSON object");
        }
        resolve(parsed);
      } catch (error) {
        reject(error);
      }
    });
    req.on("error", reject);
  });
}

function parseSearch(body) {
  const city = text(body.city);
  if (!city) return { error: "city is required" };
  const checkIn = date(body.check_in);
  const checkOut = date(body.check_out);
  if (!checkIn) return { error: "check_in must use YYYY-MM-DD" };
  if (!checkOut) return { error: "check_out must use YYYY-MM-DD" };
  if (checkOut <= checkIn) return { error: "check_out must be after check_in" };
  const keyword = optionalText(body.keyword, 60);
  if (keyword === null) return { error: "keyword is too long" };
  const near = optionalText(body.near, 60);
  if (near === null) return { error: "near is too long" };
  const minStar = optionalInt(body.min_star, 1, 5);
  if (minStar === null) return { error: "min_star must be between 1 and 5" };
  const maxPrice = optionalInt(body.max_price, 0, 1000000);
  if (maxPrice === null) return { error: "max_price must be non-negative" };
  const limit = optionalInt(body.limit, 1, 10) ?? DEFAULT_LIMIT;
  if (body.sort !== undefined && !sorts.has(String(body.sort))) {
    return { error: "sort is not supported" };
  }
  return {
    value: {
      city,
      checkIn,
      checkOut,
      keyword: keyword || "",
      near: near || "",
      minStar: minStar ?? 0,
      maxPrice: maxPrice ?? 0,
      sort: body.sort ? String(body.sort) : "",
      limit,
    },
  };
}

function date(value) {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return "";
  const parsed = new Date(`${value}T00:00:00Z`);
  return Number.isNaN(parsed.getTime()) ? "" : value;
}

function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

function optionalText(value, max) {
  if (value === undefined) return "";
  const parsed = text(value);
  return Array.from(parsed).length > max ? null : parsed;
}

function optionalInt(value, min, max) {
  if (value === undefined) return undefined;
  if (typeof value !== "number" || !Number.isInteger(value) || value < min || value > max) return null;
  return value;
}

function runSearch(config, input) {
  const args = [
    "search-hotel",
    "--dest-name",
    input.city,
    "--check-in-date",
    input.checkIn,
    "--check-out-date",
    input.checkOut,
  ];
  if (input.keyword) args.push("--key-words", input.keyword);
  if (input.near) args.push("--poi-name", input.near);
  if (input.minStar) args.push("--hotel-stars", String(input.minStar));
  if (input.maxPrice > 0) args.push("--max-price", String(input.maxPrice));
  if (input.sort) args.push("--sort", input.sort);

  config.onCall?.(args);

  return new Promise((resolve, reject) => {
    const child = spawn(config.flyaiBin, args, {
      shell: false,
      stdio: ["ignore", "pipe", "pipe"],
      env: process.env,
    });
    const stdout = [];
    let outputSize = 0;
    let stderrSize = 0;
    let settled = false;
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill("SIGKILL");
      reject(Object.assign(new Error("timeout"), { timeout: true }));
    }, config.cliTimeoutMs);

    child.stdout.on("data", (chunk) => {
      if (outputSize >= MAX_OUTPUT) return;
      outputSize += chunk.length;
      stdout.push(chunk);
    });
    child.stderr.on("data", (chunk) => {
      stderrSize = Math.min(stderrSize + chunk.length, MAX_OUTPUT);
    });
    child.on("error", () => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      reject(new Error("spawn failed"));
    });
    child.on("close", (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (code !== 0) {
        reject(Object.assign(new Error(`exit ${code}`), { stderrBytes: stderrSize }));
        return;
      }
      try {
        const payload = parsePayload(Buffer.concat(stdout).toString("utf8"));
        resolve(normalizeHotels(payload, input));
      } catch {
        reject(new Error("invalid cli output"));
      }
    });
  });
}

function parsePayload(output) {
  const lines = output
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  for (let index = lines.length - 1; index >= 0; index--) {
    try {
      const parsed = JSON.parse(lines[index]);
      if (parsed && typeof parsed === "object") return parsed;
    } catch {
      // Keep looking for the JSON line.
    }
  }
  throw new Error("no json output");
}

function normalizeHotels(payload, input) {
  const data = payload.data && typeof payload.data === "object" ? payload.data : payload;
  const items = Array.isArray(data.itemList) ? data.itemList : Array.isArray(data.items) ? data.items : [];
  const hotels = [];
  for (const item of items) {
    if (!item || typeof item !== "object") continue;
    const lat = number(pick(item, ["latitude", "lat"]));
    const lng = number(pick(item, ["longitude", "lng"]));
    if (!Number.isFinite(lat) || !Number.isFinite(lng) || lat === 0 || lng === 0) continue;
    const name = text(pick(item, ["hotelName", "name", "title"]));
    if (!name) continue;
    const price = priceValue(pick(item, ["price", "pricePerNight", "avgPrice", "lowPrice"]));
    hotels.push({
      id: text(pick(item, ["hotelId", "shId", "id", "shangriLaId"])),
      name,
      address: text(pick(item, ["address", "hotelAddress"])),
      city: text(pick(item, ["city", "cityName", "destName"])) || input.city,
      lat,
      lng,
      star: integer(pick(item, ["star", "starRating", "hotelStar"])),
      score: number(pick(item, ["score", "commentScore", "rating"])) || null,
      price_per_night_yuan: price.value,
      price_estimated: price.estimated,
      booking_url: httpURL(pick(item, ["bookingUrl", "detailUrl", "url"])),
      provider: "fliggy",
    });
    if (hotels.length >= input.limit) break;
  }
  return hotels;
}

function pick(item, keys) {
  for (const key of keys) {
    if (item[key] !== undefined && item[key] !== null && item[key] !== "") return item[key];
  }
  return undefined;
}

function number(value) {
  const parsed = typeof value === "number" ? value : Number.parseFloat(String(value ?? "").replace(/[^\d.-]/g, ""));
  return Number.isFinite(parsed) ? parsed : Number.NaN;
}

function integer(value) {
  const parsed = number(value);
  return Number.isFinite(parsed) ? Math.max(0, Math.trunc(parsed)) : 0;
}

function priceValue(value) {
  if (typeof value === "object" && value !== null) {
    value = pick(value, ["amount", "price", "value"]);
  }
  if (value === undefined || value === null || value === "") return { value: 0, estimated: true };
  if (typeof value !== "string") {
    const parsed = number(value);
    return { value: Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0, estimated: false };
  }
  const normalized = value.replace(/,/g, "").trim();
  const exact = normalized.match(/\d+(?:\.\d+)?/);
  if (!exact) return { value: 0, estimated: false };
  const range = normalized.match(/(\d+)\s*[xX]/);
  if (range) {
    return { value: Number.parseInt(range[1], 10) * 10 ** (normalized.match(/[xX]+/)?.[0].length ?? 1), estimated: true };
  }
  const parsed = Number.parseFloat(exact[0]);
  return { value: Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0, estimated: false };
}

function httpURL(value) {
  if (typeof value !== "string") return "";
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:" ? parsed.toString() : "";
  } catch {
    return "";
  }
}

function respond(res, status, body) {
  if (res.headersSent) return;
  const encoded = JSON.stringify(body);
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Content-Length": Buffer.byteLength(encoded),
  });
  res.end(encoded);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const server = createHotelServer();
  server.listen(Number(process.env.PORT || 8090), () => {
    console.log(`hotel-tool listening on ${server.address().port}`);
  });
}
