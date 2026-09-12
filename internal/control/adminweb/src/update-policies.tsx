import { useState, type FormEvent } from "react";
import { Card, Field, Table, date } from "./components";
import type { API } from "./types";
import { useUpdatesOverview, type Policy } from "./update-data";

export function UpdatePolicies({ api }: { api: API }) {
  const { data, error: loadError, refresh, loading } = useUpdatesOverview(api);
  const [error, setError] = useState("");
  const problem = error || loadError;
  const [policy, setPolicy] = useState<Policy>();
  const [busy, setBusy] = useState(false);
  async function save(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api("/updates/policies", policy, "PUT");
      setPolicy(undefined);
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
      <Card title="更新规则">
        <p>
          推荐版本用于普通更新。最低版本在生效时间到达后限制旧客户端联机，并断开其现有连接；更新和诊断仍可用。
        </p>
        <Table
          heads={[
            "平台",
            "推荐版本",
            "最低允许版本",
            "强制更新生效时间",
            "操作",
          ]}
        >
          {["windows", "linux"].flatMap((os) =>
            ["amd64", "arm64"].map((arch) => {
              const p = data.policies.find(
                (value) => value.os === os && value.arch === arch,
              );
              return (
                <tr key={`${os}-${arch}`}>
                  <td>
                    {os} / {arch}
                  </td>
                  <td>
                    {data.releases.find(
                      (release) => release.id === p?.release_id,
                    )?.version || "未设置"}
                  </td>
                  <td>{p?.minimum_version || "未限制"}</td>
                  <td>{date(p?.effective_at)}</td>
                  <td>
                    <button
                      aria-label={`配置 ${os} ${arch}`}
                      onClick={() =>
                        setPolicy(
                          p || {
                            os,
                            arch,
                            release_id: "",
                            minimum_version: "",
                            revision: 0,
                          },
                        )
                      }
                    >
                      配置
                    </button>
                  </td>
                </tr>
              );
            }),
          )}
        </Table>
      </Card>
      {policy && (
        <Card title={`配置更新规则 · ${policy.os}/${policy.arch}`}>
          <form onSubmit={(e) => void save(e)}>
            <h3>
              {policy.os} / {policy.arch}
            </h3>
            <label>
              推荐版本
              <select
                value={policy.release_id}
                onChange={(e) =>
                  setPolicy({ ...policy, release_id: e.target.value })
                }
              >
                <option value="">移除此平台规则</option>
                {data.releases
                  .filter(
                    (r) =>
                      r.state === "published" &&
                      r.os === policy.os &&
                      r.arch === policy.arch,
                  )
                  .map((r) => (
                    <option key={r.id} value={r.id}>
                      {r.version}
                    </option>
                  ))}
              </select>
            </label>
            <Field
              label="最低允许版本（留空不强制）"
              value={policy.minimum_version || ""}
              onChange={(e) =>
                setPolicy({ ...policy, minimum_version: e.target.value })
              }
            />
            <Field
              label="强制更新生效时间"
              type="datetime-local"
              required={!!policy.minimum_version}
              value={
                policy.effective_at
                  ? new Date(
                      Date.parse(policy.effective_at) -
                        new Date(policy.effective_at).getTimezoneOffset() *
                          60000,
                    )
                      .toISOString()
                      .slice(0, 16)
                  : ""
              }
              onChange={(e) =>
                setPolicy({
                  ...policy,
                  effective_at: e.target.value
                    ? new Date(e.target.value).toISOString()
                    : undefined,
                })
              }
            />
            <button className="primary" disabled={busy}>
              保存规则
            </button>
            <button type="button" onClick={() => setPolicy(undefined)}>
              取消
            </button>
          </form>
        </Card>
      )}
    </>
  );
}
