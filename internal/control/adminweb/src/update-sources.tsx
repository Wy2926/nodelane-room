import { useState, type FormEvent } from "react";
import { Action, Card, Field, Table } from "./components";
import type { API } from "./types";
import { useUpdatesOverview, type Source } from "./update-data";

const blankSource: Source = {
  id: "",
  name: "",
  kind: "r2",
  endpoint: "",
  bucket: "",
  region: "auto",
  prefix: "releases",
  public_url: "",
  path_style: false,
  enabled: true,
  priority: 10,
  revision: 0,
  has_credentials: false,
};

export function UpdateSources({ api }: { api: API }) {
  const { data, error: loadError, refresh, loading } = useUpdatesOverview(api);
  const [error, setError] = useState("");
  const problem = error || loadError;
  const [source, setSource] = useState<Source>();
  const [busy, setBusy] = useState(false);
  async function save(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api("/updates/sources", source, "PUT");
      setSource(undefined);
      refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  if (!data)
    return (
      <Card>
        <p role={problem ? "alert" : "status"}>{problem || "正在加载…"}</p>
        <button onClick={refresh} disabled={loading}>
          重试
        </button>
      </Card>
    );
  return (
    <>
      <div className="page-actions">
        <button onClick={refresh} disabled={loading}>
          {loading ? "刷新中…" : "刷新"}
        </button>
      </div>
      {problem && (
        <p className="error" role="alert">
          {problem}
        </p>
      )}
      <Card>
        <div className="section-heading">
          <div>
            <h2>更新存储源</h2>
            <p className="muted">维护安装包来源与备用顺序。</p>
          </div>
          <button
            className="primary"
            onClick={() => setSource({ ...blankSource })}
          >
            添加存储源
          </button>
        </div>
        <p>
          数值较小的优先使用，其余作为备用源。修改地址或凭据后，已有包需重新验证。凭据只写入、不回显。
        </p>
        <Table
          heads={["名称", "类型", "优先级", "状态", "操作"]}
          empty={!data.sources.length}
        >
          {data.sources.map((s) => (
            <tr key={s.id}>
              <td>{s.name}</td>
              <td>{s.kind}</td>
              <td>{s.priority}</td>
              <td>{s.enabled ? "启用" : "停用"}</td>
              <td>
                <button
                  onClick={() =>
                    setSource({ ...s, access_key: "", secret_key: "" })
                  }
                >
                  配置
                </button>
                <Action
                  run={async () => {
                    await api(`/updates/sources/${s.id}/test`, {});
                  }}
                >
                  测试连接
                </Action>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {source && (
        <Card title={source.id ? `配置存储源 · ${source.name}` : "添加存储源"}>
          <form onSubmit={(e) => void save(e)}>
            <Field
              autoFocus
              label="名称"
              value={source.name}
              maxLength={80}
              required
              onChange={(e) => setSource({ ...source, name: e.target.value })}
            />
            <label>
              类型
              <select
                value={source.kind}
                onChange={(e) => setSource({ ...source, kind: e.target.value })}
              >
                <option value="r2">Cloudflare R2</option>
                <option value="s3">AWS S3 / MinIO / S3 兼容</option>
                <option value="https">HTTPS / CDN 镜像</option>
              </select>
            </label>
            {source.kind !== "https" && (
              <>
                <Field
                  label="S3 Endpoint（HTTPS）"
                  value={source.endpoint || ""}
                  required
                  onChange={(e) =>
                    setSource({ ...source, endpoint: e.target.value })
                  }
                />
                <Field
                  label="Bucket"
                  value={source.bucket || ""}
                  required
                  onChange={(e) =>
                    setSource({ ...source, bucket: e.target.value })
                  }
                />
                <Field
                  label="Region（R2 使用 auto）"
                  value={source.region || ""}
                  onChange={(e) =>
                    setSource({ ...source, region: e.target.value })
                  }
                />
                <Field
                  label={
                    source.has_credentials
                      ? "Access Key（留空保留）"
                      : "Access Key"
                  }
                  autoComplete="off"
                  value={source.access_key || ""}
                  onChange={(e) =>
                    setSource({ ...source, access_key: e.target.value })
                  }
                />
                <Field
                  label="Secret Key"
                  type="password"
                  autoComplete="new-password"
                  value={source.secret_key || ""}
                  onChange={(e) =>
                    setSource({ ...source, secret_key: e.target.value })
                  }
                />
                <label>
                  <input
                    type="checkbox"
                    checked={source.path_style}
                    onChange={(e) =>
                      setSource({ ...source, path_style: e.target.checked })
                    }
                  />
                  使用 Path Style（部分 MinIO 部署需要）
                </label>
              </>
            )}
            <Field
              label="对象前缀"
              value={source.prefix || ""}
              onChange={(e) => setSource({ ...source, prefix: e.target.value })}
            />
            <Field
              label="公开下载根地址（留空使用私有预签名 URL）"
              value={source.public_url || ""}
              required={source.kind === "https"}
              onChange={(e) =>
                setSource({ ...source, public_url: e.target.value })
              }
            />
            <Field
              label="优先级"
              type="number"
              min={0}
              max={1000}
              value={source.priority}
              onChange={(e) =>
                setSource({ ...source, priority: Number(e.target.value) })
              }
            />
            <label>
              <input
                type="checkbox"
                checked={source.enabled}
                onChange={(e) =>
                  setSource({ ...source, enabled: e.target.checked })
                }
              />
              启用
            </label>
            <button className="primary" disabled={busy}>
              保存
            </button>
            <button type="button" onClick={() => setSource(undefined)}>
              取消
            </button>
          </form>
        </Card>
      )}
    </>
  );
}
