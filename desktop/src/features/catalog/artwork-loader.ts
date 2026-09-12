import { invoke } from "@tauri-apps/api/core";
// Only public artwork is cached, with bounded requests and retained bytes.
const images = new Map<string, string>();
let bytes = 0;
let active = 0;
const queue: (() => void)[] = [];
export async function artwork(game: string): Promise<string> {
  const key = game;
  const existing = images.get(key);
  if (existing) {
    images.delete(key);
    images.set(key, existing);
    return existing;
  }
  await new Promise<void>((resolve) => {
    const start = () => {
      active++;
      resolve();
    };
    if (active < 2) start();
    else if (queue.length < 16) queue.push(start);
    else throw new Error("artwork queue full");
  });
  try {
    const data = await invoke<string>("game_image", { game, kind: "cover" });
    while (bytes + data.length > 16 * 1024 * 1024 && images.size) {
      const first = images.keys().next().value!;
      bytes -= images.get(first)!.length;
      images.delete(first);
    }
    if (!images.has(key)) {
      images.set(key, data);
      bytes += data.length;
    }
    return data;
  } finally {
    active--;
    queue.shift()?.();
  }
}
