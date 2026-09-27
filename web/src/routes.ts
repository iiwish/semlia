export type WorkScope = "pending" | "initiated" | "done";
export type SourceSection = "sources" | "schedules" | "runs";

const encode = encodeURIComponent;

export const productRoutes = {
  ask: "/ask",
  assets: "/assets",
  work: "/work",
  sources: "/sources",
  schedules: "/sources/schedules",
  runs: "/sources/runs",
  settings: "/settings",
  settingsSections: {
    members: "/settings/members",
    access: "/settings/access",
    models: "/settings/models",
    integrations: "/settings/integrations",
    audit: "/settings/audit",
  },
  auditReleases: "/settings/audit/releases",
  status: "/status",
} as const;

export function assetRoute(assetId?: string, pin?: { revisionId: string; releaseId: string }) {
  const path = assetId ? `${productRoutes.assets}/${encode(assetId)}` : productRoutes.assets;
  return pin && assetId ? `${path}?${new URLSearchParams({ revision: pin.revisionId, release: pin.releaseId })}` : path;
}

export function assetVersionRoute(assetId: string) {
  return `${assetRoute(assetId)}/versions`;
}

export function workRoute(scope: WorkScope = "pending", taskId?: string) {
  const query = scope === "pending" ? "" : `?scope=${scope}`;
  return taskId ? `${productRoutes.work}/${encode(taskId)}${query}` : `${productRoutes.work}${query}`;
}

export function workReviewsRoute(scope: WorkScope = "pending") {
  const query = scope === "pending" ? "" : `?scope=${scope}`;
  return `${productRoutes.work}/reviews${query}`;
}

export function workOperationRoute(operationId: string, scope: WorkScope = "pending", options: { productionRelease?: string; from?: "drafts" } = {}) {
  const query = new URLSearchParams();
  if (scope !== "pending") query.set("scope", scope);
  if (options.productionRelease) query.set("productionRelease", options.productionRelease);
  if (options.from) query.set("from", options.from);
  return `${productRoutes.work}/operations/${encode(operationId)}${query.size ? `?${query}` : ""}`;
}

export function changeRoute(proposalId: string, scope: WorkScope = "pending") {
  const query = scope === "pending" ? "" : `?from=work&scope=${scope}`;
  return `/changes/${encode(proposalId)}${query}`;
}

export function releaseRoute(releaseId?: string, options: { from?: "asset" | "work" | "audit"; returnTo?: string } = {}) {
  const query = new URLSearchParams();
  if (options.from) query.set("from", options.from);
  if (options.returnTo) query.set("returnTo", options.returnTo);
  const suffix = query.toString() ? `?${query.toString()}` : "";
  return releaseId ? `/releases/${encode(releaseId)}${suffix}` : `${productRoutes.auditReleases}${suffix}`;
}

export function sourceRoute(sourceId?: string, section: SourceSection = "sources") {
  if (section === "schedules") return sourceId ? `${productRoutes.schedules}/${encode(sourceId)}` : productRoutes.schedules;
  if (section === "runs") return sourceId ? `${productRoutes.runs}/${encode(sourceId)}` : productRoutes.runs;
  return sourceId ? `${productRoutes.sources}/${encode(sourceId)}` : productRoutes.sources;
}

export function sourceScheduleRoute(scheduleId: string) {
  return `${productRoutes.schedules}/${encode(scheduleId)}`;
}

export function sourceRunRoute(runId: string) {
  return `${productRoutes.runs}/${encode(runId)}`;
}

export function settingsRoute(section: keyof typeof productRoutes.settingsSections = "members") {
  return productRoutes.settingsSections[section];
}

export function auditRunRoute(runId: string) {
  return `/settings/audit/runs/${encode(runId)}`;
}

export function compatibilityRoute(bindingId: string, queryId: string, consumerId?: string) {
  const query = new URLSearchParams({ binding: bindingId, query: queryId });
  if (consumerId) query.set("consumer", consumerId);
  return `/compatibility?${query.toString()}`;
}
