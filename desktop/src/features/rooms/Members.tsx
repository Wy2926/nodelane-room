import { t } from "../../i18n";
import { Copy, DotsThree } from "@phosphor-icons/react";
import type { Status } from "../../shared/model";
import type { Actions } from "../../app/use-actions";
import type { RoomView } from "./use-room";
import { PlayerAvatar } from "../../shared/ui/PlayerAvatar";

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
  const { room, members, isCurrent, roomFresh, canManage, owner } = view;
  if (!room) return null;
  const valid =
    roomFresh &&
    status.engine === "running" &&
    Date.parse(status.lease_expires_at || "") > Date.now() &&
    Date.parse(status.membership.valid_until || "") > Date.now();
  return (
    <section className="party" aria-labelledby="party-title">
      <h3 id="party-title">
        {t("members.roomMembers")}{" "}
        <span className="muted">
          {members.length} / {room.capacity}
        </span>
      </h3>
      <div
        className="member-table-scroll"
        tabIndex={0}
        role="region"
        aria-label={t("members.roomMembers")}
      >
        <table className="member-table">
          <colgroup>
            <col className="member-col-name" />
            <col className="member-col-ip" />
            <col className="member-col-link" />
            <col className="member-col-metric" />
            <col className="member-col-metric" />
            <col className="member-col-actions" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col">{t("desk.member")}</th>
              <th scope="col">{t("members.virtualIp")}</th>
              <th scope="col">{t("desk.connectionType")}</th>
              <th scope="col" className="metric">
                {t("members.latency")}
              </th>
              <th scope="col" className="metric">
                {t("diagnostics.packetLoss")}
              </th>
              <th scope="col">
                <span className="sr-only">{t("desk.moreActions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {members.map((member) => {
              const self = member.device_id === status.device_id;
              const host = member.user_id === room.owner_user_id;
              const peer = isCurrent
                ? status.peers.find((p) => p.device_id === member.device_id)
                : undefined;
              const linked =
                valid && peer && ["direct", "relay"].includes(peer.mode);
              const age = Date.now() - Date.parse(peer?.measured_at || "");
              const measured =
                valid && !self && peer && age >= -1000 && age < 30000;
              const rtt =
                measured && Number.isFinite(peer.rtt_ms) && peer.rtt_ms! >= 0
                  ? peer.rtt_ms
                  : undefined;
              const loss =
                measured &&
                Number.isFinite(peer.loss_percent) &&
                peer.loss_percent! >= 0 &&
                peer.loss_percent! <= 100
                  ? peer.loss_percent
                  : undefined;
              return (
                <tr className="member-row" key={member.device_id}>
                  <td>
                    <div className="member-name">
                      <PlayerAvatar name={member.name} />
                      <div>
                        <h4>{member.name}</h4>
                        <small>
                          {t(host ? "members.owner" : "members.partyMember")}
                          {self && ` · ${t("members.you")}`}
                        </small>
                      </div>
                    </div>
                  </td>
                  <td>
                    <button
                      className="member-address"
                      aria-label={t("members.copyVirtualIpFor", {
                        0: member.name,
                      })}
                      onClick={() => void actions.copy(member.ip)}
                    >
                      <span className="mono">{member.ip}</span>
                      <Copy size={16} aria-hidden="true" />
                    </button>
                  </td>
                  <td>
                    <div className="member-link">
                      <span>
                        {self
                          ? t("members.thisDevice")
                          : linked
                            ? t(
                                peer.mode === "direct"
                                  ? "diagnostics.direct"
                                  : "diagnostics.relay",
                              )
                            : t("diagnostics.connectionUnknown")}
                      </span>
                    </div>
                  </td>
                  <td
                    className="metric"
                    title={
                      self
                        ? t("members.thisDevice")
                        : t("members.measurementsHelp")
                    }
                  >
                    {rtt == null ? "—" : `${rtt.toFixed(1)} ms`}
                  </td>
                  <td
                    className="metric"
                    title={
                      self
                        ? t("members.thisDevice")
                        : t("members.measurementsHelp")
                    }
                  >
                    {loss == null ? "—" : `${loss.toFixed(1)}%`}
                  </td>
                  <td>
                    <div className="member-actions">
                      {member.user_id !== status.user?.id && owner && (
                        <details
                          className="overflow-menu"
                          onKeyDown={(e) => {
                            if (e.key === "Escape") {
                              e.currentTarget.open = false;
                              e.currentTarget.querySelector("summary")?.focus();
                              e.stopPropagation();
                            }
                          }}
                        >
                          <summary
                            aria-label={t("members.manage", { 0: member.name })}
                          >
                            <DotsThree size={24} aria-hidden="true" />
                          </summary>
                          <div>
                            <button
                              disabled={!usable || !canManage}
                              onClick={() =>
                                actions.confirm(
                                  t("members.transferOwnership"),
                                  t("members.transferHelp", { 0: member.name }),
                                  {
                                    action: "transfer",
                                    room: room.id,
                                    body: {
                                      device_id: member.device_id,
                                      expected_revision: room.revision,
                                    },
                                  },
                                )
                              }
                            >
                              {t("members.transferOwnership")}
                            </button>
                            <button
                              className="danger"
                              disabled={!usable || !canManage}
                              onClick={() =>
                                actions.confirm(
                                  t("members.removeMember"),
                                  t("members.kickHelp", { 0: member.name }),
                                  {
                                    action: "kick",
                                    room: room.id,
                                    body: {
                                      device_id: member.device_id,
                                      expected_revision: room.revision,
                                    },
                                  },
                                )
                              }
                            >
                              {t("members.removeMember")}
                            </button>
                          </div>
                        </details>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
            {!members.length && (
              <tr>
                <td colSpan={6} className="empty-members">
                  {t("members.noMembersRightNow")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <p className="hint">{t("members.measurementsHelp")}</p>
    </section>
  );
}
