import { afterEach, expect, test, vi } from "vitest";
import zhCN from "./locales/zh-CN.json";
import enUS from "./locales/en-US.json";
import { getLanguage, setLanguage, t, translate, type MessageKey } from ".";
import { formatTime } from "../shared/time";

afterEach(() => {
  vi.restoreAllMocks();
  setLanguage("zh-CN");
});

test("separate dictionaries have identical keys and interpolation parameters", () => {
  expect(Object.keys(enUS).sort()).toEqual(Object.keys(zhCN).sort());
  for (const key of Object.keys(zhCN) as MessageKey[]) {
    expect(enUS[key].trim(), key).not.toBe("");
    expect(enUS[key], key).not.toMatch(/\p{Script=Han}/u);
    expect(enUS[key].match(/\{\w+\}/g)?.sort(), key).toEqual(
      zhCN[key].match(/\{\w+\}/g)?.sort(),
    );
  }
});

test("production UI keeps Chinese messages in language dictionaries", () => {
  const sources = import.meta.glob<string>(
    ["../**/*.ts", "../**/*.tsx", "!../**/*.test.*", "!../preview.tsx"],
    { query: "?raw", import: "default", eager: true },
  );
  for (const [file, source] of Object.entries(sources))
    expect(source, file).not.toMatch(/\p{Script=Han}/u);
});

test("interpolation keeps user content literal and dates use the selected locale", () => {
  expect(
    translate("en-US", "confirmation.action", { action: "$& <room> {time}" }),
  ).toBe("Confirm: $& <room> {time}");
  const date = "2026-09-11T06:30:00Z";
  const spy = vi.spyOn(Date.prototype, "toLocaleString");
  setLanguage("en-US");
  formatTime(date);
  expect(spy).toHaveBeenCalledWith("en-US", expect.any(Object));
  setLanguage("zh-CN");
  formatTime(date);
  expect(spy).toHaveBeenLastCalledWith("zh-CN", expect.any(Object));
});

test("a storage failure still allows a session language choice", () => {
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("storage blocked");
  });
  setLanguage("en-US");
  expect(getLanguage()).toBe("en-US");
  expect(t("navigation.settings")).toBe("Settings");
});
