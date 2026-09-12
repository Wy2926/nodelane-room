const copy = {
  zh: {
    loading: "正在加载可用版本…",
    ready: "选择系统架构和版本，获取完整客户端安装包。",
    empty: "暂未发布可下载版本，请稍后再来查看。",
    failed: "暂时无法加载版本，请重试。",
    unavailable: "下载服务尚未就绪，请稍后重试。",
    limited: "请求较频繁，请稍后重试。",
    noPlatform: "此系统架构暂无可下载版本。",
    choose: "暂无可用版本",
    recommended: "推荐",
    download: "下载安装包",
    preparing: "正在准备下载…",
    started: "已请求下载。若未开始，可选择下方地址或重新获取。",
    source: "下载地址",
    mirror: "备用地址",
    changed: "此版本已不可下载，请刷新版本列表。",
    downloadFailed: "暂时无法获取下载链接，请重试。",
    noNotes: "此版本暂无更新说明。",
    refresh: "刷新版本",
  },
  en: {
    loading: "Loading available releases…",
    ready:
      "Choose your architecture and version to get the full desktop installer.",
    empty: "No downloads have been published yet. Please check back later.",
    failed: "Unable to load releases. Please try again.",
    unavailable: "Downloads are not ready yet. Please try again later.",
    limited: "Too many requests. Please try again shortly.",
    noPlatform: "No download is available for this architecture yet.",
    choose: "No available version",
    recommended: "Recommended",
    download: "Download installer",
    preparing: "Preparing download…",
    started:
      "Download requested. If it does not start, use a link below or try again.",
    source: "Download link",
    mirror: "Mirror",
    changed: "This release is no longer available. Refresh the release list.",
    downloadFailed: "Unable to get a download link. Please try again.",
    noNotes: "No release notes are available for this version.",
    refresh: "Refresh releases",
  },
};

async function request(path) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15000);
  try {
    const response = await fetch(path, {
      headers: {
        "X-NodeLane-Contract": "interaction-1",
        Accept: "application/json",
      },
      credentials: "omit",
      cache: "no-store",
      signal: controller.signal,
    });
    const result = await response.json();
    if (
      result?.contract !== "interaction-1" ||
      !response.ok ||
      result.code !== "ok"
    ) {
      const error = new Error("download_request_failed");
      error.status = response.status;
      throw error;
    }
    return result.data;
  } finally {
    clearTimeout(timeout);
  }
}

function fileSize(bytes, language) {
  const units = ["B", "KB", "MB", "GB"];
  let index = 0;
  while (bytes >= 1024 && index < units.length - 1) {
    bytes /= 1024;
    index++;
  }
  return `${new Intl.NumberFormat(language, { maximumFractionDigits: 1 }).format(bytes)} ${units[index]}`;
}

export async function initDownloads(root = document) {
  const section = root.querySelector("[data-downloads]");
  if (!section) return;
  const language = root.documentElement.lang;
  const text = copy[language.startsWith("en") ? "en" : "zh"];
  const status = section.querySelector("[data-download-status]");
  const refresh = section.querySelector("[data-download-refresh]");
  let releases = [];
  let loaded = false;
  let loading = false;
  const cards = Array.from(
    section.querySelectorAll("[data-platform]"),
    (element) => ({
      element,
      os: element.dataset.platform,
      arch: element.querySelector("[data-architecture]"),
      version: element.querySelector("[data-version]"),
      button: element.querySelector("[data-download-button]"),
      status: element.querySelector("[data-platform-status]"),
      details: element.querySelector("[data-release-details]"),
      mirrors: element.querySelector("[data-download-mirrors]"),
      busy: false,
      stale: false,
    }),
  );

  function selection(card) {
    return releases.find(
      (release) =>
        release.id === card.version.value &&
        release.os === card.os &&
        release.arch === card.arch.value,
    );
  }

  function renderSelection(card) {
    const release = selection(card);
    card.mirrors.replaceChildren();
    card.stale = false;
    card.details.hidden = !release;
    card.button.disabled = !release || loading || card.busy;
    card.button.textContent = card.busy ? text.preparing : text.download;
    card.status.textContent = loaded && !release ? text.noPlatform : "";
    if (release) {
      card.element.querySelector("[data-file-name]").textContent =
        release.target;
      card.element.querySelector("[data-file-size]").textContent = fileSize(
        release.size,
        language,
      );
      card.element.querySelector("[data-file-hash]").textContent =
        release.sha256;
      card.element.querySelector("[data-release-notes]").textContent =
        release.notes || text.noNotes;
    }
  }

  function renderVersions(card, previous = "") {
    const available = releases.filter(
      (release) => release.os === card.os && release.arch === card.arch.value,
    );
    card.version.replaceChildren();
    for (const release of available) {
      const option = root.createElement("option");
      option.value = release.id;
      option.textContent = `${release.version}${release.recommended ? ` · ${text.recommended}` : ""}`;
      card.version.append(option);
    }
    if (!available.length) {
      const option = root.createElement("option");
      option.textContent = loading ? text.loading : text.choose;
      option.value = "";
      card.version.append(option);
    } else if (available.some((release) => release.id === previous)) {
      card.version.value = previous;
    }
    card.version.disabled = !available.length || loading || card.busy;
    renderSelection(card);
  }

  function errorMessage(error, fallback) {
    if (error.status === 503) return text.unavailable;
    if (error.status === 429) return text.limited;
    return fallback;
  }

  async function load() {
    if (loading) return;
    loading = true;
    status.textContent = text.loading;
    section.setAttribute("aria-busy", "true");
    refresh.disabled = true;
    for (const card of cards) {
      card.arch.disabled = true;
      card.version.disabled = true;
      card.button.disabled = true;
    }
    const previous = cards.map((card) => card.version.value);
    try {
      const data = await request("/v2/downloads");
      if (!Array.isArray(data?.releases))
        throw new Error("download_response_invalid");
      releases = data.releases;
      loaded = true;
      status.textContent = releases.length ? text.ready : text.empty;
    } catch (error) {
      releases = [];
      loaded = false;
      status.textContent = errorMessage(error, text.failed);
    } finally {
      loading = false;
      section.setAttribute("aria-busy", "false");
      refresh.disabled = false;
      refresh.hidden = false;
      refresh.textContent = text.refresh;
      cards.forEach((card, index) => {
        card.arch.disabled = card.busy;
        renderVersions(card, previous[index]);
      });
    }
  }

  refresh.addEventListener("click", load);
  for (const card of cards) {
    card.arch.addEventListener("change", () => renderVersions(card));
    card.version.addEventListener("change", () => renderSelection(card));
    card.button.addEventListener("click", async () => {
      const release = selection(card);
      if (!release || card.busy || loading || card.stale) return;
      card.busy = true;
      card.arch.disabled = card.version.disabled = card.button.disabled = true;
      refresh.disabled = true;
      card.button.textContent = text.preparing;
      card.status.textContent = "";
      card.mirrors.replaceChildren();
      try {
        const data = await request(
          `/v2/downloads/${encodeURIComponent(release.id)}`,
        );
        const urls = data?.urls?.map((value) => new URL(value));
        if (
          data.release?.id !== release.id ||
          !urls?.length ||
          urls.some(
            (url) => url.protocol !== "https:" || url.username || url.password,
          )
        ) {
          throw new Error("download_response_invalid");
        }
        for (const [index, url] of urls.entries()) {
          const link = root.createElement("a");
          link.href = url.href;
          link.target = "_blank";
          link.rel = "noopener noreferrer";
          link.referrerPolicy = "no-referrer";
          link.textContent =
            index === 0 ? text.source : `${text.mirror} ${index + 1}`;
          card.mirrors.append(link);
        }
        card.mirrors.firstElementChild.click();
        card.status.textContent = text.started;
      } catch (error) {
        card.stale = error.status === 404;
        card.status.textContent =
          error.status === 404
            ? text.changed
            : errorMessage(error, text.downloadFailed);
      } finally {
        card.busy = false;
        card.arch.disabled = card.version.disabled = loading;
        card.button.disabled = loading || card.stale;
        card.button.textContent = text.download;
        refresh.disabled = loading || cards.some((other) => other.busy);
      }
    });
  }
  await load();
}

if (typeof document !== "undefined") void initDownloads();
