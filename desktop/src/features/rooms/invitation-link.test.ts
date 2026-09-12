import { expect, test } from "vitest";
import { invitationCode, invitationLink } from "./invitation-link";

const code = "0123456789abcdef0123456789abcdef";
test("copied browser invitation and native invitation round trip", () => {
  expect(invitationCode(invitationLink(code))).toBe(code);
  expect(invitationCode(`https://room.nodelane.net/en/join#${code}`)).toBe(code);
  expect(invitationCode(`nodelane-room://join#${code}`)).toBe(code);
  expect(invitationCode(code)).toBe(code);
});
test("invitation cannot override control origin or add command arguments", () => {
  for (const value of [`https://evil.test/join#${code}`, `https://room.nodelane.net.evil.test/join#${code}`, `nodelane-room://join#${code} --server=x`, `nodelane-room://join?server=x#${code}`, `https://room.nodelane.net/join?code=${code}`, "invalid", ""]) {
    expect(invitationCode(value)).toBeUndefined();
  }
});
