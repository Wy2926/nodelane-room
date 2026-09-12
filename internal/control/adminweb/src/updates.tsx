import { useState, type FormEvent } from "react";
import { Action, Card, Field, Table, bytes } from "./components";
import type { API } from "./types";
import { useUpdatesOverview, states, type Release } from "./update-data";

export function Releases({ api }: { api: API }) {
  const { data, error: loadError, refresh, loading } = useUpdatesOverview(api);
  const [error, setError] = useState("");
  const problem = error || loadError;
  const [release, setRelease] = useState<Release>();
  const [target, setTarget] = useState("");
  const [notes, setNotes] = useState("");
  const [repositoryFile, setRepositoryFile] = useState<File>();
  const [replicaSource, setReplicaSource] = useState("");
  const [packageFile, setPackageFile] = useState<File>();
  const [busy, setBusy] = useState(false);
  const [creating, setCreating] = useState(false);
  async function save(event: FormEvent, body: unknown, done: () => void) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    try {
      await api("/updates/releases", body, "PUT");
      done();
      refresh();
      setError("");
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
      <Card title="发布准备">
        <p>
          按顺序完成签名清单、版本草稿和安装包验证，确认后发布；推荐版本在
          <a href="#policies">更新规则</a>中设置。
        </p>
        {!data.sources.length && (
          <p className="notice">
            尚未配置下载源，请先<a href="#sources">添加更新存储源</a>。
          </p>
        )}
        <details>
          <summary>
            导入签名发布清单 · 当前修订 {data.repository_revision}
          </summary>
          <p>
            当前清单修订 {data.repository_revision}。先导入由发布工具生成的
            repository.json，再登记包、验证下载源并发布。签名私钥留在发布环境。
          </p>
          <input
            aria-label="签名发布清单"
            type="file"
            accept=".json"
            onChange={(e) => setRepositoryFile(e.target.files?.[0])}
          />
          <Action
            run={async () => {
              if (!repositoryFile) throw new Error("请选择签名清单");
              if (repositoryFile.size > 3 * 1024 * 1024)
                throw new Error("清单过大");
              const bundle = JSON.parse(await repositoryFile.text());
              await api(
                "/updates/repository",
                {
                  metadata: bundle.metadata,
                  revision: data.repository_revision,
                },
                "PUT",
              );
              refresh();
            }}
          >
            导入签名清单
          </Action>
        </details>
      </Card>
      <Card>
        <div className="section-heading">
          <div>
            <h2>版本与安装包</h2>
            <p className="muted">查看各平台版本，上传与验证安装包。</p>
          </div>
          <button className="primary" onClick={() => setCreating(!creating)}>
            {creating ? "收起新建" : "创建版本"}
          </button>
        </div>
        {creating && (
          <form
            className="editor-panel"
            onSubmit={(e) =>
              void save(
                e,
                { target, notes, state: "draft", revision: 0 },
                () => {
                  setTarget("");
                  setNotes("");
                  setCreating(false);
                },
              )
            }
          >
            <Field
              label="签名清单中的安装包文件名"
              required
              value={target}
              onChange={(e) => setTarget(e.target.value)}
            />
            <label>
              更新说明
              <textarea
                value={notes}
                maxLength={16000}
                onChange={(e) => setNotes(e.target.value)}
              />
            </label>
            <button disabled={busy}>创建草稿</button>
          </form>
        )}
        <Table
          heads={["版本", "平台", "状态", "大小", "已验证源", "操作"]}
          empty={!data.releases.length}
        >
          {data.releases.map((r) => (
            <tr key={r.id}>
              <td>{r.version}</td>
              <td>
                {r.os} / {r.arch}
              </td>
              <td>{states[r.state]}</td>
              <td>{bytes(r.size)}</td>
              <td>{r.sources.length}</td>
              <td>
                <button
                  onClick={() => {
                    setRelease(r);
                    setReplicaSource("");
                    setPackageFile(undefined);
                  }}
                >
                  管理
                </button>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {release && (
        <Card
          title={`管理版本 · ${release.version} · ${release.os}/${release.arch}`}
        >
          <form
            onSubmit={(e) => void save(e, release, () => setRelease(undefined))}
          >
            <h3>
              {release.version} · {release.os}/{release.arch}
            </h3>
            <p>{release.target}</p>
            <p className="mono">SHA256 {release.sha256}</p>
            <label>
              更新说明
              <textarea
                value={release.notes}
                maxLength={16000}
                onChange={(e) =>
                  setRelease({ ...release, notes: e.target.value })
                }
              />
            </label>
            <label>
              发布状态
              <select
                value={release.state}
                onChange={(e) =>
                  setRelease({ ...release, state: e.target.value })
                }
              >
                {["draft", "published", "paused", "withdrawn"].map((v) => (
                  <option key={v} value={v}>
                    {states[v]}
                  </option>
                ))}
              </select>
            </label>
            <p>暂停或撤回前，先移除引用此版本的更新规则。</p>
            <div className="actions">
              <button className="primary" disabled={busy}>
                保存发布状态
              </button>
              <button type="button" onClick={() => setRelease(undefined)}>
                关闭
              </button>
            </div>
            <h3>安装包分发</h3>
            <label>
              包存储源
              <select
                value={replicaSource}
                onChange={(e) => setReplicaSource(e.target.value)}
              >
                <option value="">选择存储源</option>
                {data.sources.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
            </label>
            <input
              aria-label="完整安装包"
              type="file"
              accept=".exe,.deb"
              onChange={(e) => setPackageFile(e.target.files?.[0])}
            />
            <Action
              run={async () => {
                if (!replicaSource || !packageFile)
                  throw new Error("请选择源和安装包");
                if (packageFile.size !== release.size)
                  throw new Error("包大小与签名清单不符");
                await api(
                  `/updates/releases/${release.id}/sources/${replicaSource}`,
                  packageFile,
                  "PUT",
                );
                refresh();
              }}
            >
              上传并验证
            </Action>
            <Action
              run={async () => {
                if (!replicaSource) throw new Error("请选择存储源");
                await api(
                  `/updates/releases/${release.id}/sources/${replicaSource}`,
                  {},
                );
                refresh();
              }}
            >
              验证源上已有包
            </Action>
          </form>
        </Card>
      )}
    </>
  );
}
