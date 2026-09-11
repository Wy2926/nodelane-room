export type Page = "rooms" | "games" | "doctor" | "settings";
export const titles = {
  rooms: "navigation.myRooms",
  games: "navigation.gameLibrary",
  doctor: "navigation.diagnostics",
  settings: "navigation.settings",
} as const;
