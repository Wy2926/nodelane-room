import { useState } from "react";
import { setLanguage, translate, type Language } from ".";
import { WindowControls } from "../app/Shell";
import logo from "../../icon.svg";

export function LanguageSelection() {
  const [choice, setChoice] = useState<Language>("zh-CN");
  return (
    <div className="shell language-selection">
      <header className="console-bar" data-tauri-drag-region>
        <div className="brand" data-tauri-drag-region>
          <img src={logo} alt="" />
          <span>
            NodeLane<small>ROOM</small>
          </span>
        </div>
        <div className="titlebar-space" data-tauri-drag-region />
        <WindowControls language={choice} />
      </header>
      <main className="onboarding" aria-labelledby="language-title">
        <span className="eyebrow">NodeLane Room</span>
        <h1 id="language-title">
          <span lang="zh-CN">{translate("zh-CN", "language.choose")}</span>
          <br />
          <span lang="en-US">{translate("en-US", "language.choose")}</span>
        </h1>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setLanguage(choice);
          }}
        >
          <fieldset className="language-options">
            <legend className="sr-only">
              {translate(choice, "language.choose")}
            </legend>
            {(["zh-CN", "en-US"] as const).map((language) => (
              <label key={language} lang={language}>
                <input
                  type="radio"
                  name="language"
                  value={language}
                  checked={choice === language}
                  onChange={() => setChoice(language)}
                />
                <span>{translate(language, "language.name")}</span>
              </label>
            ))}
          </fieldset>
          <p className="hint" lang={choice}>
            {translate(choice, "language.changeLater")}
          </p>
          <button className="primary" lang={choice}>
            {translate(choice, "language.continue")}
          </button>
        </form>
      </main>
    </div>
  );
}
