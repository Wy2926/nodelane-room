import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Action, Card, Field, Table, bytes, date } from "./components";
import type { API } from "./types";

type Source = { id: string; name: string; kind: string; endpoint: string; bucket: string; region: string; prefix: string; public_url: string; path_style: boolean; enabled: boolean; priority: number; revision: number; has_credentials: boolean; access_key?: string; secret_key?: string };
type Release = { id: string; version: string; os: string; arch: string; target: string; sha256: string; size: number; notes: string; state: string; revision: number; sources: string[]; created_at: string };
type Policy = { os: string; arch: string; release_id: string; minimum_version: string; effective_at?: string; revision: number };
export type Software = { version: string; gui_version?: string; os: string; arch: string; state: string; release_id?: string; error_code?: string; reported_at: string };
type Overview = { sources: Source[]; releases: Release[]; policies: Policy[]; repository_revision: number; devices: { device_id: string; user_id: string; name: string; software: Software }[]; versions: {os: string; arch: string; version: string; devices: number; fresh: number}[]; attempts: {device_id: string; release_id: string; state: string; error_code?: string; updated_at: string}[]; next?: string };
const blankSource: Source = { id: "", name: "", kind: "r2", endpoint: "", bucket: "", region: "auto", prefix: "releases", public_url: "", path_style: false, enabled: true, priority: 10, revision: 0, has_credentials: false };
const states: Record<string, string> = { draft: "草稿", published: "已发布", paused: "已暂停", withdrawn: "已撤回", idle: "待机", checking: "检查中", available: "可更新", downloading: "下载中", ready: "待安装", installing: "安装中", succeeded: "已升级", failed: "失败", rolled_back: "已恢复旧版", unconfigured: "未配置更新信任" };

export function Updates({ api }: { api: API }) {
  const [data, setData] = useState<Overview>();
  const [after, setAfter] = useState("");
  const [revision, setRevision] = useState(0);
  const [error, setError] = useState("");
  const [source, setSource] = useState<Source>();
  const [release, setRelease] = useState<Release>();
  const [policy, setPolicy] = useState<Policy>();
  const [target, setTarget] = useState("");
  const [notes, setNotes] = useState("");
  const [repositoryFile, setRepositoryFile] = useState<File>();
  const [replicaSource, setReplicaSource] = useState("");
  const [packageFile, setPackageFile] = useState<File>();
  const [section, setSection] = useState("releases");
  const refresh = useCallback(() => setRevision(v => v + 1), []);
  useEffect(() => {
    const abort = new AbortController();
    void api<Overview>(`/updates?after=${encodeURIComponent(after)}`, undefined, "GET", abort.signal).then(v => { setData(v); setError(""); }).catch(e => { if (!abort.signal.aborted) setError(String(e)); });
    return () => abort.abort();
  }, [api, revision, after]);
  async function save(event: FormEvent, body: unknown, path: string, done: () => void) {
    event.preventDefault();
    try { await api(path, body, "PUT"); done(); refresh(); setError(""); } catch (e) { setError(String(e)); }
  }
  if (!data) return <Card title="更新管理"><p>{error || "正在加载…"}</p></Card>;
  return <>
    <Card title="更新管理">
      <div className="actions">{[["releases", "版本与发布"], ["sources", "存储源"], ["policies", "更新规则"], ["devices", "设备版本"]].map(([id, label]) => <button key={id} aria-pressed={section === id} onClick={() => setSection(id)}>{label}</button>)}<button onClick={refresh}>刷新</button></div>
      {error && <p className="error" role="alert">{error}</p>}
    </Card>
    {section === "sources" && <Card title="存储源">
      <p>数值较小的优先使用，其余作为备用源。修改地址或凭据后，已有包需重新验证。凭据只写入、不回显。</p>
      <button onClick={() => setSource({ ...blankSource })}>添加存储源</button>
      <Table heads={["名称", "类型", "优先级", "状态", "操作"]} empty={!data.sources.length}>{data.sources.map(s => <tr key={s.id}><td>{s.name}</td><td>{s.kind}</td><td>{s.priority}</td><td>{s.enabled ? "启用" : "停用"}</td><td><button onClick={() => setSource({ ...s, access_key: "", secret_key: "" })}>配置</button><Action run={async () => { await api(`/updates/sources/${s.id}/test`, {}); }}>测试连接</Action></td></tr>)}</Table>
      {source && <form onSubmit={e => void save(e, source, "/updates/sources", () => setSource(undefined))}>
        <Field label="名称" value={source.name} maxLength={80} required onChange={e => setSource({ ...source, name: e.target.value })} />
        <label>类型<select value={source.kind} onChange={e => setSource({ ...source, kind: e.target.value })}><option value="r2">Cloudflare R2</option><option value="s3">AWS S3 / MinIO / S3 兼容</option><option value="https">HTTPS / CDN 镜像</option></select></label>
        {source.kind !== "https" && <>
          <Field label="S3 Endpoint（HTTPS）" value={source.endpoint || ""} required onChange={e => setSource({ ...source, endpoint: e.target.value })} />
          <Field label="Bucket" value={source.bucket || ""} required onChange={e => setSource({ ...source, bucket: e.target.value })} />
          <Field label="Region（R2 使用 auto）" value={source.region || ""} onChange={e => setSource({ ...source, region: e.target.value })} />
          <Field label={source.has_credentials ? "Access Key（留空保留）" : "Access Key"} autoComplete="off" value={source.access_key || ""} onChange={e => setSource({ ...source, access_key: e.target.value })} />
          <Field label="Secret Key" type="password" autoComplete="new-password" value={source.secret_key || ""} onChange={e => setSource({ ...source, secret_key: e.target.value })} />
          <label><input type="checkbox" checked={source.path_style} onChange={e => setSource({ ...source, path_style: e.target.checked })} />使用 Path Style（部分 MinIO 部署需要）</label>
        </>}
        <Field label="对象前缀" value={source.prefix || ""} onChange={e => setSource({ ...source, prefix: e.target.value })} />
        <Field label="公开下载根地址（留空使用私有预签名 URL）" value={source.public_url || ""} required={source.kind === "https"} onChange={e => setSource({ ...source, public_url: e.target.value })} />
        <Field label="优先级" type="number" min={0} max={1000} value={source.priority} onChange={e => setSource({ ...source, priority: Number(e.target.value) })} />
        <label><input type="checkbox" checked={source.enabled} onChange={e => setSource({ ...source, enabled: e.target.checked })} />启用</label>
        <button>保存</button><button type="button" onClick={() => setSource(undefined)}>取消</button>
      </form>}
    </Card>}
    {section === "releases" && <>
      <Card title="签名发布清单">
        <p>当前清单修订 {data.repository_revision}。先导入由发布工具生成的 repository.json，再登记包、验证下载源并发布。签名私钥留在发布环境。</p>
        <input aria-label="签名发布清单" type="file" accept=".json" onChange={e => setRepositoryFile(e.target.files?.[0])} />
        <Action run={async () => { if (!repositoryFile) throw new Error("请选择签名清单"); if (repositoryFile.size > 3 * 1024 * 1024) throw new Error("清单过大"); const bundle = JSON.parse(await repositoryFile.text()); await api("/updates/repository", { metadata: bundle.metadata, revision: data.repository_revision }, "PUT"); refresh(); }}>导入签名清单</Action>
      </Card>
      <Card title="版本与安装包">
        <form onSubmit={e => void save(e, { target, notes, state: "draft", revision: 0 }, "/updates/releases", () => { setTarget(""); setNotes(""); })}>
          <Field label="签名清单中的安装包文件名" required value={target} onChange={e => setTarget(e.target.value)} />
          <label>更新说明<textarea value={notes} maxLength={16000} onChange={e => setNotes(e.target.value)} /></label><button>创建草稿</button>
        </form>
        <Table heads={["版本", "平台", "状态", "大小", "已验证源", "操作"]} empty={!data.releases.length}>{data.releases.map(r => <tr key={r.id}><td>{r.version}</td><td>{r.os} / {r.arch}</td><td>{states[r.state]}</td><td>{bytes(r.size)}</td><td>{r.sources.length}</td><td><button onClick={() => { setRelease(r); setReplicaSource(""); setPackageFile(undefined); }}>管理</button></td></tr>)}</Table>
        {release && <form onSubmit={e => void save(e, release, "/updates/releases", () => setRelease(undefined))}>
          <h3>{release.version} · {release.os}/{release.arch}</h3><p>{release.target}</p><p className="mono">SHA256 {release.sha256}</p>
          <label>更新说明<textarea value={release.notes} maxLength={16000} onChange={e => setRelease({ ...release, notes: e.target.value })} /></label>
          <label>发布状态<select value={release.state} onChange={e => setRelease({ ...release, state: e.target.value })}>{["draft", "published", "paused", "withdrawn"].map(v => <option key={v} value={v}>{states[v]}</option>)}</select></label>
          <p>暂停或撤回前，先移除引用此版本的更新规则。</p>
          <button>保存发布状态</button><button type="button" onClick={() => setRelease(undefined)}>关闭</button>
          <label>包存储源<select value={replicaSource} onChange={e => setReplicaSource(e.target.value)}><option value="">选择存储源</option>{data.sources.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <input aria-label="完整安装包" type="file" accept=".exe,.deb" onChange={e => setPackageFile(e.target.files?.[0])} />
          <Action run={async () => { if (!replicaSource || !packageFile) throw new Error("请选择源和安装包"); if (packageFile.size !== release.size) throw new Error("包大小与签名清单不符"); await api(`/updates/releases/${release.id}/sources/${replicaSource}`, packageFile, "PUT"); refresh(); }}>上传并验证</Action>
          <Action run={async () => { if (!replicaSource) throw new Error("请选择存储源"); await api(`/updates/releases/${release.id}/sources/${replicaSource}`, {}); refresh(); }}>验证源上已有包</Action>
        </form>}
      </Card>
    </>}
    {section === "policies" && <Card title="更新规则">
      <p>推荐版本用于普通更新。最低版本在生效时间到达后限制旧客户端联机，并断开其现有连接；更新和诊断仍可用。</p>
      {["windows", "linux"].flatMap(os => ["amd64", "arm64"].map(arch => { const p = data.policies.find(v => v.os === os && v.arch === arch); return <div key={`${os}-${arch}`} className="actions"><span>{os} / {arch} · 最低版本 {p?.minimum_version || "未限制"} · {date(p?.effective_at)}</span><button onClick={() => setPolicy(p || { os, arch, release_id: "", minimum_version: "", revision: 0 })}>配置</button></div>; }))}
      {policy && <form onSubmit={e => void save(e, policy, "/updates/policies", () => setPolicy(undefined))}>
        <h3>{policy.os} / {policy.arch}</h3>
        <label>推荐版本<select value={policy.release_id} onChange={e => setPolicy({ ...policy, release_id: e.target.value })}><option value="">移除此平台规则</option>{data.releases.filter(r => r.state === "published" && r.os === policy.os && r.arch === policy.arch).map(r => <option key={r.id} value={r.id}>{r.version}</option>)}</select></label>
        <Field label="最低允许版本（留空不强制）" value={policy.minimum_version || ""} onChange={e => setPolicy({ ...policy, minimum_version: e.target.value })} />
        <Field label="强制更新生效时间" type="datetime-local" required={!!policy.minimum_version} value={policy.effective_at ? new Date(Date.parse(policy.effective_at) - new Date(policy.effective_at).getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ""} onChange={e => setPolicy({ ...policy, effective_at: e.target.value ? new Date(e.target.value).toISOString() : undefined })} />
        <button>保存规则</button><button type="button" onClick={() => setPolicy(undefined)}>取消</button>
      </form>}
    </Card>}
    {section === "devices" && <Card title="设备版本与更新结果">
      <p>版本来自设备上报。超过两分钟无上报显示陈旧；下载完成不代表安装成功。</p>
      <Table heads={["平台", "版本", "设备总数", "两分钟内上报"]} empty={!data.versions.length}>{data.versions.map(v => <tr key={`${v.os}-${v.arch}-${v.version}`}><td>{v.os}/{v.arch}</td><td>{v.version}</td><td>{v.devices}</td><td>{v.fresh}</td></tr>)}</Table>
      <Table heads={["用户", "设备", "服务 / 界面", "平台", "更新状态", "最后上报"]} empty={!data.devices.length}>{data.devices.map(d => <tr key={d.device_id}><td>{d.name}</td><td>{d.device_id}</td><td>{d.software.version} / {d.software.gui_version || "未知"}</td><td>{d.software.os}/{d.software.arch}</td><td>{states[d.software.state] || d.software.state}{d.software.error_code && <small>{d.software.error_code}</small>}</td><td>{date(d.software.reported_at)}{Date.now() - Date.parse(d.software.reported_at) > 120000 && "（陈旧）"}</td></tr>)}</Table>
      <button disabled={!after} onClick={() => setAfter("")}>返回首页</button><button disabled={!data.next} onClick={() => setAfter(data.next || "")}>下一页</button>
      <h3>最近 100 条设备更新结果</h3>
      <Table heads={["设备", "目标版本", "结果", "更新时间"]} empty={!data.attempts.length}>{data.attempts.map(a => <tr key={`${a.device_id}-${a.release_id}`}><td>{a.device_id}</td><td>{data.releases.find(r => r.id === a.release_id)?.version || a.release_id}</td><td>{states[a.state] || a.state}{a.error_code && <small>{a.error_code}</small>}</td><td>{date(a.updated_at)}</td></tr>)}</Table>
    </Card>}
  </>;
}
