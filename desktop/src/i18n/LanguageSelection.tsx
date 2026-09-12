import { useState } from "react";
import { setLanguage, translate, type Language } from ".";
import { WindowControls } from "../app/Shell";
import brandMark from "../assets/brand-mark.png";

export function LanguageSelection() {
  const [choice, setChoice] = useState<Language>("zh-CN");
  return (
    <div className="shell language-selection">
      <header className="app-header" data-tauri-drag-region>
        <div className="brand" data-tauri-drag-region>
          <img src={brandMark} alt="" />
          <span>
            NodeLane <b>Room</b>
          </span>
        </div>
        <div className="titlebar-space" data-tauri-drag-region />
        <WindowControls language={choice} />
      </header>
      <main id="main-content" tabIndex={0} aria-labelledby="language-title">
        <div className="onboarding">
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
            <button className="primary" lang={choice}>
              {translate(choice, "language.continue")}
            </button>
          </form>
        </div>
      </main>
      <footer className="app-status" lang={choice}>
        <span>{translate(choice, "language.changeLater")}</span>
      </footer>
    </div>
  );
}
