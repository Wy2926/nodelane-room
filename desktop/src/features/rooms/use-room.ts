import { useState } from "react";
import { useQuery } from "../../native/use-query";
import { fresh } from "../../shared/time";
import type { Status, Management, Failure } from "../../shared/model";

export function useRoom(
  status: Status,
  serviceError: Failure | undefined,
  reload: number,
) {
  const [selected, setSelected] = useState("");
  const activeRoom =
    status.selected_room && status.room?.id === status.selected_room
      ? status.room
      : undefined;
  const isCurrent = !!activeRoom && (!selected || selected === activeRoom.id);
  const management = useQuery<Management>(
    { action: "manage", room: selected },
    7000,
    {
      enabled: !!selected && !isCurrent && !!status.device_id && !serviceError,
      scope: `${status.service_instance_id}:${status.device_id}:${status.user?.id}`,
      reload,
    },
  );
  const room = isCurrent ? activeRoom : management.data?.room;
  const game = isCurrent ? status.game : management.data?.game;
  const members = (isCurrent ? status.members : management.data?.members) || [];
  const roomFresh = isCurrent
    ? fresh(status.snapshot_at) && !serviceError
    : !serviceError &&
      !management.error &&
      Date.now() - management.updatedAt < 15000;
  const owner = room?.owner_user_id === status.user?.id;
  const canManage =
    !!room &&
    owner &&
    roomFresh &&
    !room.closed &&
    Date.parse(room.expires_at) > Date.now() &&
    (isCurrent
      ? status.permissions.manage
      : management.data?.permissions.manage);
  return {
    selected,
    setSelected,
    activeRoom,
    isCurrent,
    room,
    game,
    members,
    roomFresh,
    owner,
    canManage,
    loading: management.loading,
    managementError: isCurrent ? undefined : management.error,
  };
}
export type RoomView = ReturnType<typeof useRoom>;
