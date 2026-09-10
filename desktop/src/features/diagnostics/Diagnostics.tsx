import { useState } from "react";
import type { Status, Failure } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import { formatTime } from "../../shared/time";
export function Diagnostics({
  status,
  actions,
  usable,
  serviceError,
}: {
  status: Status;
  actions: Actions;
  usable: boolean;
  serviceError?: Failure;
}) {
  const { perform, copy } = actions;
  const [diagnostic, setDiagnostic] = useState<Record<string, unknown>>();
  return (
    <section className="panel">
      <div className="section-head">
        <div>
          <span className="eyebrow">让问题更清楚</span>
          <h2>网络诊断</h2>
        </div>
        <button
          className="primary"
          disabled={!usable}
          onClick={() =>
            void perform<Record<string, unknown>>(
              "运行诊断",
              { action: "doctor" },
              setDiagnostic,
            )
          }
        >
          运行诊断
        </button>
      </div>
      <p className="muted">
        查看网络后台、隧道驱动、授权期限及真实链路测量。诊断不会改变网络权限。
      </p>
      <div className="metric-grid">
        <div>
          <small>网络后台</small>
          <strong>{serviceError ? "不可用" : "已连接"}</strong>
        </div>
        <div>
          <small>游戏网络</small>
          <strong>{status.engine === "running" ? "运行中" : "已停止"}</strong>
        </div>
        <div>
          <small>虚拟 IP</small>
          <strong className="mono">{status.ip || "—"}</strong>
        </div>
        <div>
          <small>授权截止</small>
          <strong>{formatTime(status.lease_expires_at)}</strong>
        </div>
      </div>
      {diagnostic && (
        <>
          <pre className="diagnostics">
            {JSON.stringify(diagnostic, null, 2)}
          </pre>
          <button
            onClick={() => {
              const {
                device_id: _id,
                virtual_ip: _ip,
                peers: _peers,
                platform: _platform,
                ...safe
              } = diagnostic;
              void copy(
                JSON.stringify(
                  {
                    ...safe,
                    error: safe.error ? "存在错误，请在应用内查看" : "",
                    platform: "已省略本机环境与网卡信息",
                  },
                  null,
                  2,
                ),
              );
            }}
          >
            复制脱敏诊断
          </button>
          <p className="hint">复制时省略设备标识、IP、网卡和逐成员信息。</p>
        </>
      )}
    </section>
  );
}
