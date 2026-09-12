import { useQuery } from "../../native/use-query";
import type { Game, Room } from "../../shared/model";

export function useCatalog(
  device: string | undefined,
  unavailable: boolean,
  reload: number,
  instance?: string,
) {
  const options = {
    enabled: !!device && !unavailable,
    reload,
    scope: `${instance}:${device}`,
  };
  const games = useQuery<Game[]>({ action: "games" }, 15000, options);
  const rooms = useQuery<{ rooms: Room[]; truncated: boolean }>(
    { action: "rooms" },
    15000,
    options,
  );
  return {
    games: games.data || [],
    rooms: rooms.data?.rooms || [],
    truncated: rooms.data?.truncated || false,
    gamesError: games.error,
    roomsError: rooms.error,
    loading: games.loading || rooms.loading,
  };
}
export type Catalog = ReturnType<typeof useCatalog>;
