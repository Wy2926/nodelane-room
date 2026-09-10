import { describe, expect, it } from "vitest";
import { exits, latest, roomMetrics, traffic } from "./metrics";
import type { Sample, Series, Telemetry } from "./types";

const now = Date.parse("2026-09-10T08:00:00Z");
const sample = (
  seconds: number,
  upload: number,
  download: number,
  generation = 1,
): Sample => ({
  at: new Date(now + seconds * 1000).toISOString(),
  generation,
  epoch: "process-one",
  connections: 1,
  peers: [],
  traffic: { upload_bytes: upload, download_bytes: download, scope: "overlay" },
});
describe("real traffic accounting", () => {
  it("uses elapsed seconds and resets on restart or missing samples", () => {
    const s: Series = {
      device_id: "a",
      samples: [sample(-5, 100, 200), sample(0, 200, 400)],
    };
    expect(traffic(s, now)).toMatchObject({
      upload: 20,
      download: 40,
      uploadBytes: 100,
    });
    s.samples[1].generation = 2;
    expect(traffic(s, now).upload).toBeUndefined();
    s.samples[1].generation = 1;
    s.samples[1].epoch = "process-two";
    expect(traffic(s, now).upload).toBeUndefined();
    s.samples = [sample(-30, 0, 0), sample(0, 200, 400)];
    expect(traffic(s, now).upload).toBeUndefined();
    s.samples = [sample(-5, 100, 200), sample(0, 20, 40)];
    expect(traffic(s, now).upload).toBeUndefined();
  });
  it("keeps unknown separate from measured zero and expires stale data", () => {
    expect(traffic(undefined, now).upload).toBeUndefined();
    const s: Series = {
      device_id: "a",
      samples: [sample(-5, 100, 200), sample(0, 100, 200)],
    };
    expect(traffic(s, now).upload).toBe(0);
    expect(latest(s, now + 16000)).toBeUndefined();
  });
});
it("deduplicates member pairs, excludes infrastructure and uses observed exits", () => {
  const a = sample(0, 0, 0),
    b = sample(0, 0, 0);
  a.peers = [
    {
      device_id: "b",
      ip: "10.203.0.2",
      mode: "relay",
      relay_ips: ["10.203.0.9"],
      probe_at: a.at,
      rtt_ms: 20,
      loss_percent: 5,
    },
    {
      device_id: "node",
      ip: "10.203.0.9",
      mode: "direct",
      remote: "8.8.8.8:4242",
      relay_ips: [],
      probe_at: a.at,
      rtt_ms: 90,
    },
  ];
  b.peers = [
    {
      device_id: "a",
      ip: "10.203.0.1",
      mode: "relay",
      relay_ips: ["10.203.0.9"],
      probe_at: b.at,
      rtt_ms: 22,
      loss_percent: 0,
    },
  ];
  const data: Telemetry = {
    server_time: a.at,
    retention_seconds: 60,
    stale_seconds: 15,
    geoip: false,
    series: [
      { device_id: "a", room_id: "r", samples: [a] },
      { device_id: "b", room_id: "r", samples: [b] },
    ],
  };
  expect(roomMetrics("r", data, new Set(["node"]), now)).toMatchObject({
    connections: 1,
    relay: 1,
    direct: 0,
    reporting: 2,
    min: 20,
    max: 22,
  });
  expect(exits("a", data, now)).toHaveLength(0);
  expect(exits("node", data, now)[0].link.remote).toBe("8.8.8.8:4242");
  expect(
    roomMetrics("r", data, new Set(["node"]), now, new Set(["a"])),
  ).toMatchObject({ connections: 0, reporting: 1 });
});
