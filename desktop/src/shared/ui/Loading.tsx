export function Spinner() {
  return <span className="spinner" aria-hidden="true" />;
}

export function Loading({ label, help }: { label: string; help?: string }) {
  return (
    <div className="operation-progress" role="status" aria-atomic="true">
      <Spinner />
      <div>
        <strong>{label}</strong>
        {help && <p>{help}</p>}
      </div>
    </div>
  );
}
