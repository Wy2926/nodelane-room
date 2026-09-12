import type { ReactNode } from "react";
import { GameController } from "@phosphor-icons/react";
import { Spinner } from "./Loading";
export function Empty({
  title,
  children,
  loading = false,
}: {
  title: string;
  children?: ReactNode;
  loading?: boolean;
}) {
  return (
    <div
      className="empty"
      role={loading ? "status" : undefined}
      aria-atomic={loading || undefined}
    >
      <div className="empty-mark" aria-hidden="true">
        {loading ? <Spinner /> : <GameController size={42} weight="light" />}
      </div>
      <h2>{title}</h2>
      <div className="muted">{children}</div>
    </div>
  );
}
