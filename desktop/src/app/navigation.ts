export type Page = "rooms" | "doctor" | "settings";
export const titles = {
  rooms: "navigation.myRooms",
  doctor: "navigation.diagnostics",
  settings: "navigation.settings",
} as const;
