import { getLanguage } from "../i18n";

export const formatTime = (value?: string) =>
  value
    ? new Date(value).toLocaleString(getLanguage(), {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      })
    : "—";
export const fresh = (time?: string) =>
  !!time &&
  Date.now() - Date.parse(time) < 15000 &&
  Date.now() - Date.parse(time) > -30000;
