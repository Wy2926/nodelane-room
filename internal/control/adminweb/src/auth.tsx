import { useState, type FormEvent } from "react";
import { Field } from "./components";
import type { API, Session } from "./types";

export function Auth({
  api,
  signedIn,
  initiallySetup = false,
  initialMessage = "",
}: {
  api: API;
  signedIn: (session: Session) => void;
  initiallySetup?: boolean;
  initialMessage?: string;
}) {
  const [setup, setSetup] = useState(initiallySetup),
    [mode, setMode] = useState("create"),
    [ca, setCA] = useState("generate"),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(initialMessage);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    setBusy(true);
    setError("");
    try {
      const fd = new FormData(form);
      const values: Record<string, unknown> = {};
      for (const [k, v] of fd) if (typeof v === "string") values[k] = v;
      if (setup) {
        values.mode = mode;
        if (mode === "create" && ca === "upload")
          for (const key of ["ca_cert", "ca_key"]) {
            const f = fd.get(key);
            if (!(f instanceof File) || !f.size || f.size > 16384)
              throw new Error("请选择不超过 16 KB 的 PEM 文件。");
            values[key] = await f.text();
          }
        const result = await api<{ public_url: string }>("/setup", values);
        form.reset();
        setSetup(false);
        setError(
          `配置已保存，请稍后在当前入口登录。控制面地址：${result.public_url}`,
        );
      } else {
        const session = await api<Session>("/login", values);
        form.reset();
        signedIn(session);
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="auth-shell">
      <div className={"auth-card " + (setup ? "setup-card" : "")}>
        <a className="brand" href={window.location.pathname}>
          <b>N</b>
          <span>
            NodeLane <small>ROOM CONTROL</small>
          </span>
        </a>
        <p className="eyebrow">你的游戏网络，一目了然</p>
        <h1>{setup ? "初始化控制实例" : "欢迎回来"}</h1>
        <p className="muted">
          {setup
            ? "先在本实例终端运行 nodelane-server admin bootstrap，取得 10 分钟初始化码。"
            : "登录管理节点、房间与实时链路。"}
        </p>
        <form onSubmit={submit}>
          <Field
            label="管理员账号"
            name="username"
            autoComplete="username"
            required
            maxLength={80}
          />
          <Field
            label="密码"
            name="password"
            type="password"
            autoComplete={setup ? "new-password" : "current-password"}
            required
            minLength={setup ? 12 : undefined}
            maxLength={128}
          />
          {setup && (
            <>
              <label>
                部署方式
                <select
                  name="mode"
                  value={mode}
                  onChange={(e) => setMode(e.target.value)}
                >
                  <option value="create">创建控制面</option>
                  <option value="connect">接入已有控制面</option>
                </select>
              </label>
              <Field
                label="初始化码"
                name="code"
                type="password"
                autoComplete="off"
                required
                minLength={64}
                maxLength={64}
              />
              <Field
                label="PostgreSQL 连接串"
                name="database_url"
                type="password"
                autoComplete="off"
                required
                maxLength={8192}
              />
              {mode === "create" && (
                <>
                  <Field
                    label="统一公网地址"
                    name="public_url"
                    defaultValue={location.origin}
                    required
                  />
                  <Field
                    label="游戏地址池"
                    name="network"
                    defaultValue="10.203.0.0/16"
                    required
                  />
                  <Field
                    label="节点镜像仓库"
                    name="registry"
                    defaultValue="docker.nodelane.net"
                    required
                  />
                  <label>
                    CA 来源
                    <select
                      name="ca_mode"
                      value={ca}
                      onChange={(e) => setCA(e.target.value)}
                    >
                      <option value="generate">直接生成 CA</option>
                      <option value="upload">上传已有 CA</option>
                    </select>
                  </label>
                  {ca === "upload" && (
                    <>
                      <Field
                        label="CA 证书"
                        name="ca_cert"
                        type="file"
                        accept=".crt,.pem"
                        required
                      />
                      <Field
                        label="CA 私钥"
                        name="ca_key"
                        type="file"
                        accept=".key,.pem"
                        required
                      />
                    </>
                  )}
                  <small>地址池和 CA 初始化后固定，请避开已有网络。</small>
                </>
              )}
            </>
          )}
          <button className="primary" disabled={busy}>
            {busy ? "处理中…" : setup ? "保存并启动" : "登录管理台"}
          </button>
        </form>
        <button
          className="text-button"
          disabled={busy}
          onClick={() => {
            setSetup(!setup);
            setError("");
          }}
        >
          {setup ? "返回登录" : "首次部署？初始化控制实例"}
        </button>
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        <small>NodeLane Room · V2</small>
      </div>
      <div className="auth-decoration">
        <span>
          NETWORK
          <br />
          WITHOUT
          <br />
          DISTANCE.
        </span>
        <p>把连接留给我们，把时间留给游戏。</p>
      </div>
    </div>
  );
}
