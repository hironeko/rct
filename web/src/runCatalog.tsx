import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { ApiError, getRuns } from "./api/client";
import type { RunSnapshot } from "./api/types";

interface RunCatalogValue {
  runs?: RunSnapshot[];
  error?: string;
  refresh: () => Promise<void>;
}

const RunCatalogContext = createContext<RunCatalogValue | undefined>(undefined);

export function RunCatalogProvider({ children }: { children: ReactNode }) {
  const [runs, setRuns] = useState<RunSnapshot[]>();
  const [error, setError] = useState<string>();
  const refresh = async () => {
    try {
      setRuns(await getRuns());
      setError(undefined);
    } catch (reason: unknown) {
      setError(reason instanceof ApiError ? `${reason.message} · ${reason.requestId}` : "Unable to load local runs");
    }
  };
  useEffect(() => {
    let active = true;
    const load = () => { if (active) void refresh(); };
    load();
    const interval = window.setInterval(load, 3000);
    return () => { active = false; window.clearInterval(interval); };
  }, []);
  const value = useMemo(() => ({ runs, error, refresh }), [runs, error]);
  return <RunCatalogContext.Provider value={value}>{children}</RunCatalogContext.Provider>;
}

export function useRunCatalog(): RunCatalogValue {
  const value = useContext(RunCatalogContext);
  if (!value) throw new Error("useRunCatalog must be used within RunCatalogProvider");
  return value;
}
