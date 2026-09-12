import { useEffect, useState } from "react";
import { GameController } from "@phosphor-icons/react";
import type { Game } from "../../shared/model";
import { artwork } from "./artwork-loader";

export function Art({ game }: { game: Game }) {
  const source = game.cover_url;
  const [url, setURL] = useState("");
  useEffect(() => {
    let cancelled = false;
    setURL("");
    if (source)
      void artwork(game.id)
        .then((value) => {
          if (!cancelled) setURL(value);
        })
        .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [game.id, source]);
  return url ? (
    <img className="art" src={url} alt="" onError={() => setURL("")} />
  ) : (
    <span className="art-fallback" aria-hidden="true">
      <GameController size={40} weight="light" />
    </span>
  );
}
