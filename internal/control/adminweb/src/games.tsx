import { useState, type FormEvent } from "react";
import { Badge, Card, Field, Modal, Table } from "./components";
import type { API, Game, GamePort } from "./types";

export function GameEditor({
  game,
  api,
  saved,
}: {
  game: Game;
  api: API;
  saved: () => Promise<void>;
}) {
  const [name, setName] = useState(game.name);
  const [ports, setPorts] = useState<GamePort[]>(game.ports);
  const [enabled, setEnabled] = useState(game.enabled);
  const [revision] = useState(game.revision);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  function update(index: number, change: Partial<GamePort>) {
    setPorts(ports.map((p, i) => (i === index ? { ...p, ...change } : p)));
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(`/games/${game.id}`, { name, ports, enabled, revision }, "PUT");
      await saved();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit}>
      {(game.cover_url || game.background_url) && (
        <div className="game-art">
          {game.background_url && (
            <img
              className="game-background"
              src={game.background_url}
              alt={`${game.name} 背景图`}
            />
          )}
          {game.cover_url && (
            <img
              className="game-cover"
              src={game.cover_url}
              alt={`${game.name} 封面`}
            />
          )}
        </div>
      )}
      <p>{game.summary}</p>
      {game.source_url && (
        <p>
          <a href={game.source_url} target="_blank" rel="noreferrer">
            Steam 游戏资料来源 ↗
          </a>
        </p>
      )}
      <fieldset disabled={busy} className="game-fields">
        <Field
          label="游戏名称"
          value={name}
          maxLength={200}
          required
          onChange={(e) => setName(e.target.value)}
        />
        <h3>开放端口</h3>
        <p className="muted">
          按游戏实际需要添加 TCP/UDP 端口；结束端口留空表示单个端口。合计最多 32
          个，不可重复或包含 4243。
        </p>
        {ports.map((port, i) => (
          <div className="game-port" key={i}>
            <label>
              协议 {i + 1}
              <select
                value={port.protocol}
                onChange={(e) =>
                  update(i, { protocol: e.target.value as "tcp" | "udp" })
                }
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </label>
            <Field
              label={`起始端口 ${i + 1}`}
              type="number"
              min={1}
              max={65535}
              required
              value={port.port || ""}
              onChange={(e) => update(i, { port: Number(e.target.value) })}
            />
            <Field
              label={`结束端口 ${i + 1}`}
              type="number"
              min={port.port || 1}
              max={65535}
              value={port.port_end || ""}
              onChange={(e) =>
                update(i, { port_end: Number(e.target.value) || undefined })
              }
            />
            <Field
              label={`用途 ${i + 1}`}
              maxLength={120}
              value={port.description || ""}
              onChange={(e) => update(i, { description: e.target.value })}
            />
            <button
              type="button"
              aria-label={`删除端口 ${i + 1}`}
              onClick={() => setPorts(ports.filter((_, index) => index !== i))}
            >
              删除
            </button>
          </div>
        ))}
        <button
          type="button"
          disabled={ports.length >= 32}
          onClick={() => setPorts([...ports, { protocol: "tcp", port: 0 }])}
        >
          添加端口
        </button>
        <label className="game-enable">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          启用，允许客户端选择此游戏
        </label>
        <p className="notice">
          保存后已有房间同步采用新的端口配置。停用会移除已有游戏端口授权，并停止新建、加入该游戏的房间。
        </p>
        <button className="primary" disabled={busy}>
          {busy ? "正在保存…" : "保存游戏配置"}
        </button>
      </fieldset>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}

export function Games({
  games,
  api,
  refresh,
}: {
  games: Game[];
  api: API;
  refresh: () => Promise<void>;
}) {
  const [url, setURL] = useState("");
  const [query, setQuery] = useState("");
  const [editing, setEditing] = useState<Game>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function importGame(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const game = await api<Game>("/games/import", { url });
      setEditing(game);
      setURL("");
      await refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const filtered = games.filter((g) =>
    `${g.name} ${g.id}`.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <>
      <Card title="从 Steam 添加游戏">
        <p>
          粘贴 Steam
          商店游戏链接，自动导入名称、简介并下载封面和背景图。导入后配置端口，再启用供客户端选择。
        </p>
        <form onSubmit={importGame} className="game-import">
          <Field
            label="Steam 游戏链接"
            type="url"
            value={url}
            maxLength={2048}
            placeholder="https://store.steampowered.com/app/105600/"
            required
            disabled={busy}
            onChange={(e) => setURL(e.target.value)}
          />
          <button className="primary" disabled={busy}>
            {busy ? "正在下载资料与图片…" : "导入游戏"}
          </button>
        </form>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
      </Card>
      <Card title={`游戏列表 · ${games.length}`}>
        <p className="muted">
          通用游戏始终可选，玩家可自行开放端口；已配置游戏的端口统一在此管理。
        </p>
        <Field
          label="搜索游戏"
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <Table
          heads={["游戏", "状态", "开放端口", "操作"]}
          empty={!filtered.length}
        >
          {filtered.map((game) => (
            <tr key={game.id}>
              <td>
                <div className="game-title">
                  {game.cover_url && (
                    <img src={game.cover_url} alt="" loading="lazy" />
                  )}
                  <span>
                    <strong>{game.name}</strong>
                    <small>{game.id}</small>
                  </span>
                </div>
              </td>
              <td>
                <Badge kind={game.enabled ? "good" : ""}>
                  {game.enabled ? "已启用" : "未启用"}
                </Badge>
              </td>
              <td>
                {game.id === "custom" ? (
                  "由客户端自定义"
                ) : (
                  <>
                    {game.ports.map((p) => (
                      <small key={`${p.protocol}/${p.port}`}>
                        {p.protocol.toUpperCase()}/{p.port}
                        {p.port_end && p.port_end !== p.port
                          ? `–${p.port_end}`
                          : ""}{" "}
                        {p.description}
                      </small>
                    ))}
                    {!game.ports.length && "待配置"}
                  </>
                )}
              </td>
              <td>
                {game.id !== "custom" && (
                  <button onClick={() => setEditing(game)}>配置</button>
                )}
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {editing && (
        <Modal
          title={`配置 · ${editing.name}`}
          close={() => setEditing(undefined)}
        >
          <GameEditor
            key={editing.id}
            game={editing}
            api={api}
            saved={async () => {
              await refresh();
              setEditing(undefined);
            }}
          />
        </Modal>
      )}
    </>
  );
}
