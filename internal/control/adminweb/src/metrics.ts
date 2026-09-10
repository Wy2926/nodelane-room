import type { Link, Sample, Series, Telemetry } from "./types";

export const time = (v: string) => Date.parse(v);
export function latest(s?: Series, now = Date.now()): Sample | undefined {
  const x = s?.samples.at(-1);
  return x && now - time(x.at) <= 15000 && time(x.at) <= now + 10000
    ? x
    : undefined;
}
export function measured(p: Link, now: number) {
  return now - time(p.probe_at) <= 60000 && time(p.probe_at) <= now + 1000;
}
export function trafficPoints(s?: Series) {
  return (s?.samples || []).map((x, i, all) => {
    const prev = all[i - 1],
      elapsed = prev ? (time(x.at) - time(prev.at)) / 1000 : 0;
    if (
      !prev?.traffic ||
      !x.traffic ||
      prev.generation !== x.generation ||
      prev.epoch !== x.epoch ||
      prev.traffic.scope !== x.traffic.scope ||
      elapsed <= 0 ||
      elapsed > 15 ||
      x.traffic.upload_bytes < prev.traffic.upload_bytes ||
      x.traffic.download_bytes < prev.traffic.download_bytes
    )
      return { at: time(x.at) };
    return {
      at: time(x.at),
      upload: (x.traffic.upload_bytes - prev.traffic.upload_bytes) / elapsed,
      download:
        (x.traffic.download_bytes - prev.traffic.download_bytes) / elapsed,
      uploadBytes: x.traffic.upload_bytes - prev.traffic.upload_bytes,
      downloadBytes: x.traffic.download_bytes - prev.traffic.download_bytes,
    };
  });
}
export function traffic(s?: Series, now = Date.now()) {
  const points = trafficPoints(s),
    last = latest(s, now) ? points.at(-1) : undefined;
  const observed = points.some((p) => p.upload != null);
  return {
    upload: last?.upload,
    download: last?.download,
    uploadBytes: observed
      ? points.reduce((n, p) => n + (p.uploadBytes || 0), 0)
      : undefined,
    downloadBytes: observed
      ? points.reduce((n, p) => n + (p.downloadBytes || 0), 0)
      : undefined,
  };
}
export function quality(peers: Link[], now: number) {
  const valid = peers.filter((p) => measured(p, now));
  const rtts = valid.flatMap((p) => (p.rtt_ms == null ? [] : [p.rtt_ms]));
  const losses = valid.flatMap((p) =>
    p.loss_percent == null ? [] : [p.loss_percent],
  );
  return {
    min: rtts.length ? Math.min(...rtts) : undefined,
    max: rtts.length ? Math.max(...rtts) : undefined,
    loss: losses.length
      ? losses.reduce((a, b) => a + b, 0) / losses.length
      : undefined,
  };
}
export function roomMetrics(
  room: string,
  data: Telemetry | undefined,
  nodeDevices: Set<string>,
  now: number,
  memberIDs?: Set<string>,
) {
  const sources = (data?.series || []).filter(
    (s) => s.room_id === room && (!memberIDs || memberIDs.has(s.device_id)),
  );
  const active = sources.filter((s) => latest(s, now));
  const edges = new Map<string, { source: string; peer: Link; at: number }>();
  const observations: Link[] = [];
  for (const s of active)
    for (const p of latest(s, now)?.peers || []) {
      if (
        nodeDevices.has(p.device_id) ||
        (memberIDs && !memberIDs.has(p.device_id))
      )
        continue;
      observations.push(p);
      const key = [s.device_id, p.device_id].sort().join("/"),
        at = time(s.samples.at(-1)!.at);
      const old = edges.get(key);
      if (!old || at > old.at || (at === old.at && p.mode === "relay"))
        edges.set(key, { source: s.device_id, peer: p, at });
    }
  const links = [...edges.values()];
  const rates = active.map((s) => traffic(s, now));
  const sum = (key: "upload" | "download") =>
    rates.some((r) => r[key] != null)
      ? rates.reduce((n, r) => n + (r[key] || 0), 0)
      : undefined;
  return {
    links,
    connections: active.length ? links.length : undefined,
    direct: links.filter((x) => x.peer.mode === "direct").length,
    relay: links.filter((x) => x.peer.mode === "relay").length,
    upload: sum("upload"),
    download: sum("download"),
    reporting: active.length,
    trafficReporting: rates.filter((r) => r.upload != null).length,
    ...quality(observations, now),
  };
}
export function roomHistory(
  room: string,
  data: Telemetry | undefined,
  nodeDevices: Set<string>,
  now: number,
  memberIDs: Set<string>,
) {
  if (!data) return [];
  const sources = data.series.filter(
    (s) => s.room_id === room && memberIDs.has(s.device_id),
  );
  return Array.from({ length: 13 }, (_, index) => {
    const at = Math.floor(now / 5000) * 5000 - (12 - index) * 5000;
    const series = sources.map((s) => ({
      ...s,
      samples: s.samples.filter((p) => time(p.at) <= at),
    }));
    return {
      at,
      ...roomMetrics(room, { ...data, series }, nodeDevices, at, memberIDs),
    };
  });
}

export function exits(
  device: string,
  data: Telemetry | undefined,
  now: number,
) {
  const out: { source: string; node: boolean; link: Link; at: string }[] = [];
  for (const s of data?.series || []) {
    if (!latest(s, now)) continue;
    for (let i = s.samples.length - 1; i >= 0; i--) {
      const sample = s.samples[i];
      if (now - time(sample.at) > 60000) break;
      const p = sample.peers?.find((p) => p.device_id === device);
      if (!p) continue;
      if (p.remote)
        out.push({
          source: s.device_id,
          node: !!s.node_id,
          link: p,
          at: sample.at,
        });
      break;
    }
  }
  return out.sort((a, b) => Number(b.node) - Number(a.node));
}
