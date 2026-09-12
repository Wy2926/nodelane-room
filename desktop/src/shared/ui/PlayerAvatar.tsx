export function PlayerAvatar({
  name,
  size = "medium",
}: {
  name: string;
  size?: "small" | "medium";
}) {
  return (
    <span className={`player-avatar avatar-${size}`} aria-hidden="true">
      {Array.from(name.trim())[0] || "N"}
    </span>
  );
}
