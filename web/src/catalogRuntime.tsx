/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import {
  createAsset as createCatalogAsset,
  createWorkspace as createCatalogWorkspace,
  getAsset,
  listAssetAuthorityRecords,
  listAssetRevisions,
  listAssets,
  listWorkspaces,
  type CatalogAsset,
  type CatalogAuthoritySectionKind,
  type CatalogAssetDetail,
  type CatalogRevision,
  type SemanticAssetType,
  type Workspace,
} from "./catalog";
import type { Asset, AssetAuthoritySection, AssetReadinessGate, AssetType, AssetTypeSpec, EvidenceAuthority } from "./types";
import { useOptionalSessionRuntime } from "./sessionRuntime";

interface CreateAssetInput {
  address: string;
  assetType: SemanticAssetType;
  title: string;
  summary: string;
}

interface CatalogPageState {
  total?: number;
  nextCursor?: string;
  loadingMore: boolean;
  appendError: string;
}

interface CatalogRevisionState extends CatalogPageState {
  state: "idle" | "loading" | "ready" | "error";
  error: string;
  items: CatalogRevision[];
}

interface CatalogAuthorityPageState {
  loadingMore: boolean;
  appendError: string;
}

interface CatalogRequestFingerprint {
  workspaceId: string;
  query: string;
  assetType: SemanticAssetType | "";
  generation: number;
}

function sameCatalogRequest(left: CatalogRequestFingerprint, right: CatalogRequestFingerprint) {
  return left.workspaceId === right.workspaceId
    && left.query === right.query
    && left.assetType === right.assetType
    && left.generation === right.generation;
}

interface CatalogRuntimeValue {
  workspaces: Workspace[];
  workspaceId: string;
  workspace?: Workspace;
  assets: Asset[];
  catalogAssetIds: string[];
  loading: boolean;
  error: string;
  query: string;
  assetType: SemanticAssetType | "";
  total?: number;
  nextCursor?: string;
  loadingMore: boolean;
  appendError: string;
  detailStates: Record<string, { state: "idle" | "loading" | "ready" | "error"; error: string }>;
  revisionStates: Record<string, CatalogRevisionState>;
  authorityPageStates: Record<string, CatalogAuthorityPageState>;
  setWorkspaceId: (workspaceId: string) => void;
  setQuery: (query: string, assetType: SemanticAssetType | "") => void;
  ensureAsset: (assetId: string) => Promise<void>;
  ensureRevisions: (assetId: string) => Promise<void>;
  loadMoreAssets: () => Promise<void>;
  loadMoreRevisions: (assetId: string) => Promise<void>;
  loadMoreAuthorityRecords: (assetId: string, sectionKind: CatalogAuthoritySectionKind) => Promise<void>;
  createWorkspace: (slug: string, displayName: string) => Promise<void>;
  createAsset: (input: CreateAssetInput) => Promise<Asset>;
  refresh: () => void;
}

export const CatalogRuntimeContext = createContext<CatalogRuntimeValue | null>(null);

export function CatalogRuntimeProvider({ children }: { children: ReactNode }) {
  const sessionRuntime = useOptionalSessionRuntime();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceId, setWorkspaceIdState] = useState("");
  const [assets, setAssets] = useState<Asset[]>([]);
  const [catalogAssetIds, setCatalogAssetIds] = useState<string[]>([]);
  const [query, setQueryState] = useState("");
  const [assetType, setAssetType] = useState<SemanticAssetType | "">("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [pageState, setPageState] = useState<CatalogPageState>({ loadingMore: false, appendError: "" });
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [detailStates, setDetailStates] = useState<Record<string, { state: "idle" | "loading" | "ready" | "error"; error: string }>>({});
  const [revisionStates, setRevisionStates] = useState<Record<string, CatalogRevisionState>>({});
  const [authorityPageStates, setAuthorityPageStates] = useState<Record<string, CatalogAuthorityPageState>>({});
  const workspaceRef = useRef(workspaceId);
  const catalogRequestRef = useRef<CatalogRequestFingerprint>({ workspaceId, query, assetType, generation: 0 });
  const assetsRef = useRef(assets);
  const revisionStatesRef = useRef(revisionStates);

  useEffect(() => {
    assetsRef.current = assets;
  }, [assets]);

  useEffect(() => {
    revisionStatesRef.current = revisionStates;
  }, [revisionStates]);

  useEffect(() => {
    const controller = new AbortController();
    listWorkspaces(controller.signal)
      .then((items) => {
        setWorkspaces(items);
        const preferredWorkspaceId = sessionRuntime?.activeWorkspaceId ?? "";
        setWorkspaceIdState((current) => {
          const next = current || (items.some((item) => item.id === preferredWorkspaceId) ? preferredWorkspaceId : items[0]?.id) || "";
          workspaceRef.current = next;
          catalogRequestRef.current = { ...catalogRequestRef.current, workspaceId: next, generation: catalogRequestRef.current.generation + 1 };
          return next;
        });
        setError("");
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, [sessionRuntime?.activeWorkspaceId]);

  useEffect(() => {
    if (!workspaceId) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      const request = catalogRequestRef.current;
      setLoading(true);
      listAssets(workspaceId, query, assetType, undefined, controller.signal)
        .then((page) => {
          if (controller.signal.aborted || !sameCatalogRequest(catalogRequestRef.current, request)) return;
          setAssets((current) => page.items.map((item) => {
            const summary = projectSummary(workspaceId, item);
            const loaded = current.find((candidate) => candidate.id === item.id && candidate.detailLoaded);
            return loaded?.revisionRecord.revisionId === summary.revisionRecord.revisionId ? loaded : summary;
          }).concat(current.filter((candidate) => candidate.detailLoaded && !page.items.some((item) => item.id === candidate.id))));
          setCatalogAssetIds(page.items.map((item) => item.id));
          setPageState({ total: page.page.total, nextCursor: page.page.nextCursor, loadingMore: false, appendError: "" });
          setError("");
        })
        .catch((reason: Error) => {
          if (!controller.signal.aborted && sameCatalogRequest(catalogRequestRef.current, request)) setError(reason.message);
        })
        .finally(() => {
          if (!controller.signal.aborted && sameCatalogRequest(catalogRequestRef.current, request)) setLoading(false);
        });
    }, 160);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [assetType, query, refreshVersion, workspaceId]);

  const setWorkspaceId = useCallback((nextWorkspaceId: string) => {
    if (nextWorkspaceId === workspaceId) {
      catalogRequestRef.current = { ...catalogRequestRef.current, generation: catalogRequestRef.current.generation + 1 };
      setRefreshVersion((value) => value + 1);
      return;
    }
    workspaceRef.current = nextWorkspaceId;
    catalogRequestRef.current = { workspaceId: nextWorkspaceId, query, assetType, generation: catalogRequestRef.current.generation + 1 };
    setWorkspaceIdState(nextWorkspaceId);
    sessionRuntime?.selectWorkspace(nextWorkspaceId);
    setAssets([]);
    setCatalogAssetIds([]);
    setDetailStates({});
    setRevisionStates({});
    setAuthorityPageStates({});
    setPageState({ loadingMore: false, appendError: "" });
    setError("");
  }, [assetType, query, sessionRuntime, workspaceId]);

  const setQuery = useCallback((nextQuery: string, nextAssetType: SemanticAssetType | "") => {
    if (nextQuery === query && nextAssetType === assetType) return;
    catalogRequestRef.current = { workspaceId, query: nextQuery, assetType: nextAssetType, generation: catalogRequestRef.current.generation + 1 };
    setQueryState(nextQuery);
    setAssetType(nextAssetType);
    setCatalogAssetIds([]);
    setPageState({ loadingMore: false, appendError: "" });
  }, [assetType, query, workspaceId]);

  const loadMoreAssets = useCallback(async () => {
    const cursor = pageState.nextCursor;
    if (!workspaceId || !cursor || pageState.loadingMore) return;
    const request = catalogRequestRef.current;
    setPageState((current) => ({ ...current, loadingMore: true, appendError: "" }));
    try {
      const page = await listAssets(workspaceId, query, assetType, cursor);
      if (!sameCatalogRequest(catalogRequestRef.current, request)) return;
      setAssets((current) => {
        const next = [...current];
        for (const item of page.items) {
          const summary = projectSummary(workspaceId, item);
          const index = next.findIndex((candidate) => candidate.id === item.id);
          if (index < 0) next.push(summary);
          else if (!next[index].detailLoaded || next[index].revisionRecord.revisionId !== summary.revisionRecord.revisionId) next[index] = summary;
        }
        return next;
      });
      setCatalogAssetIds((current) => [...current, ...page.items.map((item) => item.id).filter((id) => !current.includes(id))]);
      setPageState({
        total: page.page.total,
        nextCursor: page.page.nextCursor === cursor ? undefined : page.page.nextCursor,
        loadingMore: false,
        appendError: page.page.nextCursor === cursor ? "服务器返回了重复游标，已停止继续加载。" : "",
      });
    } catch (reason) {
      if (!sameCatalogRequest(catalogRequestRef.current, request)) return;
      const message = reason instanceof Error ? reason.message : "无法加载更多资产。";
      setPageState((current) => ({ ...current, loadingMore: false, appendError: message }));
    }
  }, [assetType, pageState.loadingMore, pageState.nextCursor, query, workspaceId]);

  const ensureRevisions = useCallback(async (assetId: string) => {
    if (!workspaceId) return;
    const requestWorkspaceId = workspaceId;
    const existing = revisionStates[assetId];
    if (existing?.state === "loading" || existing?.state === "ready") return;
    setRevisionStates((current) => ({ ...current, [assetId]: { state: "loading", error: "", items: [], loadingMore: false, appendError: "" } }));
    try {
      const page = await listAssetRevisions(workspaceId, assetId);
      if (workspaceRef.current !== requestWorkspaceId) return;
      setRevisionStates((current) => ({ ...current, [assetId]: {
        state: "ready",
        error: "",
        items: page.items,
        total: page.page.total,
        nextCursor: page.page.nextCursor,
        loadingMore: false,
        appendError: "",
      } }));
    } catch (reason) {
      if (workspaceRef.current !== requestWorkspaceId) return;
      const message = reason instanceof Error ? reason.message : "无法读取修订历史。";
      setRevisionStates((current) => ({ ...current, [assetId]: { state: "error", error: message, items: [], loadingMore: false, appendError: "" } }));
    }
  }, [revisionStates, workspaceId]);

  const loadMoreRevisions = useCallback(async (assetId: string) => {
    const existing = revisionStates[assetId];
    const cursor = existing?.nextCursor;
    if (!workspaceId || !cursor || existing.loadingMore) return;
    const requestWorkspaceId = workspaceId;
    setRevisionStates((current) => ({ ...current, [assetId]: { ...current[assetId], loadingMore: true, appendError: "" } }));
    try {
      const page = await listAssetRevisions(workspaceId, assetId, cursor);
      if (workspaceRef.current !== requestWorkspaceId) return;
      if (revisionStatesRef.current[assetId]?.nextCursor !== cursor) return;
      setRevisionStates((current) => {
        const prior = current[assetId];
        if (!prior || prior.nextCursor !== cursor) return current;
        const items = [...prior.items];
        for (const item of page.items) {
          if (!items.some((candidate) => candidate.id === item.id)) items.push(item);
        }
        return { ...current, [assetId]: {
          ...prior,
          state: "ready",
          items,
          total: page.page.total,
          nextCursor: page.page.nextCursor === cursor ? undefined : page.page.nextCursor,
          loadingMore: false,
          appendError: page.page.nextCursor === cursor ? "服务器返回了重复游标，已停止继续加载。" : "",
        } };
      });
    } catch (reason) {
      if (workspaceRef.current !== requestWorkspaceId) return;
      const message = reason instanceof Error ? reason.message : "无法加载更多修订。";
      setRevisionStates((current) => ({ ...current, [assetId]: { ...current[assetId], loadingMore: false, appendError: message } }));
    }
  }, [revisionStates, workspaceId]);

  const loadMoreAuthorityRecords = useCallback(async (assetId: string, sectionKind: CatalogAuthoritySectionKind) => {
    const asset = assets.find((item) => item.id === assetId);
    const section = asset?.authoritySections?.find((item) => item.kind === sectionKind);
    const cursor = section?.recordsPage.nextCursor;
    const key = `${assetId}:${sectionKind}`;
    if (!workspaceId || !cursor || authorityPageStates[key]?.loadingMore) return;
    const requestWorkspaceId = workspaceId;
    setAuthorityPageStates((current) => ({ ...current, [key]: { loadingMore: true, appendError: "" } }));
    try {
      const page = await listAssetAuthorityRecords(workspaceId, assetId, sectionKind, cursor);
      if (workspaceRef.current !== requestWorkspaceId) return;
      if (assetsRef.current.find((item) => item.id === assetId)?.authoritySections?.find((item) => item.kind === sectionKind)?.recordsPage.nextCursor !== cursor) return;
      setAssets((current) => current.map((item) => {
        if (item.id !== assetId || !item.authoritySections) return item;
        return { ...item, authoritySections: item.authoritySections.map((candidate) => {
          if (candidate.kind !== sectionKind) return candidate;
          if (candidate.recordsPage.nextCursor !== cursor) return candidate;
          const records = [...candidate.records];
          for (const record of page.items) {
            if (!records.some((existing) => existing.kind === record.kind && existing.id === record.id)) records.push(record);
          }
          return { ...candidate, records, recordsPage: { ...page.page, nextCursor: page.page.nextCursor === cursor ? undefined : page.page.nextCursor } };
        }) };
      }));
      setAuthorityPageStates((current) => ({ ...current, [key]: {
        loadingMore: false,
        appendError: page.page.nextCursor === cursor ? "服务器返回了重复游标，已停止继续加载。" : "",
      } }));
    } catch (reason) {
      if (workspaceRef.current !== requestWorkspaceId) return;
      const message = reason instanceof Error ? reason.message : "无法加载更多权威记录。";
      setAuthorityPageStates((current) => ({ ...current, [key]: { loadingMore: false, appendError: message } }));
    }
  }, [assets, authorityPageStates, workspaceId]);

  const ensureAsset = useCallback(async (assetId: string) => {
    if (!workspaceId) return;
    const requestWorkspaceId = workspaceId;
    setDetailStates((current) => ({ ...current, [assetId]: { state: "loading", error: "" } }));
    try {
      const detail = await getAsset(workspaceId, assetId);
      if (workspaceRef.current !== requestWorkspaceId) return;
      const projected = projectDetail(workspaceId, detail);
      setAssets((current) => current.some((item) => item.id === assetId)
        ? current.map((item) => item.id === assetId ? projected : item)
        : [projected, ...current]);
      setRevisionStates((current) => Object.fromEntries(Object.entries(current).filter(([key]) => key !== assetId)));
      setAuthorityPageStates((current) => Object.fromEntries(Object.entries(current).filter(([key]) => !key.startsWith(`${assetId}:`))));
      setDetailStates((current) => ({ ...current, [assetId]: { state: "ready", error: "" } }));
      setError("");
    } catch (reason) {
      if (workspaceRef.current !== requestWorkspaceId) return;
      const message = reason instanceof Error ? reason.message : "无法加载资产详情。";
      setDetailStates((current) => ({ ...current, [assetId]: { state: "error", error: message } }));
    }
  }, [workspaceId]);

  const createWorkspace = useCallback(async (slug: string, displayName: string) => {
    const workspace = await createCatalogWorkspace(slug, displayName);
    workspaceRef.current = workspace.id;
    catalogRequestRef.current = { workspaceId: workspace.id, query: "", assetType: "", generation: catalogRequestRef.current.generation + 1 };
    setWorkspaces((current) => [...current, workspace]);
    setWorkspaceIdState(workspace.id);
    setQueryState("");
    setAssetType("");
    setAssets([]);
    setCatalogAssetIds([]);
    setDetailStates({});
    setRevisionStates({});
    setAuthorityPageStates({});
    setPageState({ loadingMore: false, appendError: "" });
    setError("");
    if (sessionRuntime) {
      await sessionRuntime.refresh();
      sessionRuntime.selectWorkspace(workspace.id);
    }
  }, [sessionRuntime]);

  const createAsset = useCallback(async (input: CreateAssetInput) => {
    if (!workspaceId) throw new Error("请先创建工作区。");
    const requestWorkspaceId = workspaceId;
    const detail = await createCatalogAsset(workspaceId, input);
    const projected = projectDetail(workspaceId, detail);
    if (workspaceRef.current !== requestWorkspaceId) return projected;
    setAssets((current) => [projected, ...current.filter((item) => item.id !== projected.id)]);
    catalogRequestRef.current = { ...catalogRequestRef.current, generation: catalogRequestRef.current.generation + 1 };
    setRefreshVersion((value) => value + 1);
    return projected;
  }, [workspaceId]);

  const refresh = useCallback(() => {
    catalogRequestRef.current = { ...catalogRequestRef.current, generation: catalogRequestRef.current.generation + 1 };
    setRefreshVersion((value) => value + 1);
  }, []);

  const value = useMemo<CatalogRuntimeValue>(() => ({
    workspaces,
    workspaceId,
    workspace: workspaces.find((item) => item.id === workspaceId),
    assets,
    catalogAssetIds,
    loading,
    error,
    query,
    assetType,
    total: pageState.total,
    nextCursor: pageState.nextCursor,
    loadingMore: pageState.loadingMore,
    appendError: pageState.appendError,
    detailStates,
    revisionStates,
    authorityPageStates,
    setWorkspaceId,
    setQuery,
    ensureAsset,
    ensureRevisions,
    loadMoreAssets,
    loadMoreRevisions,
    loadMoreAuthorityRecords,
    createWorkspace,
    createAsset,
    refresh,
  }), [assetType, assets, authorityPageStates, catalogAssetIds, createAsset, createWorkspace, detailStates, ensureAsset, ensureRevisions, error, loadMoreAssets, loadMoreAuthorityRecords, loadMoreRevisions, loading, pageState, query, refresh, revisionStates, setQuery, setWorkspaceId, workspaceId, workspaces]);

  return <CatalogRuntimeContext.Provider value={value}>{children}</CatalogRuntimeContext.Provider>;
}

export function useCatalogRuntime() {
  const value = useContext(CatalogRuntimeContext);
  if (!value) throw new Error("useCatalogRuntime must be used inside CatalogRuntimeProvider");
  return value;
}

const typeLabels: Record<SemanticAssetType, AssetType> = {
  concept: "业务概念",
  entity: "业务实体",
  semantic_model: "语义模型",
  dimension: "维度",
  measure: "度量",
  metric: "指标",
  segment: "分群",
};

const evidenceAuthorities: Record<string, EvidenceAuthority> = {
  declared: "DECLARED",
  constrained: "CONSTRAINED",
  derived: "DERIVED",
  observed: "OBSERVED",
  inferred: "INFERRED",
};

function projectSummary(workspaceId: string, item: CatalogAsset): Asset {
  return projectAsset(workspaceId, item, undefined);
}

function projectDetail(workspaceId: string, item: CatalogAssetDetail): Asset {
  return projectAsset(workspaceId, item, item);
}

function authorityCount(section: AssetAuthoritySection | undefined, key: string): number | null {
  const value = section?.values[key];
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

function projectAsset(workspaceId: string, summary: CatalogAsset, detail: CatalogAssetDetail | undefined): Asset {
  const revision = detail?.currentRevision;
  const content = revision?.content ?? {};
  const [namespace, key] = splitAddress(summary.address);
  const name = stringValue(content.displayName) || stringValue(content.name) || stringValue(content.title) || summary.title || key;
  const definition = stringValue(content.definition) || stringValue(content.summary) || summary.summary || "尚未声明规范定义。";
  const aliases = stringArray(content.aliases);
  const includes = stringArray(content.includes);
  const excludes = stringArray(content.excludes);
  const examples = stringArray(content.examples);
  const type = typeLabels[summary.assetType];
  const typeSpec = projectTypeSpec(type, content);
  const evidence = revision?.evidence ?? [];
  const revisionId = revision?.id ?? summary.currentRevisionId ?? "尚无修订";
  const recordedAt = revision?.createdAt ?? summary.updatedAt;
  const relationSection = detail?.authoritySections.find((section) => section.kind === "relations");
  const relationRecords = (relationSection?.records ?? []).flatMap((record) => {
    if (!record.relation) return [];
    const counterpartId = record.relation.direction === "outgoing" ? record.relation.objectAssetId : record.relation.subjectAssetId;
    return [{
      id: record.id,
      type: record.relation.predicate,
      plane: record.relation.plane,
      assertionState: record.relation.assertionState,
      targetId: counterpartId,
      targetName: counterpartId,
      direction: record.relation.direction,
      evidence: "服务端未提供证据引用",
      release: record.releaseId ?? relationSection?.releaseId ?? "服务端未提供 release 基准",
    }];
  });
  const owner = stringValue(content.ownerPrincipalId) || stringValue(content.owner) || "未分配";
  const maintainer = stringValue(content.maintainer) || "未分配";
  const domain = stringValue(content.domain) || namespace || "未分域";

  const authoritySections = detail?.authoritySections.map((section): AssetAuthoritySection => ({ ...section, values: { ...section.values }, records: section.records.map((record) => ({ ...record })) }));
  const releaseKnown = Boolean(detail);
  const releasedState = authoritySections?.find((section) => section.kind === "released_state");
  const validationAuthority = authoritySections?.find((section) => section.kind === "validation");
  const hasReleasedBasis = releasedState?.availability === "available" && Boolean(releasedState.revisionId);
  const currentRevisionReleased = hasReleasedBasis && releasedState?.revisionId === revisionId;
  const validationRunCount = authorityCount(validationAuthority, "runCount");
  const validationBlockingCount = authorityCount(validationAuthority, "blockerCount");
  const validationWarningCount = authorityCount(validationAuthority, "warningCount");
  const validationComplete = validationAuthority?.availability === "available"
    && validationRunCount !== null
    && validationRunCount > 0
    && validationBlockingCount !== null
    && validationWarningCount !== null;
  const validationState = (validationBlockingCount ?? 0) > 0
    ? "blocked" as const
    : validationComplete && validationWarningCount === 0
      ? "healthy" as const
      : "warning" as const;
  const validationBasis = validationAuthority?.availability === "available"
    ? validationComplete ? `${validationAuthority.authority} · ${validationRunCount} 次运行` : `${validationAuthority.authority} · 结果不完整`
    : `${validationAuthority?.authority ?? "validation_runs"} · ${validationAuthority?.availability ?? "未返回"}`;
  const readiness = projectReadiness(summary, definition, evidence.length, relationRecords.length, Boolean(detail), detail?.authoritySections);

  return {
    id: summary.id,
    detailLoaded: Boolean(detail),
    authoritySections,
    revision: revision?.sequence ? `@${revision.sequence}` : "@0",
    namespace,
    key,
    name,
    aliases,
    type,
    status: !releaseKnown ? "待确认" : currentRevisionReleased ? "已发布" : "草稿",
    owner,
    maintainer,
    domain,
    definition,
    formula: typeSpec.expression,
    grain: typeSpec.grain,
    defaultTime: typeSpec.defaultTime,
    unit: typeSpec.unit,
    aggregation: typeSpec.aggregation,
    source: evidence[0]?.locator ?? "Semlia Catalog API",
    updatedAt: formatTimestamp(summary.updatedAt),
    release: !releaseKnown ? "发布状态待加载" : hasReleasedBasis ? releasedState.releaseId ?? `release #${releasedState.releaseSequence ?? "?"}` : "尚未发布",
    tags: stringArray(content.tags),
    includes,
    excludes,
    examples,
    readiness,
    relations: relationRecords,
    bindings: [],
    joinContracts: [],
    validations: [],
    evidence: evidence.map((item) => ({
      id: item.id,
      kind: item.evidenceType,
      label: item.note || item.locator,
      source: item.locator,
      verifiedAt: formatTimestamp(item.createdAt),
      authority: evidenceAuthorities[item.evidenceType] ?? "OBSERVED",
      supports: item.fieldPath ?? "revision",
      state: item.role === "conflicts" ? "需确认" : "已验证",
    })),
    upstream: relationRecords.filter((item) => item.direction === "outgoing").map((item) => item.targetName),
    downstream: relationRecords.filter((item) => item.direction === "incoming").map((item) => item.targetName),
    consumers: [],
    identity: { workspaceId, assetId: summary.id, namespace, key, type, domain, lifecycleState: summary.lifecycleState },
    revisionRecord: {
      revisionId,
      assetId: summary.id,
      sequence: revision?.sequence ?? 0,
      schemaVersion: revision?.schemaVersion ?? "未声明",
      contentHash: revision?.contentDigest ?? "尚无内容摘要",
      recordedAt: formatTimestamp(recordedAt),
      validFrom: formatTimestamp(recordedAt),
      workflowState: !releaseKnown ? "unknown" : currentRevisionReleased ? "released" : "draft",
      name,
      aliases,
      definition,
      includes,
      excludes,
      examples,
      typeSpec,
    },
    deployment: {
      environment: !releaseKnown ? "unknown" : hasReleasedBasis ? "production" : "unknown",
      state: !releaseKnown ? "unknown" : hasReleasedBasis ? "production" : "unreleased",
      releaseId: hasReleasedBasis ? releasedState.releaseId : undefined,
      revisionId: hasReleasedBasis ? releasedState.revisionId : undefined,
    },
    qualitySnapshot: {
      snapshotId: validationAuthority?.records[0]?.id ?? `${validationAuthority?.authority ?? "validation_runs"}:${releasedState?.revisionId ?? revisionId}`,
      state: validationState,
      policyVersion: validationBasis,
      evaluatedAt: "服务端未提供评估时间",
      blockingCount: validationBlockingCount ?? 0,
      warningCount: validationWarningCount ?? 0,
    },
    claims: [],
    evidenceArtifacts: evidence.map((item) => ({ evidenceId: item.id, sourceRevision: item.sourceRevisionId ?? "未关联来源修订", authority: evidenceAuthorities[item.evidenceType] ?? "OBSERVED", observedAt: formatTimestamp(item.createdAt), scope: item.fieldPath ?? item.locator, digest: item.contentDigest })),
    evidenceLinks: [],
    validationRuns: [],
    consumerBindings: [],
    wikiContext: { contextId: "unavailable", assetRevisionId: revisionId, canonicalSummary: "服务端未返回 LLM Wiki 编译上下文。", retrievalTerms: [], disambiguationRules: [], groundedQuestions: [], compiledAt: "未提供" },
    ontologyContext: {
      ontologyId: "unavailable",
      revisionId: "未提供",
      domainPath: [],
      assetId: summary.id,
      parentConcepts: [],
      relatedConcepts: [],
      relationConstraints: [],
      consistencyState: "warning",
      consistencyIssues: ["服务端未返回本体权威上下文。"],
      revisionDelta: { addedRelations: 0, removedRelations: 0, changedConstraints: 0 },
    },
  };
}

function projectReadiness(summary: CatalogAsset, definition: string, evidenceCount: number, relationCount: number, detailLoaded: boolean, authoritySections?: CatalogAssetDetail["authoritySections"]): AssetReadinessGate[] {
  if (authoritySections) {
    const section = (kind: CatalogAssetDetail["authoritySections"][number]["kind"]) => authoritySections.find((item) => item.kind === kind);
    const gate = (id: AssetReadinessGate["id"], label: string, kind: CatalogAssetDetail["authoritySections"][number]["kind"]): AssetReadinessGate => {
      const value = section(kind);
      return { id, label, state: value?.availability === "available" ? "passed" : value?.availability === "not_configured" || value?.availability === "not_released" ? "not_applicable" : "warning", detail: value ? `${value.authority} · ${value.availability}` : "服务端未返回该权威分区。" };
    };
    const validation = section("validation");
    const runCount = authorityCount(validation, "runCount");
    const blockerCount = authorityCount(validation, "blockerCount");
    const warningCount = authorityCount(validation, "warningCount");
    const validationGate: AssetReadinessGate = validation?.availability === "not_configured" || validation?.availability === "not_released"
      ? { id: "validation", label: "验证结果", state: "not_applicable", detail: `${validation.authority} · ${validation.availability}` }
      : validation?.availability !== "available"
        ? { id: "validation", label: "验证结果", state: "warning", detail: validation ? `${validation.authority} · ${validation.availability}` : "服务端未返回验证权威分区。" }
        : runCount === null || runCount === 0 || blockerCount === null || warningCount === null
          ? { id: "validation", label: "验证结果", state: "warning", detail: `${validation.authority} · 验证分区可用，但未返回完整运行与结果计数。` }
          : blockerCount > 0 || warningCount > 0
            ? { id: "validation", label: "验证结果", state: "warning", detail: `${validation.authority} · ${runCount} 次运行，${blockerCount} 项阻断，${warningCount} 项提醒。` }
            : { id: "validation", label: "验证结果", state: "passed", detail: `${validation.authority} · ${runCount} 次运行，未返回阻断或提醒。` };
    return [
      { id: "identity", label: "稳定身份", state: "passed", detail: `${summary.id} · ${summary.address}` },
      { id: "ownership", label: "责任归属", state: "not_applicable", detail: "当前权威详情合同未提供责任人投影。" },
      gate("definition", "规范定义", "definition"),
      gate("relations", "本体关系", "relations"),
      gate("mapping", "物理实现", "physical_bindings"),
      gate("evidence", "证据覆盖", "evidence"),
      validationGate,
      gate("compatibility", "消费兼容", "consumer_impact"),
    ];
  }
  return [
    { id: "identity", label: "稳定身份", state: "passed", detail: `${summary.id} · ${summary.address}` },
    { id: "ownership", label: "责任归属", state: "warning", detail: "M1 Catalog 尚未提供责任人字段。" },
    { id: "definition", label: "规范定义", state: definition === "尚未声明规范定义。" ? "warning" : "passed", detail: definition === "尚未声明规范定义。" ? definition : "当前不可变修订包含规范定义。" },
    { id: "relations", label: "本体关系", state: !detailLoaded ? "not_applicable" : relationCount > 0 ? "passed" : "warning", detail: !detailLoaded ? "打开资产后按需读取有界关系。" : relationCount > 0 ? `${relationCount} 条真实注册表关系。` : "尚未声明关系。" },
    { id: "mapping", label: "物理实现", state: "not_applicable", detail: "M1 Catalog API 不包含物理绑定。" },
    { id: "evidence", label: "证据覆盖", state: evidenceCount > 0 ? "passed" : "warning", detail: evidenceCount > 0 ? `${evidenceCount} 项不可变证据。` : "当前修订尚未关联证据。" },
    { id: "validation", label: "验证结果", state: "not_applicable", detail: "验证运行属于后续里程碑。" },
    { id: "compatibility", label: "消费兼容", state: "not_applicable", detail: "消费绑定属于后续里程碑。" },
  ];
}

function projectTypeSpec(type: AssetType, content: Record<string, unknown>): AssetTypeSpec {
  const base = {
    expression: contentString(content, "expression") || "尚未声明",
    grain: contentString(content, "grain") || "尚未声明",
    defaultTime: contentString(content, "defaultTime", "default_time") || "尚未声明",
    unit: contentString(content, "unit") || "尚未声明",
    aggregation: contentString(content, "aggregation") || "尚未声明",
  };
  if (type === "指标") return { ...base, kind: "metric", metricKind: enumValue(contentValue(content, "metricKind", "metric_kind"), ["simple", "ratio", "derived", "cumulative", "conversion"] as const, "undeclared"), allowedDimensions: contentStrings(content, "allowedDimensions", "allowed_dimensions"), comparisonSemantics: contentString(content, "comparisonSemantics", "comparison_semantics") || "尚未声明" };
  if (type === "度量") return { ...base, kind: "measure", additivity: enumValue(contentValue(content, "additivity"), ["additive", "semi_additive", "non_additive"] as const, "undeclared"), nullHandling: contentString(content, "nullHandling", "null_handling") || "尚未声明" };
  if (type === "维度") return { ...base, kind: "dimension", valueType: contentString(content, "valueType", "value_type") || "尚未声明", nullSemantics: contentString(content, "nullSemantics", "null_semantics") || "尚未声明", hierarchy: contentStrings(content, "hierarchy") };
  if (type === "业务实体") return { ...base, kind: "entity", entityKeys: contentStrings(content, "entityKeys", "entity_keys"), identityPolicy: contentString(content, "identityPolicy", "identity_policy") || "尚未声明", lifecycle: contentString(content, "lifecycle") || "尚未声明" };
  if (type === "语义模型") return { ...base, kind: "model", primaryEntity: contentString(content, "primaryEntity", "primary_entity") || "尚未声明", publicMembers: contentStrings(content, "publicMembers", "public_members"), joinPathPolicy: contentString(content, "joinPathPolicy", "join_path_policy") || "尚未声明" };
  if (type === "分群") return { ...base, kind: "segment", baseEntity: contentString(content, "baseEntity", "base_entity") || "尚未声明", refreshPolicy: contentString(content, "refreshPolicy", "refresh_policy") || "尚未声明", effectiveTime: contentString(content, "effectiveTime", "effective_time") || "尚未声明" };
  return { ...base, kind: "concept", conceptClass: contentString(content, "conceptClass", "concept_class") || "尚未声明", disambiguationRule: contentString(content, "disambiguationRule", "disambiguation_rule") || "尚未声明" };
}

function contentValue(content: Record<string, unknown>, ...keys: string[]) {
  return keys.map((key) => content[key]).find((value) => value !== undefined);
}

function contentString(content: Record<string, unknown>, ...keys: string[]) {
  return stringValue(contentValue(content, ...keys));
}

function contentStrings(content: Record<string, unknown>, ...keys: string[]) {
  return stringArray(contentValue(content, ...keys));
}

function enumValue<const T extends readonly string[]>(value: unknown, values: T, fallback: "undeclared"): T[number] | "undeclared" {
  return typeof value === "string" && values.includes(value) ? value as T[number] : fallback;
}

function splitAddress(address: string): [string, string] {
  const index = address.lastIndexOf(".");
  return index < 0 ? ["", address] : [address.slice(0, index), address.slice(index + 1)];
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value : "";
}

function stringArray(value: unknown) {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function formatTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}
