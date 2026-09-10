import { useState } from "react";
import { useService } from "../native/use-service";
import { useActions, type Actions } from "./use-actions";
import { Shell } from "./Shell";
import { Feedback } from "./Feedback";
import type { Page } from "./navigation";
import { Empty } from "../shared/ui/Empty";
import { Setup } from "../features/device/Setup";
import { Settings } from "../features/device/Settings";
import { Diagnostics } from "../features/diagnostics/Diagnostics";
import { GameLibrary } from "../features/catalog/GameLibrary";
import { useCatalog } from "../features/catalog/use-catalog";
import { useRoom } from "../features/rooms/use-room";
import { RoomPage } from "../features/rooms/RoomPage";
import { RoomDialogs } from "../features/rooms/dialogs/RoomDialogs";
import type { Status, Failure } from "../shared/model";

export function App() {
  const service = useService();
  const [page, setPage] = useState<Page>("rooms");
  const [reload, setReload] = useState(0);
  const refreshAll = () => {
    service.refresh();
    setReload((n) => n + 1);
  };
  const actions = useActions(refreshAll);
  return (
    <Shell page={page} setPage={setPage} service={service}>
      <Feedback service={service} actions={actions} refreshAll={refreshAll} />
      {!service.status && !service.error && (
        <Empty title="正在连接本机服务">
          <p>读取设备与房间状态…</p>
        </Empty>
      )}
      {service.status && !service.status.device_id && !service.error && (
        <Setup actions={actions} />
      )}
      {service.status?.device_id && (
        <Session
          status={service.status}
          serviceError={service.error}
          page={page}
          setPage={setPage}
          actions={actions}
          reload={reload}
          refreshAll={refreshAll}
        />
      )}
    </Shell>
  );
}

function Session({
  status,
  serviceError,
  page,
  setPage,
  actions,
  reload,
  refreshAll,
}: {
  status: Status;
  serviceError?: Failure;
  page: Page;
  setPage: (page: Page) => void;
  actions: Actions;
  reload: number;
  refreshAll: () => void;
}) {
  const catalog = useCatalog(status.device_id, !!serviceError, reload);
  const view = useRoom(status, serviceError, reload);
  const usable = !serviceError && !actions.busy;
  const joined = () => {
    view.setSelected("");
    setPage("rooms");
  };
  return (
    <>
      {page === "rooms" && (
        <RoomPage
          {...{ view, status, actions, usable, catalog, setPage, refreshAll }}
        />
      )}
      {page === "games" && (
        <GameLibrary {...{ catalog, status, actions, usable, refreshAll }} />
      )}
      {page === "doctor" && (
        <Diagnostics {...{ status, actions, usable, serviceError }} />
      )}
      {page === "settings" && <Settings {...{ status, actions, usable }} />}
      <RoomDialogs
        actions={actions}
        status={status}
        gamesError={catalog.gamesError}
        serviceError={serviceError}
        onJoined={joined}
      />
    </>
  );
}
