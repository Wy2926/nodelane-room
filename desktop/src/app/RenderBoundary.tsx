import { Component, type ReactNode } from "react";
import { t } from "../i18n";

export class RenderBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() {
    if (!this.state.failed) return this.props.children;
    return <main className="render-fallback" role="alert"><h1>{t("experience.renderError")}</h1><p>{t("experience.renderErrorHelp")}</p><button className="primary" onClick={() => window.location.reload()}>{t("experience.reloadInterface")}</button></main>;
  }
}
