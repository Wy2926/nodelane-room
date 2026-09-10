import {
  useEffect,
  useRef,
  useState,
  type ReactNode,
  type InputHTMLAttributes,
} from "react";

export const date = (v?: string) =>
  !v || Date.parse(v) < 1000
    ? "—"
    : new Date(v).toLocaleString("zh-CN", { hour12: false });
export const number = (v?: number, suffix = "") =>
  v == null
    ? "—"
    : v.toLocaleString("zh-CN", { maximumFractionDigits: 1 }) + suffix;
export function bytes(v?: number) {
  if (v == null) return "—";
  const i = Math.min(
    3,
    Math.max(0, Math.floor(Math.log2(Math.max(1, v)) / 10)),
  );
  return number(v / 1024 ** i) + [" B", " KiB", " MiB", " GiB"][i];
}
export const rate = (v?: number) => (v == null ? "—" : bytes(v) + "/s");
export function Card({
  title,
  children,
  className = "",
}: {
  title?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={"card " + className}>
      {title && <h2>{title}</h2>}
      {children}
    </section>
  );
}
export function Badge({
  children,
  kind = "",
}: {
  children: ReactNode;
  kind?: string;
}) {
  return <span className={"badge " + kind}>{children}</span>;
}
export function Table({
  heads,
  children,
  empty = false,
}: {
  heads: string[];
  children: ReactNode;
  empty?: boolean;
}) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            {heads.map((h) => (
              <th key={h} scope="col">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
      {empty && <p className="empty">暂无记录</p>}
    </div>
  );
}
export function Field({
  label,
  ...input
}: InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  return (
    <label>
      {label}
      <input {...input} />
    </label>
  );
}
export function Details({ values }: { values: Record<string, ReactNode> }) {
  return (
    <dl className="details">
      {Object.entries(values).map(([k, v]) => (
        <div key={k}>
          <dt>{k}</dt>
          <dd>{v ?? "—"}</dd>
        </div>
      ))}
    </dl>
  );
}
export function Action({
  children,
  run,
  className = "",
}: {
  children: ReactNode;
  run: () => void | Promise<void>;
  className?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <span className="action">
      <button
        type="button"
        disabled={busy}
        className={className}
        onClick={async () => {
          setBusy(true);
          setError("");
          try {
            await run();
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? "处理中…" : children}
      </button>
      {error && (
        <span role="alert" className="error">
          {error}
        </span>
      )}
    </span>
  );
}
export function Modal({
  title,
  children,
  close,
}: {
  title: string;
  children: ReactNode;
  close: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    ref.current?.showModal();
    return () => previous?.focus();
  }, []);
  return (
    <dialog
      ref={ref}
      onCancel={close}
      onClose={close}
      aria-labelledby="dialog-title"
    >
      <div className="dialog-heading">
        <h2 id="dialog-title">{title}</h2>
        <button aria-label="关闭" onClick={close}>
          ×
        </button>
      </div>
      {children}
    </dialog>
  );
}
