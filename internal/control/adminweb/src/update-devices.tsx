import { useState } from "react";
import { Card, Table, date } from "./components";
import type { API } from "./types";
import { useUpdatesOverview, states } from "./update-data";

export function UpdateDevices({ api }: { api: API }) {
  const [after, setAfter] = useState("");
  const { data, error, refresh, loading } = useUpdatesOverview(api, after);
  if (!data)
    return (
      <Card>
        <p role={error ? "alert" : "status"}>{error || "正在加载…"}</p>
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
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <Card title="设备版本与更新结果">
        <p>
          版本来自设备上报。超过两分钟无上报显示陈旧；下载完成不代表安装成功。
        </p>
        <Table
          heads={["平台", "版本", "设备总数", "两分钟内上报"]}
          empty={!data.versions.length}
        >
          {data.versions.map((v) => (
            <tr key={`${v.os}-${v.arch}-${v.version}`}>
              <td>
                {v.os}/{v.arch}
              </td>
              <td>{v.version}</td>
              <td>{v.devices}</td>
              <td>{v.fresh}</td>
            </tr>
          ))}
        </Table>
        <Table
          heads={[
            "用户",
            "设备",
            "服务 / 界面",
            "平台",
            "更新状态",
            "最后上报",
          ]}
          empty={!data.devices.length}
        >
          {data.devices.map((d) => (
            <tr key={d.device_id}>
              <td>{d.name}</td>
              <td>{d.device_id}</td>
              <td>
                {d.software.version} / {d.software.gui_version || "未知"}
              </td>
              <td>
                {d.software.os}/{d.software.arch}
              </td>
              <td>
                {states[d.software.state] || d.software.state}
                {d.software.error_code && (
                  <small>{d.software.error_code}</small>
                )}
              </td>
              <td>
                {date(d.software.reported_at)}
                {Date.now() - Date.parse(d.software.reported_at) > 120000 &&
                  "（陈旧）"}
              </td>
            </tr>
          ))}
        </Table>
        <button disabled={loading || !after} onClick={() => setAfter("")}>
          返回首页
        </button>
        <button
          disabled={loading || !data.next}
          onClick={() => setAfter(data.next || "")}
        >
          下一页
        </button>
        <h3>最近 100 条设备更新结果</h3>
        <Table
          heads={["设备", "目标版本", "结果", "更新时间"]}
          empty={!data.attempts.length}
        >
          {data.attempts.map((a) => (
            <tr key={`${a.device_id}-${a.release_id}`}>
              <td>{a.device_id}</td>
              <td>
                {data.releases.find((r) => r.id === a.release_id)?.version ||
                  a.release_id}
              </td>
              <td>
                {states[a.state] || a.state}
                {a.error_code && <small>{a.error_code}</small>}
              </td>
              <td>{date(a.updated_at)}</td>
            </tr>
          ))}
        </Table>
      </Card>
    </>
  );
}
