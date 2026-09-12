import { t } from "../../../i18n";
import { useEffect, useState } from "react";
import { rpc, failure } from "../../../native/api";
import type { Actions } from "../../../app/use-actions";
import type { Dialog } from "./types";
import type { Status, Failure, InviteInfo } from "../../../shared/model";
import { formatTime } from "../../../shared/time";
import { invitationLink } from "../invitation-link";
import { Loading } from "../../../shared/ui/Loading";
export function Invitation({
  dialog,
  actions,
}: {
  dialog: Extract<Dialog, { type: "invite" }>;
  actions: Actions;
  status?: Status;
  gamesError?: Failure;
  serviceError?: Failure;
  onJoined: () => void;
}) {
  const { copy } = actions;
  const [valid, setValid] = useState(false);
  const [checking, setChecking] = useState(true);
  const [code, setCode] = useState(dialog.invitation.code);
  useEffect(() => {
    let stopped = false;
    setChecking(true);
    let timer: ReturnType<typeof setTimeout>;
    const clear = () => {
      setValid(false);
      setCode("");
      actions.setDialog((current) =>
        current?.type === "invite" && current.invitation.code
          ? { ...current, invitation: { ...current.invitation, code: "" } }
          : current,
      );
    };
    async function check() {
      if (Date.parse(dialog.invitation.expires_at) <= Date.now()) {
        setChecking(false);
        clear();
        return;
      }
      try {
        const info = await rpc<InviteInfo>({
          action: "invite-info",
          room: dialog.room.id,
        });
        if (stopped) return;
        const current =
          info.active && info.revision === dialog.invitation.revision;
        setValid(current);
        if (!current) {
          clear();
          return;
        }
      } catch {
        if (!stopped) setValid(false);
      } finally {
        if (!stopped) setChecking(false);
      }
      if (!stopped) timer = setTimeout(check, 2000);
    }
    void check();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [
    dialog.room.id,
    dialog.invitation.revision,
    dialog.invitation.expires_at,
  ]);

  return (
    <>
      <p>
        {dialog.room.name} · {dialog.room.game_name}
      </p>
      <div className="invitation selectable">
        {checking ? <Loading label={t("experience.working")} /> : valid ? invitationLink(code) : failure({ code: "invite_changed" }).error}
      </div>
      <p className="muted">
        {t("invitation.validUntil", {
          time: formatTime(dialog.invitation.expires_at),
        })}
      </p>
      <button
        className="primary"
        onClick={() => void copy(invitationLink(code))}
        disabled={!valid || !code}
      >
        {t("invitation.copyInviteCode")}
      </button>
      <p className="hint">{t("invitation.storageHelp")}</p>
      <button
        disabled={!!actions.busy}
        onClick={() =>
          actions.confirm(
            t("interaction.revokeInvite"),
            t("interaction.revokeInvite"),
            {
              action: "invite-revoke",
              room: dialog.room.id,
              body: { expected_revision: dialog.room.revision },
            },
          )
        }
      >
        {t("interaction.revokeInvite")}
      </button>
    </>
  );
}
