import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router/dom";
import { bootstrapSession } from "./api/client";
import { router } from "./router";
import { I18nProvider, useI18n } from "./i18n";
import "./styles/app.css";

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("rct UI root element is missing");

const root = createRoot(rootElement);

bootstrapSession()
  .then(() => root.render(<I18nProvider><RouterProvider router={router} /></I18nProvider>))
  .catch((error: unknown) => {
    const message = error instanceof Error ? error.message : "The local session could not be established";
    root.render(
      <I18nProvider><SessionError message={message} /></I18nProvider>,
    );
  });

function SessionError({ message }: { message: string }) {
  const { t } = useI18n();
  return <main className="session-error"><p className="eyebrow">{t("sessionRequired")}</p><h1>{t("unableToOpen")}</h1><p>{message}</p><p>{t("restartServe")}</p></main>;
}
