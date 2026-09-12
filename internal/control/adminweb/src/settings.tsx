import { useEffect, useState, type FormEvent } from "react";
import { Card, Field } from "./components";
import type { API } from "./types";

type OIDC = {
  revision: number;
  issuer: string;
  client_id: string;
  client_secret?: string;
  enabled: boolean;
};

export function Password({ api, done }: { api: API; done: () => void }) {
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/password", Object.fromEntries(new FormData(e.currentTarget)));
      done();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit}>
      <Field
        label="当前密码"
        type="password"
        name="current"
        autoComplete="current-password"
        required
      />
      <Field
        label="新密码（12–128 字节）"
        type="password"
        name="password"
        autoComplete="new-password"
        minLength={12}
        maxLength={128}
        required
      />
      <button className="primary" disabled={busy}>
        保存新密码
      </button>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
    </form>
  );
}

export function OIDCSettings({
  api,
  publicURL,
}: {
  api: API;
  publicURL: string;
}) {
  const [value, setValue] = useState<OIDC>();
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const abort = new AbortController();
    void api<OIDC>("/oidc", undefined, "GET", abort.signal)
      .then((v) => {
        if (!abort.signal.aborted) setValue(v);
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(String(e));
      });
    return () => abort.abort();
  }, [api]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    setSaved(false);
    const form = event.currentTarget;
    const data = new FormData(form);
    try {
      await api(
        "/oidc",
        {
          revision: value?.revision || 0,
          issuer: String(data.get("issuer")).trim(),
          client_id: String(data.get("client_id")).trim(),
          client_secret: String(data.get("client_secret")),
          enabled: data.get("enabled") === "on",
        },
        "PUT",
      );
      setValue(await api<OIDC>("/oidc"));
      form.reset();
      setSaved(true);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Card title="账号登录（OIDC）">
      <p>
        访客始终可以使用。开启后允许绑定账号及在其他设备登录。将以下回调地址填入身份提供方应用的
        Redirect URI：
        <code>{publicURL.replace(/\/$/, "")}/v2/auth/oidc/callback</code>
      </p>
      <p>
        Logto 选择“传统 Web 应用（Traditional Web）”。App ID 填入 Client ID，App
        Secret 填入 Client secret。
      </p>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {saved && <p role="status">OIDC 配置已保存。</p>}
      {value && (
        <form onSubmit={submit} onChange={() => setSaved(false)}>
          <Field
            label="Issuer"
            name="issuer"
            type="url"
            defaultValue={value.issuer}
            placeholder="https://auth.nodelane.net/oidc"
            required
          />
          <p>
            填写 OpenID 配置中的 issuer 值；Logto 通常包含 /oidc。授权、Token 和
            JWKS 地址自动发现。
          </p>
          <Field
            label="Client ID"
            name="client_id"
            defaultValue={value.client_id}
            placeholder="Logto App ID"
            required
          />
          <Field
            label="Client secret（相同客户端留空保留，不回显）"
            name="client_secret"
            type="password"
            autoComplete="new-password"
          />
          <label>
            <input
              name="enabled"
              type="checkbox"
              defaultChecked={value.enabled}
            />
            启用 OIDC
          </label>
          <p>
            使用授权码流程、PKCE S256 和固定 HTTPS
            回调；修改配置会使正在进行的登录失效。身份提供方的停用不会自动通知本站。
          </p>
          <button disabled={busy}>保存配置</button>
        </form>
      )}
    </Card>
  );
}
