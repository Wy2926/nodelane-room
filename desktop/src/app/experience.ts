import type { Failure, Status } from "../shared/model";
import type { MessageKey } from "../i18n";

export type ConnectionKind = "loading" | "unavailable" | "stale" | "setup" | "account" | "idle" | "joining" | "preparing" | "ready" | "reconnecting" | "paused" | "expired" | "removed" | "disabled" | "failed" | "update";
export type Recovery = "refresh" | "network" | "diagnostics" | "account" | "updates";
export type ConnectionView = {
  kind: ConnectionKind;
  tone: "neutral" | "progress" | "success" | "warning" | "danger";
  title: MessageKey;
  help: MessageKey;
  recovery?: Recovery;
  reason?: string;
  ready: boolean;
};

const views: Record<ConnectionKind, Omit<ConnectionView, "kind" | "ready" | "reason">> = {
  loading: { tone: "progress", title: "experience.loading", help: "experience.loadingHelp" },
  unavailable: { tone: "danger", title: "experience.unavailable", help: "experience.unavailableHelp", recovery: "refresh" },
  stale: { tone: "warning", title: "experience.stale", help: "experience.staleHelp", recovery: "refresh" },
  setup: { tone: "neutral", title: "experience.setup", help: "experience.setupHelp" },
  account: { tone: "warning", title: "experience.account", help: "experience.accountHelp", recovery: "account" },
  idle: { tone: "neutral", title: "experience.idle", help: "experience.idleHelp" },
  joining: { tone: "progress", title: "experience.joining", help: "experience.joiningHelp" },
  preparing: { tone: "progress", title: "experience.preparing", help: "experience.preparingHelp", recovery: "diagnostics" },
  ready: { tone: "success", title: "experience.ready", help: "experience.readyHelp" },
  reconnecting: { tone: "warning", title: "experience.reconnecting", help: "experience.reconnectingHelp", recovery: "refresh" },
  paused: { tone: "neutral", title: "experience.paused", help: "experience.pausedHelp", recovery: "network" },
  expired: { tone: "warning", title: "experience.expired", help: "experience.expiredHelp", recovery: "network" },
  removed: { tone: "warning", title: "experience.removed", help: "experience.removedHelp" },
  disabled: { tone: "warning", title: "experience.disabled", help: "experience.disabledHelp" },
  failed: { tone: "danger", title: "experience.failed", help: "experience.failedHelp", recovery: "diagnostics" },
  update: { tone: "warning", title: "experience.update", help: "experience.updateHelp", recovery: "updates" },
};

export function connectionView(status?: Status, error?: Failure, stale = false, now = Date.now()): ConnectionView {
  const view = (kind: ConnectionKind, reason?: string): ConnectionView => ({ kind, ...views[kind], reason, ready: kind === "ready" });
  if (error) return view("unavailable", error.code);
  if (stale) return view("stale");
  if (!status) return view("loading");
  if (status.update?.required || status.update?.state === "installing") return view("update");
  if (!status.device_id || status.identity === "unconfigured") return view("setup");
  if (status.identity !== "active") return view("account", status.issues.find((i) => i.scope === "identity" && !i.resolved_at)?.code);
  const reason = status.membership.reason;
  if (reason && ["member_kicked", "member_taken_over", "member_revoked", "room_closed", "room_expired", "room_banned"].includes(reason)) return view("removed", reason);
  if (!status.selected_room) return view("idle");
  if (!status.room || status.room.id !== status.selected_room) return view("joining");
  if (status.room.closed || Date.parse(status.room.expires_at) <= now) return view("removed", status.room.closed ? "room_closed" : "room_expired");
  if (status.game?.enabled === false) return view("disabled", "game_disabled");
  const networkReason = status.network.reason;
  if (["local_authorization_expired", "local_lease_expired"].includes(networkReason || "")) return view("expired", networkReason);
  const deadlines = [status.membership.valid_until, status.lease_expires_at].filter((d): d is string => !!d && !d.startsWith("0001-"));
  if (deadlines.some((d) => Date.parse(d) <= now)) return view("expired");
  if (status.network.state === "suspended") return view("paused", networkReason);
  if (status.network.state === "failed") return view("failed", networkReason);
  if (status.control !== "online") return view("reconnecting");
  if (status.network.state === "ready" && status.lan?.ready && status.membership.state === "active" && deadlines.length === 2 && deadlines.every((d) => Number.isFinite(Date.parse(d)))) return view("ready");
  if (["preparing", "registering_mac", "ready"].includes(status.network.state)) return view("preparing", networkReason);
  return view("failed", networkReason);
}

export function roomActionBlock(status: Status, unavailable: boolean, pending: boolean): MessageKey | undefined {
  if (unavailable) return "experience.actionsUnavailable";
  if (pending) return "experience.actionsPending";
  if (status.update?.required || status.update?.state === "installing") return "experience.actionsUpdate";
  if (status.identity !== "active") return "experience.actionsAccount";
  if (status.control !== "online") return "experience.actionsOffline";
  return undefined;
}

export function recoveryFor(code: string): Recovery | undefined {
  if (["local_protocol_incompatible", "local_version_mismatch", "api_contract_unsupported", "client_update_required"].includes(code)) return "updates";
  if (/^(auth_|account_)/.test(code)) return "account";
  if (code === "request_state_stale" || code === "invite_changed" || code === "local_state_stale") return "refresh";
  if (/^local_(tap_|route_|udp_|engine_|lan_|clock_|permission_|identity_|storage_|policy_|ca_|tls_)/.test(code)) return "diagnostics";
  return undefined;
}
