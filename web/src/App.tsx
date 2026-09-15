import { CatalogEntryState } from "./CatalogControls";
import { CatalogRuntimeProvider, useCatalogRuntime } from "./catalogRuntime";
import { ProductApp } from "./ProductApp";
import { SessionEntryState, SessionRuntimeProvider, useSessionRuntime } from "./sessionRuntime";
import { StatusView } from "./StatusView";
import "./styles.css";

export function App() {
  if (window.location.pathname === "/status") return <StatusView />;
  return <SessionRuntimeProvider><SessionApplication /></SessionRuntimeProvider>;
}

function SessionApplication() {
  const runtime = useSessionRuntime();
  if (runtime.phase !== "authenticated" || !runtime.session || !runtime.activeWorkspace) return <SessionEntryState />;
  return <CatalogRuntimeProvider><CatalogApplication session={runtime.capabilitySession} /></CatalogRuntimeProvider>;
}

function CatalogApplication({ session }: { session?: import("./types").CapabilitySession }) {
  const runtime = useCatalogRuntime();
  const entry = <CatalogEntryState />;
  if (!runtime.workspaceId || (runtime.loading && runtime.workspaces.length === 0)) return entry;
  return <ProductApp key={runtime.workspaceId} session={session} />;
}
