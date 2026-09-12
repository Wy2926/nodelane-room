import { useEffect, useState } from "react";
import { Action, Card, Field, Table, date } from "./components";
import type { API, Room } from "./types";
import type { Software } from "./update-data";

type User = {
  id: string;
  name: string;
  kind: string;
  state: string;
  created_at: string;
};
type UserPage = { users: User[]; next: string };
type Detail = {
  user: User;
  devices: {
    device_id: string;
    name: string;
    revoked: boolean;
    last_seen: string;
    expires_at?: string;
    software?: Software;
  }[];
  rooms: Room[];
  sessions: { device_id: string; expires_at: string }[];
};
const kinds: Record<string, string> = { guest: "访客", registered: "正式" };
const states: Record<string, string> = {
  active: "正常",
  disabled: "限制建房",
  deleted: "已删除",
};

export function Users({ api }: { api: API }) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("");
  const [after, setAfter] = useState("");
  const [page, setPage] = useState<UserPage>();
  const [selected, setSelected] = useState("");
  const [detail, setDetail] = useState<Detail>();
  const [revision, setRevision] = useState(0);
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    const abort = new AbortController();
    setPage(undefined);
    void api<UserPage>(
      `/users?q=${encodeURIComponent(filter)}&after=${encodeURIComponent(after)}`,
      undefined,
      "GET",
      abort.signal,
    )
      .then((p) => {
        if (!abort.signal.aborted) {
          setPage(p);
          setError("");
        }
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(String(e));
      });
    return () => abort.abort();
  }, [api, filter, after, revision]);
  useEffect(() => {
    setDetail(undefined);
    if (!selected) return;
    const abort = new AbortController();
    void api<Detail>(`/users/${selected}`, undefined, "GET", abort.signal)
      .then((d) => {
        if (!abort.signal.aborted) setDetail(d);
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(String(e));
      });
    return () => abort.abort();
  }, [api, selected, revision]);
  async function act(action: string, device_id?: string) {
    if (!reason.trim()) throw new Error("请先填写操作原因");
    await api(`/users/${selected}/actions`, {
      action,
      device_id,
      reason: reason.trim(),
    });
    setRevision((r) => r + 1);
  }
  return (
    <>
      <Card title="用户管理">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setAfter("");
            setFilter(query.trim());
          }}
        >
          <label>
            昵称或用户 ID
            <input
              value={query}
              maxLength={80}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <button>搜索</button>
        </form>
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        {!page ? (
          <p>正在加载…</p>
        ) : (
          <>
            <Table
              heads={["昵称", "身份", "状态", "创建时间", "用户 ID", "详情"]}
              empty={!page.users.length}
            >
              {page.users.map((u) => (
                <tr key={u.id}>
                  <td>{u.name}</td>
                  <td>{kinds[u.kind]}</td>
                  <td>{states[u.state]}</td>
                  <td>{date(u.created_at)}</td>
                  <td>{u.id}</td>
                  <td>
                    <button
                      onClick={() => {
                        setSelected(u.id);
                        setReason("");
                      }}
                    >
                      查看
                    </button>
                  </td>
                </tr>
              ))}
            </Table>
            <button disabled={!after} onClick={() => setAfter("")}>
              返回首页
            </button>
            <button disabled={!page.next} onClick={() => setAfter(page.next)}>
              下一页
            </button>
          </>
        )}
      </Card>
      {detail && (
        <Card title={`${detail.user.name} · ${kinds[detail.user.kind]}`}>
          <p>
            {detail.user.id} · {states[detail.user.state]}
            。账号身份与付费权益分别管理，当前尚未提供付费功能。
          </p>
          <label>
            操作原因
            <input
              value={reason}
              maxLength={500}
              onChange={(e) => setReason(e.target.value)}
            />
          </label>
          {detail.user.state !== "deleted" && (
            <div className="actions">
              <Action
                run={() =>
                  act(detail.user.state === "active" ? "disable" : "enable")
                }
              >
                {detail.user.state === "active"
                  ? "限制创建房间"
                  : "恢复创建房间"}
              </Action>
              <Action run={() => act("logout")}>撤销全部设备登录</Action>
              <Action run={() => act("delete")}>逻辑删除并关闭其房间</Action>
            </div>
          )}
          <p>
            建房限制保留登录、加入和已有房间。删除账号会关闭其房间并断开连接。撤销未绑定账号的访客设备后，该访客将无法找回。
          </p>
          <Table
            heads={[
              "设备",
              "标识",
              "状态",
              "最近登录",
              "软件版本 / 平台",
              "版本上报",
              "授权截止",
              "操作",
            ]}
            empty={!detail.devices.length}
          >
            {detail.devices.map((d) => (
              <tr key={d.device_id}>
                <td>{d.name}</td>
                <td>{d.device_id}</td>
                <td>{d.revoked ? "已撤销" : "已登记"}</td>
                <td>{date(d.last_seen)}</td>
                <td>
                  {d.software
                    ? `${d.software.version} / ${d.software.gui_version || "界面未知"} · ${d.software.os}/${d.software.arch}`
                    : "未知"}
                </td>
                <td>{date(d.software?.reported_at)}</td>
                <td>{d.expires_at ? date(d.expires_at) : "访客本机凭据"}</td>
                <td>
                  {!d.revoked && detail.user.state !== "deleted" && (
                    <Action run={() => act("revoke-device", d.device_id)}>
                      撤销
                    </Action>
                  )}
                </td>
              </tr>
            ))}
          </Table>
          <h3>有效会话（最多 100 条）</h3>
          <Table heads={["设备", "到期"]} empty={!detail.sessions.length}>
            {detail.sessions.map((s, i) => (
              <tr key={`${s.device_id}-${i}`}>
                <td>{s.device_id}</td>
                <td>{date(s.expires_at)}</td>
              </tr>
            ))}
          </Table>
          <h3>关联房间（最多 100 个）</h3>
          <Table heads={["房间", "角色", "状态"]} empty={!detail.rooms.length}>
            {detail.rooms.map((r) => (
              <tr key={r.id}>
                <td>{r.name}</td>
                <td>{r.owner_user_id === selected ? "房主" : "成员"}</td>
                <td>{r.closed ? "已关闭" : date(r.expires_at)}</td>
              </tr>
            ))}
          </Table>
        </Card>
      )}
    </>
  );
}
