import { useEffect, useState } from "react";
import { artwork } from "./artwork-loader";
import type { Game } from "../../shared/model";
export function Art({
  game,
  kind = "cover",
}: {
  game: Game;
  kind?: "cover" | "background";
}) {
  const [url, setURL] = useState("");
  const source = kind === "cover" ? game.cover_url : game.background_url;
  useEffect(() => {
    let cancelled = false;
    setURL("");
    if (source)
      void artwork(game.id, kind)
        .then((url) => {
          if (!cancelled) setURL(url);
        })
        .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [game.id, kind, source]);
  return <img className={`art ${kind}${url ? "" : " placeholder"}`} src={url || "/assets/console-ambient.png"} alt="" onError={() => { if (url) setURL(""); }} />;
}
