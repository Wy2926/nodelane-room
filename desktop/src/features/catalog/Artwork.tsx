import { useEffect, useState } from "react";
import { GameController } from "@phosphor-icons/react";
import type { Game } from "../../shared/model";
import { artwork } from "./artwork-loader";

export function Art({
  game,
  kind = "cover",
}: {
  game: Game;
  kind?: "cover" | "background";
}) {
  const source = kind === "cover" ? game.cover_url : game.background_url;
  const [url, setURL] = useState("");
  useEffect(() => {
    let cancelled = false;
    setURL("");
    if (source)
      void artwork(game.id, kind)
        .then((value) => {
          if (!cancelled) setURL(value);
        })
        .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [game.id, kind, source]);
  return url ? (
    <img className="art" src={url} alt="" onError={() => setURL("")} />
  ) : (
    <span className="art-fallback" aria-hidden="true">
      <GameController size={40} weight="light" />
    </span>
  );
}
