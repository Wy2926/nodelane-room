import { useCallback, useEffect, useState } from "react";
import type { API } from "./types";

export type Source = {
  id: string;
  name: string;
  kind: string;
  endpoint: string;
  bucket: string;
  region: string;
  prefix: string;
  public_url: string;
  path_style: boolean;
  enabled: boolean;
  priority: number;
  revision: number;
  has_credentials: boolean;
  access_key?: string;
  secret_key?: string;
};
export type Release = {
  id: string;
  version: string;
  os: string;
  arch: string;
  target: string;
  sha256: string;
  size: number;
  notes: string;
  state: string;
  revision: number;
  sources: string[];
  created_at: string;
};
export type Policy = {
  os: string;
  arch: string;
  release_id: string;
  minimum_version: string;
  effective_at?: string;
  revision: number;
};
export type Software = {
  version: string;
  gui_version?: string;
  os: string;
  arch: string;
  state: string;
  release_id?: string;
  error_code?: string;
  reported_at: string;
};
type Overview = {
  sources: Source[];
  releases: Release[];
  policies: Policy[];
  repository_revision: number;
  devices: {
    device_id: string;
    user_id: string;
    name: string;
    software: Software;
  }[];
  versions: {
    os: string;
    arch: string;
    version: string;
    devices: number;
    fresh: number;
  }[];
  attempts: {
    device_id: string;
    release_id: string;
    state: string;
    error_code?: string;
    updated_at: string;
  }[];
  next?: string;
};

export const states: Record<string, string> = {
  draft: "草稿",
  published: "已发布",
  paused: "已暂停",
  withdrawn: "已撤回",
  idle: "待机",
  checking: "检查中",
  available: "可更新",
  downloading: "下载中",
  ready: "待安装",
  installing: "安装中",
  succeeded: "已升级",
  failed: "失败",
  rolled_back: "已恢复旧版",
  unconfigured: "未配置更新信任",
};

export function useUpdatesOverview(api: API, after = "") {
  const [data, setData] = useState<Overview>();
  const [revision, setRevision] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const refresh = useCallback(() => setRevision((v) => v + 1), []);
  useEffect(() => {
    const abort = new AbortController();
    setLoading(true);
    void api<Overview>(
      `/updates?after=${encodeURIComponent(after)}`,
      undefined,
      "GET",
      abort.signal,
    )
      .then((value) => {
        if (!abort.signal.aborted) {
          setData(value);
          setError("");
        }
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(String(e));
      })
      .finally(() => {
        if (!abort.signal.aborted) setLoading(false);
      });
    return () => abort.abort();
  }, [api, revision, after]);
  return { data, error, refresh, loading };
}
