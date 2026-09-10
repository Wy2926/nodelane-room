import { useEffect, useState } from "react";
import { rpc, failure } from "../../native/api";
import { fresh } from "../../shared/time";
import type { Status, Management, Failure } from "../../shared/model";
export function useRoom(
  status: Status,
  serviceError: Failure | undefined,
  reload: number,
) {
  const [selected, setSelected] = useState("");
  const [management, setManagement] = useState<Management>();
  const [managementError, setManagementError] = useState<Failure>();
  const [managementAt, setManagementAt] = useState(0);
  const hasIdentity = !!status.device_id;
  const activeRoom =
    status?.selected_room && status.room?.id === status.selected_room
      ? status.room
      : undefined;
  const isCurrent = !!activeRoom && (!selected || selected === activeRoom.id);
  const room = isCurrent ? activeRoom : management?.room;
  const game = isCurrent ? status?.game : management?.game;
  const members = isCurrent ? status?.members || [] : management?.members || [];
  const endpoints = isCurrent
    ? status?.endpoints || []
    : management?.endpoints || [];
  const roomFresh = isCurrent
    ? fresh(status?.snapshot_at) && !serviceError
    : !managementError && Date.now() - managementAt < 15000;

  const owner = room?.owner_id === status?.device_id;
  const desiredPorts = status?.ports || [];
  const pendingRemovals = isCurrent
    ? endpoints.filter(
        (e) =>
          e.device_id === status?.device_id &&
          Date.parse(e.expires_at) > Date.now() &&
          !desiredPorts.some(
            (p) => p.protocol === e.protocol && p.port === e.port,
          ),
      )
    : [];
  const localPorts = [...desiredPorts, ...pendingRemovals];
  const canManage =
    !!room &&
    owner &&
    roomFresh &&
    !room.closed &&
    Date.parse(room.expires_at) > Date.now();
  useEffect(() => {
    if (
      !selected ||
      selected === activeRoom?.id ||
      !hasIdentity ||
      serviceError
    ) {
      setManagement(undefined);
      return;
    }
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    setManagement(undefined);
    setManagementError(undefined);
    const load = async () => {
      try {
        const out = await rpc<Management>({ action: "manage", room: selected });
        if (!cancelled) {
          setManagement(out);
          setManagementError(undefined);
          setManagementAt(Date.now());
        }
      } catch (e) {
        if (!cancelled) setManagementError(failure(e));
      }
      if (!cancelled) timer = setTimeout(load, 7000);
    };
    void load();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [selected, activeRoom?.id, hasIdentity, !!serviceError, reload]);

  return {
    selected,
    setSelected,
    activeRoom,
    isCurrent,
    room,
    game,
    members,
    endpoints,
    roomFresh,
    owner,
    canManage,
    managementError,
    localPorts,
    desiredPorts,
  };
}
export type RoomView = ReturnType<typeof useRoom>;
