// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { NodeEditor } from "./nodes";
import type { API, InfraNode } from "./types";

afterEach(cleanup);
it("keeps the edited configuration bound to its original revision across snapshots", async () => {
  const node: InfraNode = {
    id: "node",
    device_id: "n",
    name: "original",
    region: "test",
    address: "node.example:4242",
    notes: "",
    lighthouse: true,
    relay: true,
    ip: "10.203.0.1",
    generation: 1,
    revision: 1,
    state: "active",
    last_seen: "",
    report: {},
  };
  const api = vi.fn().mockResolvedValue(node),
    saved = vi.fn();
  const view = render(
    <NodeEditor node={node} api={api as API} saved={saved} />,
  );
  const user = userEvent.setup();
  await user.clear(screen.getByLabelText("节点名称"));
  await user.type(screen.getByLabelText("节点名称"), "edited");
  view.rerender(
    <NodeEditor
      node={{ ...node, revision: 2, name: "external change" }}
      api={api as API}
      saved={saved}
    />,
  );
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  expect(api).toHaveBeenCalledWith(
    "/nodes/node",
    expect.objectContaining({
      revision: 1,
      config: expect.objectContaining({ name: "edited" }),
    }),
    "PUT",
  );
});
