import { Link, Outlet, useRouteError } from "react-router";

export function AppShell({ error = false }: { error?: boolean }) {
  const routeError = useRouteError();
  return (
    <div className="app-shell">
      <header className="topbar">
        <Link to="/" className="brand" aria-label="rct home">
          <span className="brand-mark" aria-hidden="true">rct</span>
          <span className="brand-copy">Run Control</span>
        </Link>
        <span className="local-badge"><span aria-hidden="true">●</span> Local only</span>
      </header>
      {error ? (
        <main className="page error-page">
          <p className="eyebrow">ROUTE ERROR</p>
          <h1>This view could not be displayed</h1>
          <p>{routeError instanceof Error ? routeError.message : "Return to the run list and try again."}</p>
          <Link to="/" className="button-link">View runs</Link>
        </main>
      ) : <Outlet />}
      <footer className="footer">Workflow authority remains in rct Core · Browser progress is read-only</footer>
    </div>
  );
}
