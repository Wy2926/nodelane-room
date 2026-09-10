export function PlayerAvatar({ name, identity, size = "medium" }: {
  name: string; identity?: string; size?: "small" | "medium" | "large";
}) {
  const tone = Array.from(identity || name).reduce((sum, char) => sum + char.codePointAt(0)!, 0) % 3;
  return (
    <span className={`player-avatar player-avatar-${size}`} data-tone={tone} aria-hidden="true">
      <img src="/assets/console-ambient.png" alt="" />
      <span>{Array.from(name.trim())[0] || "N"}</span>
    </span>
  );
}
