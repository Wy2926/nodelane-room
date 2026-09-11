// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { GameEditor, Games } from "./games";
import type { API, Game } from "./types";

const originalShowModal = Object.getOwnPropertyDescriptor(
  HTMLDialogElement.prototype,
  "showModal",
);

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (originalShowModal)
    Object.defineProperty(
      HTMLDialogElement.prototype,
      "showModal",
      originalShowModal,
    );
  else Reflect.deleteProperty(HTMLDialogElement.prototype, "showModal");
});
const game: Game = {
  network: { version: 1, broadcast: true, multicast: true, ethernet_types: [] },
  id: "steam-105600",
  name: "Terraria",
  summary: "Build and play",
  source_url: "https://store.steampowered.com/app/105600/",
  cover_url: "/v2/games/steam-105600/images/cover",
  background_url: "/v2/games/steam-105600/images/background",
  ports: [{ protocol: "tcp", port: 7777 }],
  enabled: false,
  revision: 1,
};

it("keeps unsaved ports and the original revision when server snapshots change", async () => {
  const api = vi.fn().mockResolvedValue(game),
    saved = vi.fn();
  const view = render(
    <GameEditor game={game} api={api as API} saved={saved} />,
  );
  const user = userEvent.setup();
  await user.clear(screen.getByLabelText("起始端口 1"));
  await user.type(screen.getByLabelText("起始端口 1"), "7778");
  view.rerender(
    <GameEditor
      game={{ ...game, revision: 2, ports: [] }}
      api={api as API}
      saved={saved}
    />,
  );
  await user.click(screen.getByLabelText("启用，允许客户端选择此游戏"));
  await user.click(screen.getByRole("button", { name: "保存游戏配置" }));
  expect(api).toHaveBeenCalledWith(
    "/games/steam-105600",
    expect.objectContaining({
      revision: 1,
      enabled: true,
      ports: [{ protocol: "tcp", port: 7778 }],
    }),
    "PUT",
  );
});

it("shows import failures without discarding the pasted link", async () => {
  const api = vi.fn().mockRejectedValue(new Error("图片下载失败"));
  render(<Games games={[]} api={api as API} refresh={vi.fn()} />);
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Steam 游戏链接"), game.source_url);
  await user.click(screen.getByRole("button", { name: "导入游戏" }));
  expect(screen.getByRole("alert").textContent).toContain("图片下载失败");
  expect(
    (screen.getByLabelText("Steam 游戏链接") as HTMLInputElement).value,
  ).toBe(game.source_url);
});

it("manages generic games with the same server policy", () => {
  render(
    <Games
      games={[
        { ...game, id: "custom", name: "通用游戏", ports: [], enabled: true },
      ]}
      api={vi.fn() as API}
      refresh={vi.fn()}
    />,
  );
  expect(screen.getByRole("button", { name: "配置" })).toBeTruthy();
  expect(screen.queryByText("由客户端自定义")).toBeNull();
});

it("saves complete port ranges and explicit LAN permissions", async () => {
  const api = vi.fn().mockResolvedValue(game);
  render(<GameEditor game={game} api={api as API} saved={vi.fn()} />);
  const user = userEvent.setup();
  await user.clear(screen.getByLabelText("起始端口 1"));
  await user.type(screen.getByLabelText("起始端口 1"), "1");
  await user.type(screen.getByLabelText("结束端口 1"), "65535");
  await user.type(screen.getByLabelText("额外以太网协议"), "0x8137, 0");
  await user.click(screen.getByLabelText("允许游戏广播"));
  await user.click(screen.getByRole("button", { name: "保存游戏配置" }));
  expect(api).toHaveBeenCalledWith(
    "/games/steam-105600",
    expect.objectContaining({
      ports: [{ protocol: "tcp", port: 1, port_end: 65535 }],
      network: {
        version: 1,
        broadcast: false,
        multicast: true,
        ethernet_types: [0x8137, 0],
      },
    }),
    "PUT",
  );
});

it("opens the downloaded draft for manual port configuration before enabling", async () => {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.open = true;
    },
  });
  const imported = { ...game, ports: [] };
  const api = vi.fn().mockResolvedValue(imported),
    refresh = vi.fn();
  render(<Games games={[]} api={api as API} refresh={refresh} />);
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Steam 游戏链接"), game.source_url);
  await user.click(screen.getByRole("button", { name: "导入游戏" }));
  await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
  expect(api).toHaveBeenCalledWith("/games/import", { url: game.source_url });
  expect(screen.getByAltText("Terraria 封面").getAttribute("src")).toBe(
    game.cover_url,
  );
  expect(screen.getByAltText("Terraria 背景图").getAttribute("src")).toBe(
    game.background_url,
  );
  expect(
    (screen.getByLabelText("启用，允许客户端选择此游戏") as HTMLInputElement)
      .checked,
  ).toBe(false);
  await user.click(screen.getByRole("button", { name: "添加端口" }));
  await user.type(screen.getByLabelText("起始端口 1"), "7777");
  await user.click(screen.getByLabelText("启用，允许客户端选择此游戏"));
  await user.click(screen.getByRole("button", { name: "保存游戏配置" }));
  expect(api).toHaveBeenLastCalledWith(
    "/games/steam-105600",
    expect.objectContaining({
      enabled: true,
      ports: [{ protocol: "tcp", port: 7777 }],
    }),
    "PUT",
  );
});
