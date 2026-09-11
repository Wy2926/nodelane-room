import { useEffect, useState } from "react";
import { isTauri } from "@tauri-apps/api/core";
import { enable, disable, isEnabled } from "@tauri-apps/plugin-autostart";
import { ArrowClockwise, Check, Copy, Desktop, DownloadSimple, Info, Moon, Power, SlidersHorizontal } from "@phosphor-icons/react";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import { PlayerAvatar } from "../../shared/ui/PlayerAvatar";
import { clientVersion } from "../../native/api";

const categories = [
  { id: "preferences", title: "桌面偏好", icon: SlidersHorizontal },
  { id: "device", title: "设备信息", icon: Desktop },
  { id: "updates", title: "版本与更新", icon: DownloadSimple },
  { id: "session", title: "退出与联机", icon: Power },
] as const;

export function Settings({ status, actions, usable }: {
  status?: Status; actions: Actions; usable: boolean;
}) {
  const { busy, setBusy, setError, confirm, quit, copy } = actions;
  const [category, setCategory] = useState<typeof categories[number]["id"]>("preferences");
  const [autoStart, setAutoStart] = useState<boolean>();
  const [updateRequested, setUpdateRequested] = useState(false);
  useEffect(() => {
    let active = true;
    if (isTauri()) void isEnabled().then((value) => { if (active) setAutoStart(value); }).catch(() => {});
    return () => { active = false; };
  }, []);
  return (
    <div className="settings">
      <div className="page-intro"><span className="eyebrow">让这里，更像你的主场</span><h2>设置</h2><p>管理桌面偏好、设备与客户端版本。</p></div>
      <div className="settings-layout">
        <aside className="settings-sidebar">
          <div className="settings-profile">
            <PlayerAvatar name={status?.name || "N"} identity={status?.device_id} size="large" />
            <h3>{status?.name || "本机玩家"}</h3><p>{status?.device_id ? "这台设备上的联机身份" : "尚未读取设备身份"}</p>
          </div>
          <nav className="settings-nav" aria-label="设置分类">
            {categories.map(({ id, title, icon: Icon }) => <button key={id} aria-current={category === id ? "page" : undefined} onClick={() => setCategory(id)}><Icon size={21} aria-hidden="true" />{title}</button>)}
          </nav>
          <span className="settings-signature">NodeLane Room <span className="mono">{clientVersion}</span></span>
        </aside>
        <div className="settings-content console-surface">
          {category === "preferences" && <section aria-labelledby="preferences-title">
            <div className="settings-section-head"><h3 id="preferences-title">桌面偏好</h3><p>熟悉的环境，随时准备开始。</p></div>
            <div className="theme-preview">
              <div className="theme-preview-copy"><Moon size={28} weight="light" aria-hidden="true" /><h4>午夜</h4><p>深邃蓝黑，银白焦点。</p></div>
              <span className="theme-current"><Check size={16} aria-hidden="true" />当前主题</span>
            </div>
            <label className="switch-row">
              <Power size={24} weight="light" aria-hidden="true" />
              <span className="setting-row-copy"><strong>开机启动</strong><small>登录系统时打开客户端</small>{autoStart === undefined && <small>开机启动状态暂不可用</small>}</span>
              <input type="checkbox" role="switch" aria-label="登录系统时打开客户端" checked={autoStart ?? false} disabled={autoStart === undefined || !!busy}
                onChange={async (e) => {
                  const checked = e.target.checked;
                  setBusy("保存开机启动设置");
                  try { await (checked ? enable() : disable()); setAutoStart(await isEnabled()); }
                  catch { setError({ code: "autostart", error: "无法保存开机启动设置，请检查当前用户权限。" }); }
                  finally { setBusy(""); }
                }} />
            </label>
            <div className="setting-row"><Desktop size={24} weight="light" aria-hidden="true" /><div className="setting-row-copy"><strong>关闭窗口后继续联机</strong><small>窗口收至托盘；没有可用托盘时最小化。</small></div><span className="setting-value">默认行为</span></div>
          </section>}
          {category === "device" && <section aria-labelledby="device-title">
            <div className="settings-section-head"><h3 id="device-title">设备信息</h3><p>用于连接和识别当前设备。</p></div>
            <dl className="device-details">
              <div><dt>设备昵称</dt><dd>{status?.name || "尚未配置"}</dd></div>
              <div><dt>联机服务</dt><dd className="selectable">{status?.server || "尚未配置"}</dd></div>
              <div><dt>设备标识</dt><dd className="device-identity"><span className="mono selectable">{status?.device_id || "尚未配置"}</span><button className="icon-button" disabled={!status?.device_id} aria-label="复制设备标识" title="复制设备标识" onClick={() => void copy(status!.device_id)}><Copy size={19} aria-hidden="true" /></button></dd></div>
              <div><dt>后台版本</dt><dd className="mono">{status?.version || "暂不可用"}</dd></div>
            </dl>
          </section>}
          {category === "updates" && <section aria-labelledby="updates-title">
            <div className="settings-section-head"><h3 id="updates-title">版本与更新</h3><p>让下一次相聚，准备得更充分。</p></div>
            <div className="update-version"><span className="update-icon"><DownloadSimple size={36} weight="light" aria-hidden="true" /></span><div><span className="muted">NodeLane Room</span><h4>当前版本 <span className="mono">{clientVersion}</span></h4><p>后台版本 <span className="mono">{status?.version || "暂不可用"}</span></p></div></div>
            <div className="update-action"><div><strong>检查客户端更新</strong><p>在线更新服务暂未接入。</p></div><button onClick={() => setUpdateRequested(true)}><ArrowClockwise size={19} aria-hidden="true" />检查更新</button></div>
            {updateRequested && <div className="update-message" role="status"><Info size={22} aria-hidden="true" /><p>暂时无法检查更新。在线更新服务尚未接入，请使用新版完整安装包手动更新。</p></div>}
            <div className="settings-help"><h4>如何更新</h4><p>先退出界面，再运行新版完整安装包。设备身份会保留，界面与网络后台会一同更新；安装期间联机会短暂中断。</p></div>
          </section>}
          {category === "session" && <section aria-labelledby="session-title">
            <div className="settings-section-head"><h3 id="session-title">退出与联机</h3><p>按你的节奏，选择如何暂别。</p></div>
            <div className="session-option"><span className="session-icon"><Desktop size={28} weight="light" aria-hidden="true" /></span><div><h4>让这一局继续</h4><p>退出界面后，网络后台继续运行。你仍会留在当前房间。</p><button onClick={() => void quit()} disabled={!!busy}>退出界面（继续联机）</button></div></div>
            <div className="session-option"><span className="session-icon"><Power size={28} weight="light" aria-hidden="true" /></span><div><h4>今天就到这里</h4><p>离开当前房间，停止本机联机并退出界面。</p><button className="danger subtle" disabled={!usable} onClick={() => status?.selected_room ? confirm("离房并退出", "停止本机联机后退出界面。", { action: "leave" }, true) : void quit()}>离房并退出</button></div></div>
          </section>}
        </div>
      </div>
    </div>
  );
}
