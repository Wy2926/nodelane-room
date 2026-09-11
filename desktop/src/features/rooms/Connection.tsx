import { t } from "../../i18n";
import type { Status } from "../../shared/model";
import type { RoomView } from "./use-room";
import { PortList } from "../../shared/ui/PortList";
import { Info } from "@phosphor-icons/react";

export function Connection({
  view,
  status,
}: {
  view: RoomView;
  status: Status;
}) {
  if (!view.room) return null;
  return (
    <section className="connection-help">
      <div className="connection-guidance">
        <Info size={22} aria-hidden="true" />
        <p>{t("desk.gameGuidance")}</p>
      </div>
      <details>
        <summary>{t("desk.connectionDetails")}</summary>
        <p>{t("connection.lanHelp")}</p>
        {view.isCurrent && (
          <p>
            {status.lan?.ready
              ? t("connection.lanAdapterReadyMtu", {
                  0: status.lan.interface,
                  1: status.lan.mtu,
                })
              : t("connection.lanNotReadyHelp")}
          </p>
        )}
        <h3>{t("connection.serverPortSettings")}</h3>
        <PortList ports={view.game?.ports || []} />
        <p className="hint">{t("connection.readOnlyHelp")}</p>
        <p className="hint">
          {t("connection.discovery", {
            broadcast: t(
              view.game?.network.broadcast ? "connection.on" : "connection.off",
            ),
            multicast: t(
              view.game?.network.multicast ? "connection.on" : "connection.off",
            ),
          })}
        </p>
      </details>
    </section>
  );
}
