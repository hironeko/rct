import { Link, Outlet, useRouteError } from "react-router";
import { RunSidebar } from "../components/RunSidebar";
import { useI18n } from "../i18n";
import { RunCatalogProvider } from "../runCatalog";

export function AppShell({ error = false }: { error?: boolean }) {
  return <RunCatalogProvider><ShellContent error={error} /></RunCatalogProvider>;
}

function ShellContent({ error }: { error: boolean }) {
  const routeError = useRouteError();
  const { t } = useI18n();
  return (
    <div className="workspace-shell">
      <RunSidebar />
      <div className="workspace-content">
        {error ? (
          <main className="workspace-page error-page">
          <p className="eyebrow">ROUTE ERROR</p><h1>{t("routeError")}</h1>
          <p>{routeError instanceof Error ? routeError.message : "Return to the run list and try again."}</p>
          <Link to="/" className="button-link">{t("backToRuns")}</Link>
        </main>
        ) : <Outlet />}
      </div>
    </div>
  );
}
