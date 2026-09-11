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
          房间成员{" "}
          <span className="party-count">
            {members.length}
            <span> / {room.capacity}</span>
          </span>
        </h2>
        <span className="eyebrow">YOUR PARTY</span>
      </div>
      <div className="members">
        {members.map((m, index) => {
          const self = m.device_id === status.device_id;
          const host = m.device_id === room.owner_id;
          const peer = isCurrent
            ? status.peers.find((p) => p.device_id === m.device_id)
            : undefined;
          const measured = roomFresh && status.engine === "running" && peer;
          const linked =
            measured && (peer.mode === "direct" || peer.mode === "relay");
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
                  {host ? "房主" : "同行伙伴"}
                </span>
                <span className="member-number" aria-hidden="true">
                  {String(index + 1).padStart(2, "0")}
                </span>
                {!self && owner && (
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
                    <summary aria-label={`管理 ${m.name}`} title="成员操作">
                      <DotsThree size={24} aria-hidden="true" />
                    </summary>
                    <div className="member-menu-actions">
                      <span>{m.name}</span>
                      <button
                        disabled={!usable || !canManage}
                        onClick={() =>
                          confirm(
                            "转让房主",
                            `将房间管理权交给 ${m.name}。游戏进程与存档仍留在原主机。`,
                            {
                              action: "transfer",
                              room: room.id,
                              body: { device_id: m.device_id },
                            },
                          )
                        }
                      >
                        <CrownSimple
                          size={18}
                          weight="light"
                          aria-hidden="true"
                        />
                        转让房主
                      </button>
                      <button
                        className="danger"
                        disabled={!usable || !canManage}
                        onClick={() =>
                          confirm(
                            "踢出成员",
                            `${m.name} 将断开连接，并不能再使用邀请码进入本房。`,
                            {
                              action: "kick",
                              room: room.id,
                              body: { device_id: m.device_id },
                            },
                          )
                        }
                      >
                        <UserMinus
                          size={18}
                          weight="light"
                          aria-hidden="true"
                        />
                        踢出成员
                      </button>
                    </div>
                  </details>
                )}
              </div>
              <div className="member-profile">
                <PlayerAvatar name={m.name} identity={m.device_id} />
                <div className="member-info">
                  <h3>{m.name}</h3>
                  <span>{self ? "你的设备" : "房间成员"}</span>
                </div>
                {self && <span className="member-self">你</span>}
              </div>
              <button
                className="member-address"
                onClick={() => void copy(m.ip)}
                aria-label={`复制 ${m.name} 的虚拟 IP`}
              >
                <span>
                  <small>虚拟 IP</small>
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
                    ? "本机"
                    : measured
                      ? { direct: "直连", relay: "中继" }[peer.mode] ||
                        "尚未建链"
                      : "链路未知"}
                </span>
                {!self && isCurrent && (
                  <button
                    className="text-button member-ping"
                    disabled={!usable || !roomFresh}
                    onClick={() =>
                      void perform<{ rtt_ms: number }>(
                        "测量延迟",
                        { action: "ping", target: m.device_id },
                        (p) => setPing(`${m.name}：${p.rtt_ms.toFixed(1)} ms`),
                      )
                    }
                  >
                    测延迟
                  </button>
                )}
                {!self && (
                  <small>
                    {measured
                      ? `${peer.rtt_ms == null ? "延迟未测量" : peer.rtt_ms.toFixed(1) + " ms"} · ${peer.loss_percent == null ? "丢包未测量" : peer.loss_percent.toFixed(0) + "% 丢包"}`
                      : "暂无实测数据"}
                  </small>
                )}
              </div>
            </article>
          );
        })}
        {!members.length && (
          <div className="party-empty console-surface">
            <Users size={32} weight="light" aria-hidden="true" />
            <p>当前没有成员</p>
            <span>成员加入后会显示在这里。</span>
          </div>
        )}
      </div>
      <p className="party-note">连接类型与延迟以实际链路测量为准。</p>
      {ping && (
        <p role="status" className="ping-result">
          最近探测 · {ping}
        </p>
      )}
    </section>
  );
}
