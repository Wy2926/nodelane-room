import { t } from "../../i18n";
import { useEffect, useState } from "react";
import {
  Copy,
  CrownSimple,
  Desktop,
  DotsThree,
  Pulse,
  Users,
  UserMinus,
} from "@phosphor-icons/react";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { RoomView } from "./use-room";
import { PortList } from "../../shared/ui/PortList";
import { PlayerAvatar } from "../../shared/ui/PlayerAvatar";
import { fresh } from "../../shared/time";

export function Members({
  view,
  status,
  actions,
  usable,
}: {
  view: RoomView;
  status: Status;
  actions: Actions;
  usable: boolean;
}) {
  const { room, game, members, isCurrent, roomFresh, canManage, owner } = view;
  const { copy, perform, confirm } = actions;
  const [ping, setPing] = useState("");
  useEffect(() => setPing(""), [room?.id]);
  if (!room) return null;
  return (
    <section className="party" aria-labelledby="party-title">
      <div className="system-heading">
        <Users size={22} weight="light" aria-hidden="true" />
        <h2 id="party-title">
          {t("members.roomMembers")}{" "}
          <span className="party-count">
            {members.length}
            <span> / {room.capacity}</span>
          </span>
        </h2>
        <span className="eyebrow">{t("members.yourParty")}</span>
      </div>
      <div className="members">
        {members.map((m, index) => {
          const self = m.device_id === status.device_id;
          const host = m.user_id === room.owner_user_id;
          const peer = isCurrent
            ? status.peers.find((p) => p.device_id === m.device_id)
            : undefined;
          const measured =
            roomFresh &&
            status.engine === "running" &&
            peer &&
            fresh(peer.measured_at);
          const linked =
            roomFresh &&
            status.engine === "running" &&
            peer &&
            (peer.mode === "direct" || peer.mode === "relay");
          return (
            <article
              className="member console-surface"
              data-self={self}
              key={`${room.id}:${m.device_id}`}
            >
              <div className="member-topline">
                <span className="member-role">
                  {host ? (
                    <CrownSimple size={15} weight="light" aria-hidden="true" />
                  ) : (
                    <Users size={15} weight="light" aria-hidden="true" />
                  )}
                  {host ? t("members.owner") : t("members.partyMember")}
                </span>
                <span className="member-number" aria-hidden="true">
                  {String(index + 1).padStart(2, "0")}
                </span>
                {m.user_id !== status.user?.id && owner && (
                  <details
                    className="member-menu"
                    onKeyDown={(e) => {
                      if (e.key === "Escape") {
                        e.currentTarget.open = false;
                        e.currentTarget.querySelector("summary")?.focus();
                        e.stopPropagation();
                      }
                    }}
                  >
                    <summary
                      aria-label={t("members.manage", { 0: m.name })}
                      title={t("members.memberActions")}
                    >
                      <DotsThree size={24} aria-hidden="true" />
                    </summary>
                    <div className="member-menu-actions">
                      <span>{m.name}</span>
                      <button
                        disabled={!usable || !canManage}
                        onClick={() =>
                          confirm(
                            t("members.transferOwnership"),
                            t("members.transferHelp", { 0: m.name }),
                            {
                              action: "transfer",
                              room: room.id,
                              body: {
                                device_id: m.device_id,
                                expected_revision: room.revision,
                              },
                            },
                          )
                        }
                      >
                        <CrownSimple
                          size={18}
                          weight="light"
                          aria-hidden="true"
                        />
                        {t("members.transferOwnership")}
                      </button>
                      <button
                        className="danger"
                        disabled={!usable || !canManage}
                        onClick={() =>
                          confirm(
                            t("members.removeMember"),
                            t("members.kickHelp", { 0: m.name }),
                            {
                              action: "kick",
                              room: room.id,
                              body: {
                                device_id: m.device_id,
                                expected_revision: room.revision,
                              },
                            },
                          )
                        }
                      >
                        <UserMinus
                          size={18}
                          weight="light"
                          aria-hidden="true"
                        />
                        {t("members.removeMember")}
                      </button>
                    </div>
                  </details>
                )}
              </div>
              <div className="member-profile">
                <PlayerAvatar name={m.name} identity={m.device_id} />
                <div className="member-info">
                  <h3>{m.name}</h3>
                  <span>
                    {self ? t("members.yourDevice") : t("members.roomMembers")}
                  </span>
                </div>
                {self && (
                  <span className="member-self">{t("members.you")}</span>
                )}
              </div>
              <button
                className="member-address"
                onClick={() => void copy(m.ip)}
                aria-label={t("members.copyVirtualIpFor", { 0: m.name })}
              >
                <span>
                  <small>{t("members.virtualIp")}</small>
                  <span className="mono">{m.ip}</span>
                </span>
                <Copy size={17} weight="light" aria-hidden="true" />
              </button>
              <div className="member-ports">
                <PortList ports={game?.ports || []} />
              </div>
              <div className="member-link">
                <span className="member-link-state" data-linked={!!linked}>
                  {self ? (
                    <Desktop size={16} weight="light" aria-hidden="true" />
                  ) : (
                    <Pulse size={16} weight="light" aria-hidden="true" />
                  )}
                  {self
                    ? t("members.thisDevice")
                    : linked
                      ? {
                          direct: t("diagnostics.direct"),
                          relay: t("diagnostics.relay"),
                        }[peer.mode] || t("diagnostics.notConnected")
                      : t("diagnostics.connectionUnknown")}
                </span>
                {!self && isCurrent && (
                  <button
                    className="text-button member-ping"
                    disabled={!usable || !roomFresh}
                    onClick={() =>
                      void perform<{ rtt_ms: number }>(
                        t("diagnostics.measureLatency"),
                        { action: "ping", target: m.device_id },
                        (p) =>
                          setPing(
                            t("members.ms", {
                              0: m.name,
                              1: p.rtt_ms.toFixed(1),
                            }),
                          ),
                      )
                    }
                  >
                    {t("members.testLatency")}
                  </button>
                )}
                {!self && (
                  <small>
                    {measured
                      ? `${peer.rtt_ms == null ? t("members.latencyNotMeasured") : peer.rtt_ms.toFixed(1) + " ms"} · ${peer.loss_percent == null ? t("members.lossNotMeasured") : peer.loss_percent.toFixed(0) + t("members.loss")}`
                      : t("members.noMeasurementsYet")}
                  </small>
                )}
              </div>
            </article>
          );
        })}
        {!members.length && (
          <div className="party-empty console-surface">
            <Users size={32} weight="light" aria-hidden="true" />
            <p>{t("members.noMembersRightNow")}</p>
            <span>{t("members.membersWillAppearHereWhenTheyJoin")}</span>
          </div>
        )}
      </div>
      <p className="party-note">{t("members.measurementsHelp")}</p>
      {ping && (
        <p role="status" className="ping-result">
          {t("members.latestProbe")}
          {ping}
        </p>
      )}
    </section>
  );
}
