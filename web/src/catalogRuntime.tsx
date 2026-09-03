/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

import {
  createAsset as createCatalogAsset,
  createWorkspace as createCatalogWorkspace,
  getAsset,
  listAssetRelations,
  listAssets,
  listWorkspaces,
  type CatalogAsset,
  type CatalogAssetDetail,
  type CatalogRelation,
  type SemanticAssetType,
  type Workspace,
} from "./catalog";
import type { Asset, AssetReadinessGate, AssetType, AssetTypeSpec, AssetWorkflowState, EvidenceAuthority } from "./types";

interface CreateAssetInput {
  address: string;
  assetType: SemanticAssetType;
  title: string;
  summary: string;
}

interface CatalogRuntimeValue {
  workspaces: Workspace[];
  workspaceId: string;
  workspace?: Workspace;
  assets: Asset[];
  loading: boolean;
  error: string;
  query: string;
  assetType: SemanticAssetType | "";
  setWorkspaceId: (workspaceId: string) => void;
  setQuery: (query: string, assetType: SemanticAssetType | "") => void;
  ensureAsset: (assetId: string) => Promise<void>;
  createWorkspace: (slug: string, displayName: string) => Promise<void>;
  createAsset: (input: CreateAssetInput) => Promise<Asset>;
  refresh: () => void;
  setWorkflowStateOverlay: (states: Record<string, AssetWorkflowState>) => void;
}

const CatalogRuntimeContext = createContext<CatalogRuntimeValue | null>(null);

export function CatalogRuntimeProvider({ children, fixtureAssets }: { children: ReactNode; fixtureAssets?: Asset[] }) {
  const fixtureWorkspace: Workspace = { id: "wsp_01arz3ndektsv4rrffq69g5fav", slug: "product-test", displayName: "产品测试工作区", createdAt: "2026-09-02T00:00:00Z", updatedAt: "2026-09-02T00:00:00Z" };
  const [workspaces, setWorkspaces] = useState<Workspace[]>(fixtureAssets ? [fixtureWorkspace] : []);
  const [workspaceId, setWorkspaceIdState] = useState(fixtureAssets ? fixtureWorkspace.id : "");
  const [assets, setAssets] = useState<Asset[]>(fixtureAssets ?? []);
  const [query, setQueryState] = useState("");
  const [assetType, setAssetType] = useState<SemanticAssetType | "">("");
  const [loading, setLoading] = useState(!fixtureAssets);
  const [error, setError] = useState("");
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [workflowStateOverlay, setWorkflowStateOverlayState] = useState<Record<string, AssetWorkflowState>>({});

  useEffect(() => {
    if (fixtureAssets) return;
    const controller = new AbortController();
    listWorkspaces(controller.signal)
      .then((items) => {
        setWorkspaces(items);
        setWorkspaceIdState((current) => current || items[0]?.id || "");
        setError("");
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, [fixtureAssets]);

  useEffect(() => {
    if (fixtureAssets) return;
    if (!workspaceId) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setLoading(true);
      listAssets(workspaceId, query, assetType, controller.signal)
        .then((page) => {
          setAssets(page.items.map((item) => projectSummary(workspaceId, item)));
          setError("");
        })
        .catch((reason: Error) => {
          if (!controller.signal.aborted) setError(reason.message);
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    }, 160);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [assetType, fixtureAssets, query, refreshVersion, workspaceId]);

  const setWorkspaceId = useCallback((nextWorkspaceId: string) => {
    setWorkspaceIdState(nextWorkspaceId);
    setAssets([]);
    setError("");
  }, []);

  const setQuery = useCallback((nextQuery: string, nextAssetType: SemanticAssetType | "") => {
    setQueryState(nextQuery);
    setAssetType(nextAssetType);
  }, []);

  const ensureAsset = useCallback(async (assetId: string) => {
    if (!workspaceId) return;
    try {
      const [detail, relations] = await Promise.all([
        getAsset(workspaceId, assetId),
        listAssetRelations(workspaceId, assetId),
      ]);
      const projected = projectDetail(workspaceId, detail, relations.items);
      setAssets((current) => current.map((item) => item.id === assetId ? projected : item));
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "无法加载资产详情。");
    }
  }, [workspaceId]);

  const createWorkspace = useCallback(async (slug: string, displayName: string) => {
    const workspace = await createCatalogWorkspace(slug, displayName);
    setWorkspaces((current) => [...current, workspace]);
    setWorkspaceIdState(workspace.id);
    setAssets([]);
    setError("");
  }, []);

  const createAsset = useCallback(async (input: CreateAssetInput) => {
    if (!workspaceId) throw new Error("请先创建工作区。");
    const detail = await createCatalogAsset(workspaceId, input);
    const projected = projectDetail(workspaceId, detail, []);
    setAssets((current) => [projected, ...current.filter((item) => item.id !== projected.id)]);
    return projected;
  }, [workspaceId]);

  const assetsWithWorkflowStates = useMemo(() => assets.map((item) => {
    const state = workflowStateOverlay[item.id];
    if (!state || item.revisionRecord.workflowState === state) return item;
    return { ...item, revisionRecord: { ...item.revisionRecord, workflowState: state } };
  }), [assets, workflowStateOverlay]);

  const value = useMemo<CatalogRuntimeValue>(() => ({
    workspaces,
    workspaceId,
    workspace: workspaces.find((item) => item.id === workspaceId),
    assets: assetsWithWorkflowStates,
    loading,
    error,
    query,
    assetType,
    setWorkspaceId,
    setQuery,
    ensureAsset,
    createWorkspace,
    createAsset,
    refresh: () => setRefreshVersion((value) => value + 1),
    setWorkflowStateOverlay: setWorkflowStateOverlayState,
  }), [assetType, assetsWithWorkflowStates, createAsset, createWorkspace, ensureAsset, error, loading, query, setQuery, setWorkflowStateOverlayState, setWorkspaceId, workspaceId, workspaces]);

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
  return projectAsset(workspaceId, item, undefined, []);
}

function projectDetail(workspaceId: string, item: CatalogAssetDetail, relations: CatalogRelation[]): Asset {
  return projectAsset(workspaceId, item, item, relations);
}

function projectAsset(workspaceId: string, summary: CatalogAsset, detail: CatalogAssetDetail | undefined, relations: CatalogRelation[]): Asset {
  const revision = detail?.currentRevision;
  const content = revision?.content ?? {};
  const [namespace, key] = splitAddress(summary.address);
  const name = stringValue(content.name) || stringValue(content.title) || summary.title || key;
  const definition = stringValue(content.definition) || stringValue(content.summary) || summary.summary || "尚未声明规范定义。";
  const aliases = stringArray(content.aliases);
  const includes = stringArray(content.includes);
  const excludes = stringArray(content.excludes);
  const examples = stringArray(content.examples);
  const type = typeLabels[summary.assetType];
  const typeSpec = projectTypeSpec(type, content);
  const evidence = revision?.evidence ?? [];
  const readiness = projectReadiness(summary, definition, evidence.length, relations.length, Boolean(detail));
  const revisionId = revision?.id ?? summary.currentRevisionId ?? "尚无修订";
  const recordedAt = revision?.createdAt ?? summary.updatedAt;
  const relationRecords = relations.map((relation) => ({
    id: relation.id,
    type: relation.predicate,
    plane: relation.plane,
    assertionState: relation.assertionState,
    targetId: relation.counterpart.id,
    targetName: relation.counterpart.title || relation.counterpart.address,
    direction: relation.direction,
    evidence: relation.evidenceArtifactId ?? relation.sourceRevisionId ?? "未关联证据",
    release: "当前注册表",
  }));
  const owner = stringValue(content.owner) || "未分配";
  const maintainer = stringValue(content.maintainer) || "未分配";
  const domain = stringValue(content.domain) || namespace || "未分域";

  return {
    id: summary.id,
    detailLoaded: Boolean(detail),
    revision: revision?.sequence ? `@${revision.sequence}` : "@0",
    namespace,
    key,
    name,
    aliases,
    type,
    status: summary.lifecycleState === "active" && readiness.every((gate) => gate.state !== "warning") ? "已发布" : summary.lifecycleState === "active" ? "需关注" : "草稿",
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
    release: "尚未发布",
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
      workflowState: "released",
      name,
      aliases,
      definition,
      includes,
      excludes,
      examples,
      typeSpec,
    },
    deployment: { environment: "production", state: "unreleased" },
    qualitySnapshot: {
      snapshotId: `catalog:${revisionId}`,
      state: readiness.some((gate) => gate.state === "warning") ? "warning" : "healthy",
      policyVersion: "M1 registry completeness",
      evaluatedAt: formatTimestamp(summary.updatedAt),
      blockingCount: 0,
      warningCount: readiness.filter((gate) => gate.state === "warning").length,
    },
    claims: definition === "尚未声明规范定义。" ? [] : [{ claimId: `claim:${revisionId}:definition`, assetRevisionId: revisionId, fieldPath: "definition", label: "规范定义", value: definition }],
    evidenceArtifacts: evidence.map((item) => ({ evidenceId: item.id, sourceRevision: item.sourceRevisionId ?? "未关联来源修订", authority: evidenceAuthorities[item.evidenceType] ?? "OBSERVED", observedAt: formatTimestamp(item.createdAt), scope: item.fieldPath ?? item.locator, digest: item.contentDigest })),
    evidenceLinks: evidence.map((item) => ({ claimId: `claim:${revisionId}:${item.fieldPath ?? "revision"}`, evidenceId: item.id, polarity: item.role === "conflicts" ? "contradicts" : "supports", strength: item.role === "supports" ? "primary" : "corroborating" })),
    validationRuns: [],
    consumerBindings: [],
    wikiContext: { contextId: `catalog:${revisionId}`, assetRevisionId: revisionId, canonicalSummary: definition, retrievalTerms: [name, ...aliases], disambiguationRules: [], groundedQuestions: [], compiledAt: formatTimestamp(recordedAt) },
    ontologyContext: {
      ontologyId: "尚未发布本体版本",
      revisionId,
      domainPath: namespace ? namespace.split(".") : [],
      assetId: summary.id,
      parentConcepts: relationRecords.filter((item) => item.type === "broader_than").map((item) => item.targetName),
      relatedConcepts: relationRecords.map((item) => item.targetName),
      relationConstraints: [],
      consistencyState: "consistent",
      consistencyIssues: [],
      revisionDelta: { addedRelations: 0, removedRelations: 0, changedConstraints: 0 },
    },
  };
}

function projectReadiness(summary: CatalogAsset, definition: string, evidenceCount: number, relationCount: number, detailLoaded: boolean): AssetReadinessGate[] {
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
  const base = { expression: stringValue(content.expression) || "尚未声明", grain: stringValue(content.grain) || "尚未声明", defaultTime: stringValue(content.defaultTime) || "不适用", unit: stringValue(content.unit) || "未声明", aggregation: stringValue(content.aggregation) || "未声明" };
  if (type === "指标") return { ...base, kind: "metric", metricKind: "simple", allowedDimensions: [], comparisonSemantics: "尚未声明" };
  if (type === "度量") return { ...base, kind: "measure", additivity: "non_additive", nullHandling: "尚未声明" };
  if (type === "维度") return { ...base, kind: "dimension", valueType: "unknown", nullSemantics: "尚未声明", hierarchy: [] };
  if (type === "业务实体") return { ...base, kind: "entity", entityKeys: [], identityPolicy: "尚未声明", lifecycle: "尚未声明" };
  if (type === "语义模型") return { ...base, kind: "model", primaryEntity: "尚未声明", publicMembers: [], joinPathPolicy: "尚未声明" };
  if (type === "分群") return { ...base, kind: "segment", baseEntity: "尚未声明", refreshPolicy: "尚未声明", effectiveTime: "尚未声明" };
  return { ...base, kind: "concept", conceptClass: "业务概念", disambiguationRule: "尚未声明" };
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
