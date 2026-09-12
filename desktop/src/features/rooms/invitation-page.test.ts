import { afterEach, beforeEach, expect, test, vi, type MockInstance } from "vitest";

const { readFileSync } = await vi.importActual<{
  readFileSync: (path: string, encoding: "utf8") => string;
}>("node:fs");
const { fileURLToPath } = await vi.importActual<{
  fileURLToPath: (url: string) => string;
}>("node:url");
const moduleURL = import.meta.url;
const source = (name: string) => readFileSync(
  fileURLToPath(new URL(`../../../../internal/control/siteweb/${name}`, moduleURL).href),
  "utf8",
);
const script = source("assets/invitation.js");
const chinese = source("join.html");
const english = source("en/join.html");

const code = "0123456789abcdef0123456789abcdef";
const serverTime = "2026-09-12T06:00:00Z";
const fetcher = vi.fn<typeof fetch>();
let subscriptions: MockInstance<typeof window.addEventListener>;
const element = <T extends HTMLElement = HTMLElement>(name: string) =>
  document.querySelector<T>(`[data-invite-${name}]`)!;
const open = () => element<HTMLAnchorElement>("open");
function response(available = true) {
  return {
    ok: available,
    json: async () => ({
      code: available ? "ok" : "invite_unusable",
      server_time: serverTime,
      data: {
        name: "<img src=x onerror=alert(1)>",
        game_name: "Minecraft",
        member_count: 2,
        capacity: 8,
        room_expires_at: "2026-09-13T06:00:00Z",
        invite_expires_at: "2026-09-12T06:05:00Z",
      },
    }),
  } as Response;
}
function start(language = "zh-CN", fragment = code) {
  document.documentElement.lang = language;
  document.body.innerHTML = `<a class="language-switch"></a>${language === "en" ? english : chinese}`
    .replace(/{{[^}]+}}/g, "");
  history.replaceState({}, "", `/join#${fragment}`);
  subscriptions = vi.spyOn(window, "addEventListener");
  new Function(script)();
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(serverTime));
  fetcher.mockReset();
  fetcher.mockResolvedValue(response());
  vi.stubGlobal("fetch", fetcher);
});
afterEach(() => {
  window.dispatchEvent(new Event("pagehide"));
  for (const [name, callback, options] of subscriptions?.mock.calls || [])
    window.removeEventListener(name, callback, options);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  document.body.innerHTML = "";
  history.replaceState({}, "", "/");
});

test.each(["zh-CN", "en"])("%s invitation previews via POST, opens the client, and preserves the page while downloading", async (language) => {
  let complete!: (value: Response) => void;
  fetcher.mockImplementationOnce(() => new Promise((resolve) => { complete = resolve; }));
  start(language);
  expect(document.querySelector("[data-invitation]")?.getAttribute("aria-busy")).toBe("true");
  expect(open().hidden).toBe(true);
  expect(fetcher).toHaveBeenCalledExactlyOnceWith("/v2/invitations/preview", expect.objectContaining({
    method: "POST",
    credentials: "omit",
    cache: "no-store",
    referrerPolicy: "no-referrer",
    headers: { "Content-Type": "application/json", "X-NodeLane-Contract": "interaction-1" },
    body: JSON.stringify({ code }),
  }));
  complete(response());
  await vi.advanceTimersByTimeAsync(0);
  expect(document.querySelector("[data-invitation]")?.getAttribute("aria-busy")).toBe("false");
  expect(element("room").hidden).toBe(false);
  expect(element("name").textContent).toBe("<img src=x onerror=alert(1)>");
  expect(element("name").children).toHaveLength(0);
  expect(element("game").textContent).toBe("Minecraft");
  expect(element("members").textContent).toContain("2 / 8");
  expect(element("expiry").textContent).not.toBe("");
  expect(open().hidden).toBe(false);
  expect(open().getAttribute("href")).toBe(`nodelane-room://join#${code}`);
  const download = document.querySelector<HTMLAnchorElement>(`a[href="${language === "en" ? "/en" : ""}/download"]`)!;
  expect(download.target).toBe("_blank");
  expect(download.rel.split(" ")).toEqual(expect.arrayContaining(["noopener", "noreferrer"]));
  expect(document.querySelector(".language-switch")?.getAttribute("href")).toBe(`${language === "en" ? "/join" : "/en/join"}#${code}`);
  open().addEventListener("click", (event) => event.preventDefault());
  open().click();
  expect(element("help").textContent).toContain(language === "en" ? "Allow your browser" : "请在浏览器提示中允许");
});

test("an invalid fragment never requests preview or exposes an open-client action", async () => {
  start("zh-CN", "not-an-invitation");
  await vi.advanceTimersByTimeAsync(0);
  expect(fetcher).not.toHaveBeenCalled();
  expect(open().hidden).toBe(true);
  expect(open().hasAttribute("href")).toBe(false);
  expect(element("status").textContent).toContain("邀请已失效");
});

test("a revoked invitation removes an earlier preview and disables opening during refresh", async () => {
  start();
  await vi.advanceTimersByTimeAsync(0);
  expect(open().hidden).toBe(false);
  fetcher.mockResolvedValueOnce(response(false));
  element("retry").click();
  expect(open().hidden).toBe(true);
  expect(open().hasAttribute("href")).toBe(false);
  await vi.advanceTimersByTimeAsync(0);
  expect(element("room").hidden).toBe(true);
  expect(element("status").textContent).toContain("邀请已失效");
});

test("local clock skew cannot expire a valid link immediately or trigger a request loop", async () => {
  vi.setSystemTime(new Date("2050-01-01T00:00:00Z"));
  start();
  await vi.advanceTimersByTimeAsync(60000);
  expect(open().hidden).toBe(false);
  expect(fetcher).toHaveBeenCalledOnce();
  await vi.advanceTimersByTimeAsync(240000);
  expect(open().hidden).toBe(true);
  expect(open().hasAttribute("href")).toBe(false);
  expect(element("status").textContent).toContain("请刷新");
  await vi.advanceTimersByTimeAsync(600000);
  expect(fetcher).toHaveBeenCalledOnce();
});
