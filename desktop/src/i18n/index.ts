import { useSyncExternalStore } from "react";
import zhCN from "./locales/zh-CN.json";
import enUS from "./locales/en-US.json";

export type Language = "zh-CN" | "en-US";
export type MessageKey = keyof typeof zhCN;
export const languageStorageKey = "nlroom.language.v1";
const dictionaries: Record<Language, Record<MessageKey, string>> = {
  "zh-CN": zhCN,
  "en-US": enUS,
};
const listeners = new Set<() => void>();
let selected: Language | undefined;
let sessionOnly = false;

export function isLanguage(value: unknown): value is Language {
  return value === "zh-CN" || value === "en-US";
}

function snapshot() {
  if (sessionOnly) return selected;
  try {
    const saved = localStorage.getItem(languageStorageKey);
    selected = isLanguage(saved) ? saved : undefined;
  } catch {
    /* Keep the session preference when storage is unavailable. */
  }
  return selected;
}
snapshot();

function subscribe(listener: () => void) {
  listeners.add(listener);
  const changed = (event: StorageEvent) => {
    if (event.key === null || event.key === languageStorageKey) listener();
  };
  window.addEventListener("storage", changed);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", changed);
  };
}

export function setLanguage(language: Language) {
  selected = language;
  try {
    localStorage.setItem(languageStorageKey, language);
    sessionOnly = false;
  } catch {
    sessionOnly = true;
  }
  document.documentElement.lang = language;
  listeners.forEach((listener) => listener());
}

export const useLanguage = () => useSyncExternalStore(subscribe, snapshot);
export const getLanguage = (): Language => selected ?? "zh-CN";

export function translate(
  language: Language,
  key: MessageKey,
  values: Record<string, string | number> = {},
): string {
  return dictionaries[language][key].replace(
    /\{(\w+)\}/g,
    (placeholder, name: string) =>
      Object.hasOwn(values, name) ? String(values[name]) : placeholder,
  );
}

export const t = (key: MessageKey, values?: Record<string, string | number>) =>
  translate(getLanguage(), key, values);
