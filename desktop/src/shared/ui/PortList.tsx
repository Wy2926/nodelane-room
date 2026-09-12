import { t } from "../../i18n";
import type { Port } from "../model";
export function PortList({ ports }: { ports: Port[] }) {
  return (
    <div className="port-list">
      {ports.length ? (
        ports.map((p) => (
          <span
            key={`${p.protocol}/${p.port}`}
            title={p.description}
          >
            {p.protocol.toUpperCase()} {p.port}
            {p.port_end && p.port_end !== p.port ? `–${p.port_end}` : ""}
            {p.description ? <small>{p.description}</small> : null}
          </span>
        ))
      ) : (
        <span className="muted">{t("portList.noGamePortsConfigured")}</span>
      )}
    </div>
  );
}
