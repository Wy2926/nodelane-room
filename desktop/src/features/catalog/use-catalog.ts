import { useEffect, useState } from "react";
import { rpc, failure } from "../../native/api";
import type { Game, Room, Failure } from "../../shared/model";
export function useCatalog(
  device: string | undefined,
  unavailable: boolean,
  reload: number,
) {
  const [games, setGames] = useState<Game[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [gamesError, setGamesError] = useState<Failure>();
  const [roomsError, setRoomsError] = useState<Failure>();
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    if (!device || unavailable) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    const load = async () => {
      const [g, r] = await Promise.allSettled([
        rpc<Game[]>({ action: "games" }),
        rpc<Room[]>({ action: "rooms" }),
      ]);
      if (cancelled) return;
      if (g.status === "fulfilled") {
        setGames(g.value);
        setGamesError(undefined);
      } else setGamesError(failure(g.reason));
      if (r.status === "fulfilled") {
        setRooms(r.value);
        setRoomsError(undefined);
      } else setRoomsError(failure(r.reason));
      setLoading(false);
      timer = setTimeout(load, 15000);
    };
    void load();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [device, unavailable, reload]);

  return { games, rooms, gamesError, roomsError, loading };
}
export type Catalog = ReturnType<typeof useCatalog>;
