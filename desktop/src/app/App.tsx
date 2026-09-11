import { t, useLanguage } from "../i18n";
import { useEffect, useState } from "react";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { LanguageSelection } from "../i18n/LanguageSelection";
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
  const language = useLanguage();
  useEffect(() => {
    if (!language) return;
    document.documentElement.lang = language;
    if (isTauri()) void invoke("set_language", { language }).catch(() => undefined);
  }, [language]);
  return language ? <Client /> : <LanguageSelection />;
}

function Client() {
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
      {!service.status && !service.error && page !== "settings" && page !== "doctor" && (
        <Empty title={t("app.connectingToTheLocalService")}>
          <p>{t("app.readingDeviceAndRoomStatus")}</p>
        </Empty>
      )}
      {service.status && !service.status.device_id && !service.error && page !== "settings" && page !== "doctor" && (
        <Setup actions={actions} />
      )}
      {page === "settings" && <Settings status={service.status} actions={actions} usable={!!service.status && !service.error && !actions.busy} />}
      {page === "doctor" && <Diagnostics status={service.status} actions={actions} usable={!!service.status && !service.error && !actions.busy} serviceError={service.error} />}
      {service.status && !service.status.device_id && <RoomDialogs actions={actions} status={service.status} serviceError={service.error} onJoined={() => setPage("rooms")} />}
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
