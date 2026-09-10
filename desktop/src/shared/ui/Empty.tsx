import type { ReactNode } from "react";
import { GameController } from "@phosphor-icons/react";
export function Empty({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-mark" aria-hidden="true">
        <GameController size={42} weight="light" />
      </div>
      <h2>{title}</h2>
      <div className="muted">{children}</div>
    </div>
  );
}
