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
import { Account } from "../features/device/Account";
import { Settings } from "../features/device/Settings";
import { Diagnostics } from "../features/diagnostics/Diagnostics";
import { useCatalog } from "../features/catalog/use-catalog";
import { useRoom } from "../features/rooms/use-room";
import { RoomPage } from "../features/rooms/RoomPage";
import { RoomDialogs } from "../features/rooms/dialogs/RoomDialogs";
import type { Status, Failure } from "../shared/model";
import { connectionView, roomActionBlock, type Recovery } from "./experience";
import { RenderBoundary } from "./RenderBoundary";

export function App() {
  const language = useLanguage();
  useEffect(() => {
    if (!language) return;
    document.documentElement.lang = language;
    if (isTauri())
      void invoke("set_language", { language }).catch(() => undefined);
  }, [language]);
  return (
    <RenderBoundary>
      {language ? <Client /> : <LanguageSelection />}
    </RenderBoundary>
  );
}

function Client() {
  const service = useService();
  const [page, setPage] = useState<Page>("rooms");
  const [reload, setReload] = useState(0);
  const [settingsSection, setSettingsSection] = useState<
    "preferences" | "device" | "updates"
  >("preferences");
  const refreshAll = () => {
    service.refresh();
    setReload((n) => n + 1);
  };
  const unavailable = !!service.error || !!service.stale;
  const actions = useActions(refreshAll, service.status?.service_instance_id, {
    operations: service.status?.pending_operations || [],
    unavailable,
    roomBlocked:
      !!service.status && !!roomActionBlock(service.status, unavailable, false),
    roomCreation: service.status?.room_creation,
    updateRequired:
      !!service.status?.update?.required ||
      service.status?.update?.state === "installing",
  });
  const recover = (action: Recovery) => {
    if (action === "refresh") {
      refreshAll();
      return;
    }
    if (action === "network") {
      void actions.perform(t("experience.retry"), { action: "network-retry" });
      return;
    }
    if (action === "diagnostics") {
      setPage("doctor");
      return;
    }
    setSettingsSection(action === "account" ? "device" : "updates");
    setPage("settings");
  };
  const serviceError =
    service.error ||
    (service.stale
      ? { code: "local_state_stale", error: t("experience.staleHelp") }
      : undefined);
  const identityNeedsAttention =
    !!service.status?.device_id && service.status.identity !== "active";
  const systemPage = page === "settings" || page === "doctor";
  return (
    <Shell
      page={page}
      setPage={setPage}
      openProfile={() => {
        setSettingsSection("device");
        setPage("settings");
      }}
      service={service}
      actions={actions}
    >
      <Feedback
        service={service}
        actions={actions}
        refreshAll={refreshAll}
        onRecover={recover}
      />
      {!service.status &&
        !serviceError &&
        page !== "settings" &&
        page !== "doctor" && (
          <Empty title={t("app.connectingToTheLocalService")}>
            <p>{t("app.readingDeviceAndRoomStatus")}</p>
          </Empty>
        )}
      {service.status &&
        !service.status.device_id &&
        !serviceError &&
        page !== "settings" &&
        page !== "doctor" && <Setup actions={actions} />}
      {page === "settings" && (
        <Settings
          status={service.status}
          actions={actions}
          category={settingsSection}
          setCategory={setSettingsSection}
          serviceUnavailable={unavailable}
        />
      )}
      {page === "doctor" && (
        <Diagnostics
          status={service.status}
          actions={actions}
          usable={!!service.status && !unavailable && !actions.busy}
          serviceError={serviceError}
        />
      )}
      {service.status &&
        (!service.status.device_id || identityNeedsAttention) && (
          <RoomDialogs
            actions={actions}
            status={service.status}
            serviceError={serviceError}
            onRecover={recover}
            onJoined={() => setPage("rooms")}
          />
        )}
      {identityNeedsAttention && !systemPage && (
        <div className="account-page">
          <Account
            status={service.status}
            actions={actions}
            unavailable={unavailable}
          />
        </div>
      )}
      {service.status?.device_id && !identityNeedsAttention && (
        <Session
          status={service.status}
          serviceError={serviceError}
          page={page}
          setPage={setPage}
          actions={actions}
          reload={reload}
          refreshAll={refreshAll}
          onRecover={recover}
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
  onRecover,
}: {
  status: Status;
  serviceError?: Failure;
  page: Page;
  setPage: (page: Page) => void;
  actions: Actions;
  reload: number;
  refreshAll: () => void;
  onRecover: (action: Recovery) => void;
}) {
  const catalog = useCatalog(
    status.device_id,
    !!serviceError,
    reload,
    status.service_instance_id,
  );
  const view = useRoom(status, serviceError, reload);
  const blocked = roomActionBlock(status, !!serviceError, !!actions.pending);
  const usable = !blocked && !actions.busy && !actions.retrySeconds;
  const connection = connectionView(status, serviceError);
  const joined = () => {
    view.setSelected("");
    setPage("rooms");
  };
  return (
    <>
      {page === "rooms" && (
        <RoomPage
          {...{
            view,
            status,
            actions,
            usable,
            catalog,
            refreshAll,
            connection,
            onRecover,
          }}
        />
      )}
      <RoomDialogs
        actions={actions}
        status={status}
        gamesError={catalog.gamesError}
        games={catalog.games}
        serviceError={serviceError}
        onRecover={onRecover}
        onJoined={joined}
      />
    </>
  );
}
