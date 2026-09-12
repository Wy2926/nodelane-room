import { defaultServer } from "../../native/api";

export const invitationLink = (code: string) => `${defaultServer}/join#${code}`;

export function invitationCode(value: string): string | undefined {
  const input = value.trim();
  if (/^[a-f0-9]{32}$/.test(input)) return input;
  const prefix = `${defaultServer}/join#`;
  const english = `${defaultServer}/en/join#`;
  for (const start of [prefix, english, "nodelane-room://join#"]) {
    if (input.startsWith(start) && /^[a-f0-9]{32}$/.test(input.slice(start.length)))
      return input.slice(start.length);
  }
  return undefined;
}
