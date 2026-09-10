import { useEffect, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { enable, disable, isEnabled } from "@tauri-apps/plugin-autostart";
import { Check, Copy, Desktop, Palette, Power, SlidersHorizontal } from "@phosphor-icons/react";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import { PlayerAvatar } from "../../shared/ui/PlayerAvatar";

export function Settings({ status, actions, usable }: {
  status: Status; actions: Actions; usable: boolean;
}) {
  const { busy, setBusy, setError, confirm, quit, copy } = actions;
  const [autoStart, setAutoStart] = useState<boolean>();
  useEffect(() => {
    if (isTauri()) void isEnabled().then(setAutoStart).catch(() => {});
  }, []);
  return (
    <div className="settings">
      <section className="settings-profile console-surface" aria-label="本机玩家资料">
        <PlayerAvatar name={status.name} identity={status.device_id} size="large" />
        <div className="settings-profile-copy"><span className="eyebrow">YOUR PROFILE</span><h2>{status.name}</h2><p>本机玩家 · 这台设备上的联机身份</p></div>
        <div className="settings-platform"><Desktop size={28} weight="light" aria-hidden="true" /><span>桌面客户端<small>NodeLane Room</small></span></div>
      </section>
      <div className="settings-grid">
        <section className="settings-preferences" aria-labelledby="preferences-title">
          <div className="system-heading"><SlidersHorizontal size={22} weight="light" aria-hidden="true" /><h2 id="preferences-title">桌面偏好</h2><span className="eyebrow">PREFERENCES</span></div>
          <div className="theme-preview console-surface">
            <div className="theme-topline"><Palette size={21} weight="light" aria-hidden="true" /><span className="theme-current"><Check size={13} aria-hidden="true" />当前主题</span></div>
            <span className="eyebrow">MIDNIGHT</span><h3>午夜</h3><p>深邃蓝黑，银白焦点。<br />让游戏成为画面的主角。</p>
          </div>
          <label className="switch-row console-surface">
            <Power size={24} weight="light" aria-hidden="true" />
            <span className="setting-row-copy"><strong>随系统启程</strong><small>登录系统时打开客户端</small>{autoStart === undefined && <small>开机启动状态暂不可用</small>}</span>
            <input
              type="checkbox"
              role="switch"
              aria-label="登录系统时打开客户端"
              checked={autoStart ?? false}
              disabled={autoStart === undefined || !!busy}
              onChange={async (e) => {
                const checked = e.target.checked;
                setBusy("保存开机启动设置");
                try {
                  await (checked ? enable() : disable());
                  setAutoStart(await isEnabled());
                } catch {
                  setError({ code: "autostart", error: "无法保存开机启动设置，请检查当前用户权限。" });
                } finally {
                  setBusy("");
                }
              }}
            />
          </label>
        </section>
        <section className="settings-device" aria-labelledby="device-title">
          <div className="system-heading"><Desktop size={22} weight="light" aria-hidden="true" /><h2 id="device-title">这台设备</h2><span className="eyebrow">SYSTEM</span></div>
          <div className="device-details console-surface">
            <dl>
              <div><dt>设备昵称</dt><dd>{status.name}</dd></div>
              <div><dt>控制端</dt><dd>{status.server}</dd></div>
              <div><dt>设备标识</dt><dd className="device-identity"><span className="mono selectable">{status.device_id}</span><button className="text-button" aria-label="复制设备标识" title="复制设备标识" onClick={() => void copy(status.device_id)}><Copy size={17} weight="light" aria-hidden="true" /></button></dd></div>
              <div className="device-versions"><div><dt>客户端版本</dt><dd className="mono">0.2.0</dd></div><div><dt>后台版本</dt><dd className="mono">{status.version}</dd></div></div>
            </dl>
          </div>
        </section>
      </div>
      <section className="settings-session console-surface" aria-label="退出客户端">
        <Power size={25} weight="light" aria-hidden="true" />
        <div className="setting-row-copy"><h2>暂别，或继续联机</h2><p>关闭窗口后网络后台继续运行；没有可用托盘时窗口最小化。</p></div>
        <div className="actions">
          <button onClick={() => void quit()} disabled={!!busy}>退出界面（继续联机）</button>
          <button className="danger subtle" disabled={!usable} onClick={() => status.selected_room ? confirm("离房并退出", "停止本机联机后退出界面。", { action: "leave" }, true) : void quit()}>离房并退出</button>
        </div>
      </section>
    </div>
  );
}
