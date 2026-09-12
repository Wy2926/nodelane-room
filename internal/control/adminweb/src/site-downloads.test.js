// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { afterEach, expect, it, vi } from "vitest";
import { waitFor } from "@testing-library/react";
import { initDownloads } from "../../siteweb/assets/downloads.js";

const release = {
  id: "win64",
  version: "1.2.0",
  os: "windows",
  arch: "amd64",
  target: "nodelane-room-1.2.0-windows-amd64.exe",
  size: 104857600,
  sha256: "a".repeat(64),
  notes: "A new release",
  recommended: true,
};
const reply = (data, status = 200) =>
  new Response(
    JSON.stringify({
      contract: "interaction-1",
      code: status === 200 ? "ok" : "resource_not_found",
      data,
    }),
    { status },
  );

function page(language = "en") {
  document.documentElement.lang = language;
  document.body.innerHTML = readFileSync(
    resolve(
      dirname(fileURLToPath(import.meta.url)),
      `../../siteweb/${language === "en" ? "en/" : ""}download.html`,
    ),
    "utf8",
  ).replaceAll(/{{.*?}}/g, "");
}
function card(os = "windows") {
  return document.querySelector(`[data-platform="${os}"]`);
}
function change(selector, value, element = card()) {
  const select = element.querySelector(selector);
  select.value = value;
  select.dispatchEvent(new Event("change"));
}

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("uses the public contract and switches platform versions, notes, size and hash safely", async () => {
  page();
  const arm = {
    ...release,
    id: "win-arm",
    arch: "arm64",
    version: "1.1.0",
    recommended: false,
    notes: "<img src=x onerror=alert(1)>",
  };
  const linux = { ...release, id: "linux64", os: "linux", target: "room.deb" };
  const fetch = vi
    .fn()
    .mockResolvedValue(reply({ releases: [release, arm, linux] }));
  vi.stubGlobal("fetch", fetch);
  await initDownloads();
  expect(fetch).toHaveBeenCalledWith(
    "/v2/downloads",
    expect.objectContaining({
      headers: {
        "X-NodeLane-Contract": "interaction-1",
        Accept: "application/json",
      },
      credentials: "omit",
      cache: "no-store",
    }),
  );
  expect(card().querySelector("[data-version]").textContent).toContain(
    "1.2.0 · Recommended",
  );
  expect(card().querySelector("[data-file-size]").textContent).toBe("100 MB");
  expect(card().querySelector("[data-file-hash]").textContent).toBe(
    release.sha256,
  );
  expect(card("linux").querySelector("[data-file-name]").textContent).toBe(
    "room.deb",
  );
  change("[data-architecture]", "arm64");
  expect(card().querySelector("[data-version]").value).toBe("win-arm");
  expect(card().querySelector("[data-release-notes]").textContent).toBe(
    arm.notes,
  );
  expect(card().querySelector("[data-release-notes] img")).toBeNull();
  expect(card().querySelector("[data-download-button]").disabled).toBe(false);
  change("[data-architecture]", "arm64", card("linux"));
  expect(card("linux").querySelector("[data-download-button]").disabled).toBe(
    true,
  );
  expect(
    card("linux").querySelector("[data-platform-status]").textContent,
  ).toContain("No download");
});

it.each(["en", "zh-CN"])(
  "shows a localized empty state without a GitHub download fallback (%s)",
  async (language) => {
    page(language);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(reply({ releases: [] })));
    await initDownloads();
    expect(
      document.querySelector("[data-download-status]").textContent,
    ).toContain(language === "en" ? "No downloads" : "暂未发布");
    expect(
      [...document.querySelectorAll("[data-download-button]")].every(
        (button) => button.disabled,
      ),
    ).toBe(true);
    expect(document.querySelector('a[href$="/releases"]')).toBeNull();
    expect(document.querySelector("[data-release-details]").hidden).toBe(true);
  },
);

it("recovers from an unavailable service with an explicit refresh", async () => {
  page();
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(reply(null, 503))
    .mockResolvedValueOnce(reply({ releases: [release] }));
  vi.stubGlobal("fetch", fetch);
  await initDownloads();
  expect(
    document.querySelector("[data-download-status]").textContent,
  ).toContain("not ready");
  expect(card().querySelector("[data-download-button]").disabled).toBe(true);
  document.querySelector("[data-download-refresh]").click();
  await waitFor(() =>
    expect(card().querySelector("[data-download-button]").disabled).toBe(false),
  );
});

it("refreshes the signed URL for each click, locks selection and prevents duplicate requests", async () => {
  page();
  let resolveDownload;
  const urls = [];
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(reply({ releases: [release] }))
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveDownload = resolve;
        }),
    )
    .mockResolvedValueOnce(
      reply({
        release,
        urls: ["https://downloads.example.test/file?signature=second"],
      }),
    );
  vi.stubGlobal("fetch", fetch);
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
    function () {
      urls.push(this.href);
    },
  );
  await initDownloads();
  const button = card().querySelector("[data-download-button]");
  button.click();
  button.click();
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(card().querySelector("[data-architecture]").disabled).toBe(true);
  expect(card().querySelector("[data-version]").disabled).toBe(true);
  resolveDownload(
    reply({
      release,
      urls: ["https://downloads.example.test/file?signature=first"],
    }),
  );
  await waitFor(() => expect(button.disabled).toBe(false));
  button.click();
  await waitFor(() => expect(urls).toHaveLength(2));
  expect(urls).toEqual([
    "https://downloads.example.test/file?signature=first",
    "https://downloads.example.test/file?signature=second",
  ]);
  expect(fetch.mock.calls[1][0]).toBe("/v2/downloads/win64");
  expect(document.querySelector('a[href*="signature="]').href).toContain(
    "second",
  );
  expect(document.querySelector('a[href*="signature="]').target).toBe("_blank");
  change("[data-architecture]", "arm64");
  expect(document.querySelector('a[href*="signature="]')).toBeNull();
});

it("disables an unavailable release and removes it after refreshing the list", async () => {
  page("zh-CN");
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(reply({ releases: [release] }))
      .mockResolvedValueOnce(reply(null, 404))
      .mockResolvedValueOnce(reply({ releases: [] })),
  );
  const navigate = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  await initDownloads();
  card().querySelector("[data-download-button]").click();
  await waitFor(() =>
    expect(
      card().querySelector("[data-platform-status]").textContent,
    ).toContain("已不可下载"),
  );
  expect(card().querySelector("[data-download-button]").disabled).toBe(true);
  expect(navigate).not.toHaveBeenCalled();
  document.querySelector("[data-download-refresh]").click();
  await waitFor(() =>
    expect(
      document.querySelector("[data-download-status]").textContent,
    ).toContain("暂未发布"),
  );
  expect(card().querySelector("[data-release-details]").hidden).toBe(true);
});

it("offers every verified mirror in a new tab while retaining the download page", async () => {
  page();
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(reply({ releases: [release] }))
      .mockResolvedValueOnce(
        reply({
          release,
          urls: [
            "https://primary.example.test/file",
            "https://backup.example.test/file",
          ],
        }),
      ),
  );
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  await initDownloads();
  card().querySelector("[data-download-button]").click();
  await waitFor(() =>
    expect(card().querySelectorAll("[data-download-mirrors] a")).toHaveLength(
      2,
    ),
  );
  const links = [...card().querySelectorAll("[data-download-mirrors] a")];
  expect(links[1].href).toBe("https://backup.example.test/file");
  expect(links[1].textContent).toBe("Mirror 2");
  expect(
    links.every(
      (link) => link.target === "_blank" && link.rel === "noopener noreferrer",
    ),
  ).toBe(true);
});

it.each([
  "javascript:alert(1)",
  "http://downloads.example.test/file",
  "https://user:password@downloads.example.test/file",
])("rejects an unsafe download URL (%s)", async (url) => {
  page();
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(reply({ releases: [release] }))
      .mockResolvedValueOnce(reply({ release, urls: [url] })),
  );
  const navigate = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  await initDownloads();
  card().querySelector("[data-download-button]").click();
  await waitFor(() =>
    expect(
      card().querySelector("[data-platform-status]").textContent,
    ).toContain("Unable to get"),
  );
  expect(navigate).not.toHaveBeenCalled();
});

it("times out a stalled request and allows retry", async () => {
  page();
  vi.useFakeTimers();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      (_path, { signal }) =>
        new Promise((_resolve, reject) => {
          signal.addEventListener("abort", () =>
            reject(new DOMException("Aborted", "AbortError")),
          );
        }),
    ),
  );
  const loading = initDownloads();
  await vi.advanceTimersByTimeAsync(15000);
  await loading;
  expect(
    document.querySelector("[data-download-status]").textContent,
  ).toContain("Unable to load");
  expect(document.querySelector("[data-download-refresh]").disabled).toBe(
    false,
  );
});
