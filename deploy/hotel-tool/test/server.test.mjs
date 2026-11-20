import assert from "node:assert/strict";
import { chmodSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, before, test } from "node:test";

import { createHotelServer } from "../server.mjs";

const fakeScript = `#!/usr/bin/env node
const args = process.argv.slice(2);
if (args.includes("--help")) { console.log("help"); process.exit(0); }
if (process.env.FAKE_MODE === "timeout") { setTimeout(() => {}, 60000); }
if (process.env.FAKE_MODE === "fail") { console.error("secret upstream detail"); process.exit(3); }
if (process.env.FAKE_MODE === "garbage") { console.log("not json"); process.exit(0); }
const payload = {
  data: {
    itemList: [
      {
        hotelId: "h1",
        hotelName: "近胡同酒店",
        address: "北京市东城区",
        city: "北京",
        latitude: 39.91,
        longitude: 116.4,
        star: 4,
        commentScore: "4.8",
        price: "¥520",
        detailUrl: "https://example.com/h1"
      },
      { hotelId: "h2", hotelName: "没有坐标", latitude: 0, longitude: 0, price: 300 },
      { hotelId: "h3", hotelName: "无价格酒店", latitude: 39.92, longitude: 116.41 },
      { hotelId: "h4", hotelName: "危险链接", latitude: 39.93, longitude: 116.42, price: 200, detailUrl: "javascript:alert(1)" },
      { shId: "h5", name: "区间价格酒店", latitude: 39.94, longitude: 116.43, price: "¥5xx" }
    ]
  }
};
console.log(JSON.stringify(payload));
`;

let server;
let base;
let dir;
let callsFile;

before(async () => {
  dir = mkdtempSync(join(tmpdir(), "hotel-tool-"));
  const bin = join(dir, "flyai");
  callsFile = join(dir, "calls.json");
  writeFileSync(bin, fakeScript);
  chmodSync(bin, 0o755);
  server = createHotelServer({
    token: "test-token",
    flyaiBin: bin,
    cliTimeoutMs: 300,
    healthTimeoutMs: 1500,
    onCall: (args) => {
      writeFileSync(callsFile, JSON.stringify(args));
    },
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  base = `http://127.0.0.1:${server.address().port}`;
});

after(() => {
  server.close();
});

function post(body, headers = {}) {
  return fetch(`${base}/search`, {
    method: "POST",
    headers: { "content-type": "application/json", authorization: "Bearer test-token", ...headers },
    body: typeof body === "string" ? body : JSON.stringify(body),
  });
}

test("healthz reports CLI availability", async () => {
  const res = await fetch(`${base}/healthz`);
  assert.equal(res.status, 200);
  assert.equal((await res.json()).status, "ok");
});

test("rejects missing or wrong token", async () => {
  const res = await post({ city: "北京", check_in: "2026-11-20", check_out: "2026-11-22" }, { authorization: "" });
  assert.equal(res.status, 401);
  assert.equal((await res.json()).error.code, "unauthorized");
});

test("rejects non-json content type", async () => {
  const res = await fetch(`${base}/search`, {
    method: "POST",
    headers: { "content-type": "text/plain", authorization: "Bearer test-token" },
    body: "city=北京",
  });
  assert.equal(res.status, 415);
});

test("rejects invalid search input", async () => {
  const cases = [
    { check_in: "2026-11-20", check_out: "2026-11-22" },
    { city: "北京", check_in: "2026/11/20", check_out: "2026-11-22" },
    { city: "北京", check_in: "2026-11-22", check_out: "2026-11-20" },
    { city: "北京", check_in: "2026-11-20", check_out: "2026-11-22", min_star: 9 },
    { city: "北京", check_in: "2026-11-20", check_out: "2026-11-22", sort: "weird" },
    { city: "北京", check_in: "2026-11-20", check_out: "2026-11-22", keyword: "x".repeat(61) },
  ];
  for (const body of cases) {
    const res = await post(body);
    assert.equal(res.status, 400, JSON.stringify(body));
    assert.equal((await res.json()).error.code, "invalid_request");
  }
});

test("rejects oversized bodies", async () => {
  const res = await post(JSON.stringify({ city: "北京", check_in: "2026-11-20", check_out: "2026-11-22", keyword: "a".repeat(70000) }));
  assert.equal(res.status, 413);
});

test("normalizes hotels with coordinates and safe fields", async () => {
  const res = await post({ city: "北京", check_in: "2026-11-20", check_out: "2026-11-22", limit: 5 });
  assert.equal(res.status, 200);
  const payload = await res.json();
  assert.equal(payload.provider, "fliggy");
  assert.ok(payload.fetched_at);
  assert.equal(payload.hotels.length, 4);
  const first = payload.hotels[0];
  assert.equal(first.name, "近胡同酒店");
  assert.equal(first.price_per_night_yuan, 520);
  assert.equal(first.price_estimated, false);
  assert.equal(first.booking_url, "https://example.com/h1");
  assert.equal(payload.hotels.find((hotel) => hotel.id === "h3").price_estimated, true);
  assert.equal(payload.hotels.find((hotel) => hotel.id === "h4").booking_url, "");
  const range = payload.hotels.find((hotel) => hotel.id === "h5");
  assert.equal(range.price_per_night_yuan, 500);
  assert.equal(range.price_estimated, true);
});

test("passes search options as an argv array", async () => {
  const res = await post({
    city: "北京",
    check_in: "2026-11-20",
    check_out: "2026-11-22",
    keyword: "胡同",
    near: "南锣鼓巷",
    min_star: 4,
    max_price: 900,
    sort: "price_asc",
  });
  assert.equal(res.status, 200);
  const args = JSON.parse(readFileSync(callsFile, "utf8"));
  assert.deepEqual(args, [
    "search-hotel",
    "--dest-name", "北京",
    "--check-in-date", "2026-11-20",
    "--check-out-date", "2026-11-22",
    "--key-words", "胡同",
    "--poi-name", "南锣鼓巷",
    "--hotel-stars", "4",
    "--max-price", "900",
    "--sort", "price_asc",
  ]);
});

test("returns 504 on CLI timeout", async () => {
  process.env.FAKE_MODE = "timeout";
  try {
    const res = await post({ city: "北京", check_in: "2026-11-20", check_out: "2026-11-22" });
    assert.equal(res.status, 504);
  } finally {
    delete process.env.FAKE_MODE;
  }
});

test("returns 502 without leaking CLI stderr", async () => {
  process.env.FAKE_MODE = "fail";
  try {
    const res = await post({ city: "北京", check_in: "2026-11-20", check_out: "2026-11-22" });
    assert.equal(res.status, 502);
    const text = await res.text();
    assert.ok(!text.includes("secret upstream detail"));
  } finally {
    delete process.env.FAKE_MODE;
  }
});

test("returns 502 on invalid CLI output", async () => {
  process.env.FAKE_MODE = "garbage";
  try {
    const res = await post({ city: "北京", check_in: "2026-11-20", check_out: "2026-11-22" });
    assert.equal(res.status, 502);
  } finally {
    delete process.env.FAKE_MODE;
  }
});
