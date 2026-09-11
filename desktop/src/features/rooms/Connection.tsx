import { t } from "../../i18n";
import type { Status } from "../../shared/model";
import type { RoomView } from "./use-room";
import { PortList } from "../../shared/ui/PortList";
import { GameController, PlugsConnected } from "@phosphor-icons/react";

export function Connection({
  view,
  status,
}: {
  view: RoomView;
  status: Status;
}) {
  const { room, game, isCurrent } = view;
  if (!room) return null;
  return (
    <section className="connection" aria-labelledby="connection-title">
      <div className="system-heading">
        <PlugsConnected size={22} weight="light" aria-hidden="true" />
        <h2 id="connection-title">{t("connection.gameConnection")}</h2>
        <span className="eyebrow">{t("connection.connect")}</span>
      </div>
      <div className="connection-card console-surface">
        <div className="connection-intro">
          <GameController size={34} weight="light" aria-hidden="true" />
          <div>
            <span className="eyebrow">{t("connection.readyForYourNextGame")}</span>
            <h3>{t("connection.inGameLanConnection")}</h3>
          </div>
        </div>
        <p className="muted">
          {t("connection.lanHelp")}</p>
        {isCurrent && (
          <p className="hint">
            {status.lan?.ready
              ? t("connection.lanAdapterReadyMtu", { 0: status.lan.interface, 1: status.lan.mtu })
              : t("connection.lanNotReadyHelp")}
          </p>
        )}
        <div className="connection-ports">
          <h3>{t("connection.serverPortSettings")}</h3>
          <PortList ports={game?.ports || []} />
          <p className="hint">{t("connection.readOnlyHelp")}</p>
        </div>
        <details className="connection-help">
          <summary>{t("connection.connectionHelp")}</summary>
          <p className="hint">
            {t("connection.discovery", {
              broadcast: t(game?.network.broadcast ? "connection.on" : "connection.off"),
              multicast: t(game?.network.multicast ? "connection.on" : "connection.off"),
            })}</p>
        </details>
      </div>
    </section>
  );
}
