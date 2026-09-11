import { useEffect, useState, type FormEvent } from "react";
import { Action, Card, Field, Table, date } from "./components";
import type { API, Room } from "./types";
import type { Software } from "./updates";

type User = { id: string; name: string; kind: string; state: string; created_at: string };
type UserPage = { users: User[]; next: string };
type Detail = { user: User; devices: { device_id: string; name: string; revoked: boolean; last_seen: string; expires_at?: string; software?: Software }[]; rooms: Room[]; sessions: { device_id: string; expires_at: string }[] };
type OIDC = { issuer: string; client_id: string; client_secret?: string; enabled: boolean };
const kinds: Record<string, string> = { guest: "访客", registered: "正式" };
const states: Record<string, string> = { active: "正常", disabled: "停用", deleted: "已删除" };

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
    void api<UserPage>(`/users?q=${encodeURIComponent(filter)}&after=${encodeURIComponent(after)}`, undefined, "GET", abort.signal).then(p => { if (!abort.signal.aborted) { setPage(p); setError(""); } }).catch(e => { if (!abort.signal.aborted) setError(String(e)); });
    return () => abort.abort();
  }, [api, filter, after, revision]);
  useEffect(() => {
    setDetail(undefined); if (!selected) return;
    const abort = new AbortController();
    void api<Detail>(`/users/${selected}`, undefined, "GET", abort.signal).then(d => { if (!abort.signal.aborted) setDetail(d); }).catch(e => { if (!abort.signal.aborted) setError(String(e)); });
    return () => abort.abort();
  }, [api, selected, revision]);
  async function act(action: string, device_id?: string) {
    if (!reason.trim()) throw new Error("请先填写操作原因");
    await api(`/users/${selected}/actions`, { action, device_id, reason: reason.trim() });
    setRevision(r => r + 1);
  }
  return <>
    <Card title="用户管理">
      <form onSubmit={e => { e.preventDefault(); setAfter(""); setFilter(query.trim()); }}><label>昵称或用户 ID<input value={query} maxLength={80} onChange={e => setQuery(e.target.value)} /></label><button>搜索</button></form>
      {error && <p role="alert" className="error">{error}</p>}
      {!page ? <p>正在加载…</p> : <><Table heads={["昵称", "身份", "状态", "创建时间", "用户 ID", "详情"]} empty={!page.users.length}>{page.users.map(u => <tr key={u.id}><td>{u.name}</td><td>{kinds[u.kind]}</td><td>{states[u.state]}</td><td>{date(u.created_at)}</td><td>{u.id}</td><td><button onClick={() => { setSelected(u.id); setReason(""); }}>查看</button></td></tr>)}</Table><button disabled={!after} onClick={() => setAfter("")}>返回首页</button><button disabled={!page.next} onClick={() => setAfter(page.next)}>下一页</button></>}
    </Card>
    {detail && <Card title={`${detail.user.name} · ${kinds[detail.user.kind]}`}>
      <p>{detail.user.id} · {states[detail.user.state]}。账号身份与付费权益分别管理，当前尚未提供付费功能。</p>
      <label>操作原因<input value={reason} maxLength={500} onChange={e => setReason(e.target.value)} /></label>
      {detail.user.state !== "deleted" && <div className="actions"><Action run={() => act(detail.user.state === "active" ? "disable" : "enable")}>{detail.user.state === "active" ? "停用并关闭其房间" : "启用"}</Action><Action run={() => act("logout")}>撤销全部设备登录</Action><Action run={() => act("delete")}>逻辑删除并关闭其房间</Action></div>}
      <p>撤销未绑定账号的访客设备后，该访客将无法找回。停用或删除会关闭其拥有的房间，并断开相关连接。</p>
      <Table heads={["设备", "标识", "状态", "最近登录", "软件版本 / 平台", "版本上报", "授权截止", "操作"]} empty={!detail.devices.length}>{detail.devices.map(d => <tr key={d.device_id}><td>{d.name}</td><td>{d.device_id}</td><td>{d.revoked ? "已撤销" : "已登记"}</td><td>{date(d.last_seen)}</td><td>{d.software ? `${d.software.version} / ${d.software.gui_version || "界面未知"} · ${d.software.os}/${d.software.arch}` : "未知"}</td><td>{date(d.software?.reported_at)}</td><td>{d.expires_at ? date(d.expires_at) : "访客本机凭据"}</td><td>{!d.revoked && detail.user.state !== "deleted" && <Action run={() => act("revoke-device", d.device_id)}>撤销</Action>}</td></tr>)}</Table>
      <h3>有效会话（最多 100 条）</h3><Table heads={["设备", "到期"]} empty={!detail.sessions.length}>{detail.sessions.map((s, i) => <tr key={`${s.device_id}-${i}`}><td>{s.device_id}</td><td>{date(s.expires_at)}</td></tr>)}</Table>
      <h3>关联房间（最多 100 个）</h3><Table heads={["房间", "角色", "状态"]} empty={!detail.rooms.length}>{detail.rooms.map(r => <tr key={r.id}><td>{r.name}</td><td>{r.owner_user_id === selected ? "房主" : "成员"}</td><td>{r.closed ? "已关闭" : date(r.expires_at)}</td></tr>)}</Table>
    </Card>}
  </>;
}

export function OIDCSettings({ api, publicURL }: { api: API; publicURL: string }) {
  const [value, setValue] = useState<OIDC>();
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => { const abort = new AbortController(); void api<OIDC>("/oidc", undefined, "GET", abort.signal).then(v => { if (!abort.signal.aborted) setValue(v); }).catch(e => { if (!abort.signal.aborted) setError(String(e)); }); return () => abort.abort(); }, [api]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy) return; setBusy(true); setError(""); setSaved(false);
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      await api("/oidc", { issuer: String(data.get("issuer")).trim(), client_id: String(data.get("client_id")).trim(), client_secret: String(data.get("client_secret")), enabled: data.get("enabled") === "on" }, "PUT");
      setValue(await api<OIDC>("/oidc")); form.reset(); setSaved(true);
    } catch (e) { setError(String(e)); } finally { setBusy(false); }
  }
  return <Card title="账号登录（OIDC）"><p>访客始终可以使用。开启后允许绑定账号及在其他设备登录。将以下回调地址填入身份提供方应用的 Redirect URI：<code>{publicURL.replace(/\/$/, "")}/v2/auth/oidc/callback</code></p>
    <p>Logto 选择“传统 Web 应用（Traditional Web）”。App ID 填入 Client ID，App Secret 填入 Client secret。</p>
    {error && <p role="alert" className="error">{error}</p>}{saved && <p role="status">OIDC 配置已保存。</p>}{value && <form onSubmit={submit} onChange={() => setSaved(false)}>
    <Field label="Issuer" name="issuer" type="url" defaultValue={value.issuer} placeholder="https://auth.nodelane.net/oidc" required />
    <p>填写 OpenID 配置中的 issuer 值；Logto 通常包含 /oidc。授权、Token 和 JWKS 地址自动发现。</p>
    <Field label="Client ID" name="client_id" defaultValue={value.client_id} placeholder="Logto App ID" required />
    <Field label="Client secret（相同客户端留空保留，不回显）" name="client_secret" type="password" autoComplete="new-password" />
    <label><input name="enabled" type="checkbox" defaultChecked={value.enabled} />启用 OIDC</label>
    <p>使用授权码流程、PKCE S256 和固定 HTTPS 回调；修改配置会使正在进行的登录失效。身份提供方的停用不会自动通知本站。</p>
    <button disabled={busy}>保存配置</button>
  </form>}</Card>;
}
