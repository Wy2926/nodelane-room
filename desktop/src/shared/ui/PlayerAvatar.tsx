export function PlayerAvatar({
  name,
  size = "medium",
}: {
  name: string;
  identity?: string;
  size?: "small" | "medium" | "large";
}) {
  return (
    <span className={`player-avatar avatar-${size}`} aria-hidden="true">
      {Array.from(name.trim())[0] || "N"}
    </span>
  );
}
