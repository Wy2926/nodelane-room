import type { Actions } from "../../app/use-actions";
export function Setup({ actions }: { actions: Actions }) {
  const { perform, busy } = actions;
  return (
    <section className="onboarding">
      <span className="eyebrow">WELCOME TO NODELANE</span>
      <h2>好游戏，<br />一起才尽兴。</h2>
      <p className="muted">给自己取个昵称。你的下一场冒险，即将开始。</p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const data = new FormData(e.currentTarget);
          void perform("初始化设备", {
            action: "init",
            server: String(data.get("server")).trim(),
            name: String(data.get("name")).trim(),
          });
        }}
      >
        <label>
          设备昵称
          <input
            name="name"
            required
            maxLength={40}
            placeholder="朋友能认出你的名字"
          />
        </label>
        <details className="server-choice">
          <summary>联机服务 · room.nodelane.net</summary>
          <label>控制端地址<input name="server" type="url" required defaultValue="https://room.nodelane.net" maxLength={2048} /></label>
        </details>
        <button className="primary" disabled={!!busy}>
          开始旅程
        </button>
        <p className="hint">设备身份由后台安全保存。日常联机无需管理员权限。</p>
      </form>
    </section>
  );
}
