import { CatalogEntryState } from "./CatalogControls";
import { CatalogRuntimeProvider, useCatalogRuntime } from "./catalogRuntime";
import { ProductApp } from "./ProductApp";
import { StatusView } from "./StatusView";
import { assets as previewAssets } from "./data";
import "./styles.css";

export function App() {
  if (window.location.pathname === "/status") return <StatusView />;
  return (
    <CatalogRuntimeProvider fixtureAssets={import.meta.env.VITE_CATALOG_FIXTURE === "1" ? previewAssets : undefined}>
      <CatalogApplication />
    </CatalogRuntimeProvider>
  );
}

function CatalogApplication() {
  const runtime = useCatalogRuntime();
  const entry = <CatalogEntryState />;
  if (!runtime.workspaceId || (!runtime.loading && runtime.assets.length === 0 && !runtime.query) || (runtime.loading && runtime.workspaces.length === 0) || (runtime.error && !runtime.workspaceId)) return entry;
  return <ProductApp key={runtime.workspaceId} />;
}
