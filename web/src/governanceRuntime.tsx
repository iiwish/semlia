/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import {
  createModelProvider as apiCreateModelProvider,
  createModelSetting as apiCreateModelSetting,
  createProposal as apiCreateProposal,
  createReview as apiCreateReview,
  confirmReviewBatch as apiConfirmReviewBatch,
  assembleReviewBatches as apiAssembleReviewBatches,
  generateProposal as apiGenerateProposal,
  getPolicyDecision as apiGetPolicyDecision,
  getProposal as apiGetProposal,
  getRelease as apiGetRelease,
  getReviewBatch as apiGetReviewBatch,
  listModelProviders as apiListModelProviders,
  listProposals as apiListProposals,
  listReleases as apiListReleases,
  listReviews as apiListReviews,
  listReviewBatches as apiListReviewBatches,
  listValidationRuns as apiListValidationRuns,
  publishRelease as apiPublishRelease,
  rollbackRelease as apiRollbackRelease,
  setDefaultModelSetting as apiSetDefaultModelSetting,
  submitProposal as apiSubmitProposal,
  updateModelProvider as apiUpdateModelProvider,
  updateModelSetting as apiUpdateModelSetting,
  type CreateGovernanceModelProviderRequest,
  type CreateGovernanceModelSettingRequest,
  type CreateGovernanceProposalRequest,
  type GovernanceGenerateProposalRequest,
  type GovernanceGeneratedProposal,
  type GovernanceModelProviderDetail,
  type GovernanceModelSetting,
  type GovernancePolicyDecision,
  type GovernanceProposalDetail,
  type GovernanceProposalSummary,
  type GovernanceRelease,
  type GovernanceReleaseDetail,
  type GovernanceReleasePage,
  type GovernanceReview,
  type GovernanceReviewBatchDetail,
  type GovernanceReviewBatch,
  type GovernanceReviewCommandRequest,
  type GovernanceValidationRun,
  type UpdateGovernanceModelProviderRequest,
  type UpdateGovernanceModelSettingRequest,
} from "./governance";
import { listAssetRevisions } from "./catalog";
import type { AssetWorkflowState } from "./types";

export type GovernanceDataState = "idle" | "ready" | "error";
export type GovernanceReleaseDetailState = { state: "loading" | "ready" | "error"; error: string };

/** Shared governance API contract for the normal workspace. */
export interface GovernanceApi {
  listProposals(workspaceId: string, signal?: AbortSignal): Promise<GovernanceProposalSummary[]>;
  createProposal(workspaceId: string, input: CreateGovernanceProposalRequest): Promise<GovernanceProposalDetail>;
  getProposal(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernanceProposalDetail>;
  submitProposal(workspaceId: string, proposalId: string): Promise<GovernanceProposalDetail>;
  listValidationRuns(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernanceValidationRun[]>;
  getPolicyDecision(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernancePolicyDecision | null>;
  createReview(workspaceId: string, proposalId: string, input: GovernanceReviewCommandRequest): Promise<GovernanceReview>;
  listReviews(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernanceReview[]>;
  listReviewBatches(workspaceId: string, signal?: AbortSignal): Promise<GovernanceReviewBatch[]>;
  assembleReviewBatches(workspaceId: string): Promise<GovernanceReviewBatch[]>;
  getReviewBatch(workspaceId: string, batchId: string, signal?: AbortSignal): Promise<GovernanceReviewBatchDetail>;
  confirmReviewBatch(workspaceId: string, batchId: string, input: GovernanceReviewCommandRequest): Promise<GovernanceReviewBatchDetail>;
  listReleases(workspaceId: string, cursor?: string, signal?: AbortSignal): Promise<GovernanceReleasePage>;
  publishRelease(workspaceId: string, proposalId: string): Promise<GovernanceReleaseDetail>;
  getRelease(workspaceId: string, releaseId: string, signal?: AbortSignal): Promise<GovernanceReleaseDetail>;
  rollbackRelease(workspaceId: string, releaseId: string): Promise<GovernanceReleaseDetail>;
  listModelProviders(workspaceId: string, signal?: AbortSignal): Promise<GovernanceModelProviderDetail[]>;
  createModelProvider(workspaceId: string, input: CreateGovernanceModelProviderRequest): Promise<GovernanceModelProviderDetail>;
  updateModelProvider(workspaceId: string, providerId: string, input: UpdateGovernanceModelProviderRequest): Promise<GovernanceModelProviderDetail>;
  createModelSetting(workspaceId: string, input: CreateGovernanceModelSettingRequest): Promise<GovernanceModelSetting>;
  updateModelSetting(workspaceId: string, settingId: string, input: UpdateGovernanceModelSettingRequest): Promise<GovernanceModelSetting>;
  setDefaultModelSetting(workspaceId: string, settingId: string): Promise<GovernanceModelSetting>;
  generateProposal(workspaceId: string, input: GovernanceGenerateProposalRequest): Promise<GovernanceGeneratedProposal>;
}

const realApi: GovernanceApi = {
  listProposals: apiListProposals,
  createProposal: apiCreateProposal,
  getProposal: apiGetProposal,
  submitProposal: apiSubmitProposal,
  listValidationRuns: apiListValidationRuns,
  getPolicyDecision: apiGetPolicyDecision,
  createReview: apiCreateReview,
  listReviews: apiListReviews,
  listReviewBatches: apiListReviewBatches,
  assembleReviewBatches: apiAssembleReviewBatches,
  getReviewBatch: apiGetReviewBatch,
  confirmReviewBatch: apiConfirmReviewBatch,
  listReleases: apiListReleases,
  publishRelease: apiPublishRelease,
  getRelease: apiGetRelease,
  rollbackRelease: apiRollbackRelease,
  listModelProviders: apiListModelProviders,
  createModelProvider: apiCreateModelProvider,
  updateModelProvider: apiUpdateModelProvider,
  createModelSetting: apiCreateModelSetting,
  updateModelSetting: apiUpdateModelSetting,
  setDefaultModelSetting: apiSetDefaultModelSetting,
  generateProposal: apiGenerateProposal,
};

export interface GovernanceRuntimeValue {
  workspaceId: string;
  proposals: GovernanceProposalSummary[];
  proposalDetails: Record<string, GovernanceProposalDetail>;
  proposalsState: GovernanceDataState;
  proposalsError: string;
  releases: GovernanceRelease[];
  releaseDetails: Record<string, GovernanceReleaseDetail>;
  releaseDetailStates: Record<string, GovernanceReleaseDetailState>;
  releasesState: GovernanceDataState;
  releasesError: string;
  releasesTotal?: number;
  releasesNextCursor?: string;
  releasesLoadingMore: boolean;
  releasesAppendError: string;
  batches: GovernanceReviewBatch[];
  batchesState: GovernanceDataState;
  batchesError: string;
  modelProviders: GovernanceModelProviderDetail[];
  modelProvidersState: GovernanceDataState;
  modelProvidersError: string;
  decisions: Record<string, GovernancePolicyDecision>;
  validationRuns: Record<string, GovernanceValidationRun[]>;
  reviews: Record<string, GovernanceReview>;
  workflowStateByAsset: Record<string, AssetWorkflowState>;
  refreshProposals: () => void;
  refreshReleases: () => void;
  loadMoreReleases: () => Promise<void>;
  refreshBatches: () => void;
  refreshModelConfig: () => void;
  refreshAll: () => void;
  loadProposal: (proposalId: string) => Promise<GovernanceProposalDetail>;
  loadValidationRuns: (proposalId: string) => Promise<GovernanceValidationRun[]>;
  loadPolicyDecision: (proposalId: string) => Promise<GovernancePolicyDecision | null>;
  loadBatchDetail: (batchId: string) => Promise<GovernanceReviewBatchDetail>;
  loadReleaseDetail: (releaseId: string) => Promise<GovernanceReleaseDetail>;
  submitProposal: (proposalId: string) => Promise<GovernanceProposalDetail>;
  createAndSubmitProposal: (input: CreateGovernanceProposalRequest, onCreated?: (draft: GovernanceProposalDetail) => void) => Promise<GovernanceProposalDetail>;
  reviewProposal: (proposalId: string, input: GovernanceReviewCommandRequest) => Promise<GovernanceReview>;
  assembleBatches: () => Promise<GovernanceReviewBatch[]>;
  confirmBatch: (batchId: string, input: GovernanceReviewCommandRequest) => Promise<GovernanceReviewBatchDetail>;
  publishProposal: (proposalId: string) => Promise<GovernanceReleaseDetail>;
  rollback: (releaseId: string) => Promise<GovernanceReleaseDetail>;
  createProvider: (input: CreateGovernanceModelProviderRequest) => Promise<GovernanceModelProviderDetail>;
  updateProvider: (providerId: string, input: UpdateGovernanceModelProviderRequest) => Promise<GovernanceModelProviderDetail>;
  createModelSetting: (input: CreateGovernanceModelSettingRequest) => Promise<GovernanceModelSetting>;
  updateModelSetting: (settingId: string, input: UpdateGovernanceModelSettingRequest) => Promise<GovernanceModelSetting>;
  setDefaultModelSetting: (settingId: string) => Promise<GovernanceModelSetting>;
  generateProposal: (input: GovernanceGenerateProposalRequest) => Promise<GovernanceGeneratedProposal>;
  resolveRevisionLabel: (assetId: string, revisionId: string, fallback: string) => Promise<string>;
}

const GovernanceRuntimeContext = createContext<GovernanceRuntimeValue | null>(null);

const workflowStateMapping: Record<string, AssetWorkflowState> = {
  draft: "draft",
  proposed: "proposed",
  validating: "proposed",
  in_review: "in_review",
};

export function GovernanceRuntimeProvider({ children, workspaceId, api = realApi, reviewBatchesEnabled = true }: { children: ReactNode; workspaceId: string; api?: GovernanceApi; reviewBatchesEnabled?: boolean }) {
  const [proposals, setProposals] = useState<GovernanceProposalSummary[]>([]);
  const [proposalDetails, setProposalDetails] = useState<Record<string, GovernanceProposalDetail>>({});
  const [proposalsState, setProposalsState] = useState<GovernanceDataState>("idle");
  const [proposalsError, setProposalsError] = useState("");
  const [releases, setReleases] = useState<GovernanceRelease[]>([]);
  const [releasesState, setReleasesState] = useState<GovernanceDataState>("idle");
  const [releasesError, setReleasesError] = useState("");
  const [releasesTotal, setReleasesTotal] = useState<number>();
  const [releasesNextCursor, setReleasesNextCursor] = useState<string>();
  const [releasesLoadingMore, setReleasesLoadingMore] = useState(false);
  const [releasesAppendError, setReleasesAppendError] = useState("");
  const [releaseDetails, setReleaseDetails] = useState<Record<string, GovernanceReleaseDetail>>({});
  const [releaseDetailStates, setReleaseDetailStates] = useState<Record<string, GovernanceReleaseDetailState>>({});
  const [batches, setBatches] = useState<GovernanceReviewBatch[]>([]);
  const [batchesState, setBatchesState] = useState<GovernanceDataState>("idle");
  const [batchesError, setBatchesError] = useState("");
  const [modelProviders, setModelProviders] = useState<GovernanceModelProviderDetail[]>([]);
  const [modelProvidersState, setModelProvidersState] = useState<GovernanceDataState>("idle");
  const [modelProvidersError, setModelProvidersError] = useState("");
  const [decisions, setDecisions] = useState<Record<string, GovernancePolicyDecision>>({});
  const [validationRuns, setValidationRuns] = useState<Record<string, GovernanceValidationRun[]>>({});
  const [reviews, setReviews] = useState<Record<string, GovernanceReview>>({});
  const [refreshVersion, setRefreshVersion] = useState(0);
  const revisionLabels = useRef<Record<string, string>>({});
  const revisionRequests = useRef<Record<string, Promise<void>>>({});
  const workspaceRef = useRef(workspaceId);

  useEffect(() => {
    workspaceRef.current = workspaceId;
  }, [workspaceId]);

  const refreshProposals = useCallback(() => setRefreshVersion((version) => version + 1), []);

  useEffect(() => {
    if (!workspaceId) return;
    const controller = new AbortController();
    api.listProposals(workspaceId, controller.signal)
      .then(async (items) => {
        const reviewedProposals = items.filter((item) =>
          item.state === "in_review" || item.state === "rejected" || item.state === "released");
        const reviewPages = await Promise.all(reviewedProposals.map((item) =>
          api.listReviews(workspaceId, item.id, controller.signal)));
        const reviewMap: Record<string, GovernanceReview> = {};
        for (const page of reviewPages) {
          if (page[0]) reviewMap[page[0].proposalId] = page[0];
        }
        if (controller.signal.aborted) return;
        setReviews(reviewMap);
        setProposals(items);
        setProposalsState("ready");
        setProposalsError("");
      })
      .catch((reason: Error) => {
        if (!controller.signal.aborted) {
          setProposalsState("error");
          setProposalsError(reason.message);
        }
      });
    return () => controller.abort();
  }, [api, refreshVersion, workspaceId]);

  useEffect(() => {
    if (!workspaceId) return;
    const controller = new AbortController();
    api.listReleases(workspaceId, undefined, controller.signal)
      .then(async (page) => {
        const results = await Promise.all(page.items.map(async (release) => {
          try {
            return { release, detail: await api.getRelease(workspaceId, release.id, controller.signal) };
          } catch (reason) {
            return { release, error: reason instanceof Error ? reason.message : "发布清单读取失败。" };
          }
        }));
        if (controller.signal.aborted || workspaceRef.current !== workspaceId) return;
        const detailMap: Record<string, GovernanceReleaseDetail> = {};
        const detailStateMap: Record<string, GovernanceReleaseDetailState> = {};
        for (const result of results) {
          if (result.detail) {
            detailMap[result.detail.id] = result.detail;
            detailStateMap[result.release.id] = { state: "ready", error: "" };
          } else {
            detailStateMap[result.release.id] = { state: "error", error: result.error ?? "发布清单读取失败。" };
          }
        }
        setReleaseDetails(detailMap);
        setReleaseDetailStates(detailStateMap);
        setReleases(page.items);
        setReleasesTotal(page.page.total);
        setReleasesNextCursor(page.page.nextCursor);
        setReleasesLoadingMore(false);
        setReleasesAppendError("");
        setReleasesState("ready");
        setReleasesError("");
      })
      .catch((reason: Error) => {
        if (!controller.signal.aborted) {
          setReleasesState("error");
          setReleasesError(reason.message);
          setReleasesLoadingMore(false);
        }
      });
    return () => controller.abort();
  }, [api, refreshVersion, workspaceId]);

  useEffect(() => {
    if (!workspaceId || !reviewBatchesEnabled) return;
    const controller = new AbortController();
    api.listReviewBatches(workspaceId, controller.signal)
      .then((items) => {
        setBatches(items);
        setBatchesState("ready");
        setBatchesError("");
      })
      .catch((reason: Error) => {
        if (!controller.signal.aborted) {
          setBatchesState("error");
          setBatchesError(reason.message);
        }
      });
    return () => controller.abort();
  }, [api, refreshVersion, reviewBatchesEnabled, workspaceId]);

  useEffect(() => {
    if (!workspaceId) return;
    const controller = new AbortController();
    api.listModelProviders(workspaceId, controller.signal)
      .then((items) => {
        setModelProviders(items);
        setModelProvidersState("ready");
        setModelProvidersError("");
      })
      .catch((reason: Error) => {
        if (!controller.signal.aborted) {
          setModelProvidersState("error");
          setModelProvidersError(reason.message);
        }
      });
    return () => controller.abort();
  }, [api, refreshVersion, workspaceId]);

  const workflowStateByAsset = useMemo(() => {
    const states: Record<string, AssetWorkflowState> = {};
    for (const proposal of proposals) {
      const mapped = workflowStateMapping[proposal.state];
      if (proposal.assetId && mapped) states[proposal.assetId] = mapped;
    }
    return states;
  }, [proposals]);

  const loadProposal = useCallback(async (proposalId: string) => {
    const detail = await api.getProposal(workspaceId, proposalId);
    if (workspaceRef.current !== workspaceId) return detail;
    setProposalDetails((current) => ({ ...current, [detail.id]: detail }));
    setProposals((current) => current.map((item) => item.id === detail.id ? detail : item));
    return detail;
  }, [api, workspaceId]);

  const loadValidationRuns = useCallback(async (proposalId: string) => {
    const runs = await api.listValidationRuns(workspaceId, proposalId);
    setValidationRuns((current) => ({ ...current, [proposalId]: runs }));
    return runs;
  }, [api, workspaceId]);

  const loadPolicyDecision = useCallback(async (proposalId: string) => {
    const decision = await api.getPolicyDecision(workspaceId, proposalId);
    if (decision) setDecisions((current) => ({ ...current, [proposalId]: decision }));
    return decision;
  }, [api, workspaceId]);

  const loadBatchDetail = useCallback((batchId: string) => api.getReviewBatch(workspaceId, batchId), [api, workspaceId]);

  const loadReleaseDetail = useCallback(async (releaseId: string) => {
    const requestWorkspaceId = workspaceId;
    setReleaseDetailStates((current) => ({ ...current, [releaseId]: { state: "loading", error: "" } }));
    try {
      const detail = await api.getRelease(requestWorkspaceId, releaseId);
      if (workspaceRef.current !== requestWorkspaceId) return detail;
      setReleaseDetails((current) => ({ ...current, [detail.id]: detail }));
      setReleaseDetailStates((current) => ({ ...current, [releaseId]: { state: "ready", error: "" } }));
      return detail;
    } catch (reason) {
      if (workspaceRef.current === requestWorkspaceId) {
        setReleaseDetailStates((current) => ({ ...current, [releaseId]: { state: "error", error: reason instanceof Error ? reason.message : "发布清单读取失败。" } }));
      }
      throw reason;
    }
  }, [api, workspaceId]);

  const submitProposal = useCallback((proposalId: string) => api.submitProposal(workspaceId, proposalId), [api, workspaceId]);

  const createAndSubmitProposal = useCallback(async (input: CreateGovernanceProposalRequest, onCreated?: (draft: GovernanceProposalDetail) => void) => {
    const draft = await api.createProposal(workspaceId, input);
    onCreated?.(draft);
    const submitted = await api.submitProposal(workspaceId, draft.id);
    setValidationRuns((current) => ({ ...current, [submitted.id]: [] }));
    refreshProposals();
    return submitted;
  }, [api, refreshProposals, workspaceId]);

  const reviewProposal = useCallback(async (proposalId: string, input: GovernanceReviewCommandRequest) => {
    const review = await api.createReview(workspaceId, proposalId, input);
    setReviews((current) => ({ ...current, [proposalId]: review }));
    refreshProposals();
    return review;
  }, [api, refreshProposals, workspaceId]);

  const refreshBatches = useCallback(() => setRefreshVersion((version) => version + 1), []);

  const assembleBatches = useCallback(async () => {
    const created = await api.assembleReviewBatches(workspaceId);
    refreshBatches();
    return created;
  }, [api, refreshBatches, workspaceId]);

  const confirmBatch = useCallback(async (batchId: string, input: GovernanceReviewCommandRequest) => {
    const detail = await api.confirmReviewBatch(workspaceId, batchId, input);
    refreshBatches();
    return detail;
  }, [api, refreshBatches, workspaceId]);

  const refreshReleases = useCallback(() => setRefreshVersion((version) => version + 1), []);

  const loadMoreReleases = useCallback(async () => {
    const cursor = releasesNextCursor;
    if (!workspaceId || !cursor || releasesLoadingMore) return;
    const requestWorkspaceId = workspaceId;
    setReleasesLoadingMore(true);
    setReleasesAppendError("");
    try {
      const page = await api.listReleases(requestWorkspaceId, cursor);
      if (workspaceRef.current !== requestWorkspaceId) return;
      const results = await Promise.all(page.items.map(async (release) => {
        try {
          return { release, detail: await api.getRelease(requestWorkspaceId, release.id) };
        } catch (reason) {
          return { release, error: reason instanceof Error ? reason.message : "发布清单读取失败。" };
        }
      }));
      if (workspaceRef.current !== requestWorkspaceId) return;
      setReleases((current) => {
        const next = [...current];
        for (const release of page.items) {
          const index = next.findIndex((candidate) => candidate.id === release.id);
          if (index < 0) next.push(release);
          else next[index] = release;
        }
        return next;
      });
      setReleaseDetails((current) => {
        const next = { ...current };
        for (const result of results) if (result.detail) next[result.detail.id] = result.detail;
        return next;
      });
      setReleaseDetailStates((current) => {
        const next = { ...current };
        for (const result of results) next[result.release.id] = result.detail
          ? { state: "ready", error: "" }
          : { state: "error", error: result.error ?? "发布清单读取失败。" };
        return next;
      });
      setReleasesTotal(page.page.total);
      setReleasesNextCursor(page.page.nextCursor === cursor ? undefined : page.page.nextCursor);
      setReleasesAppendError(page.page.nextCursor === cursor ? "服务器返回了重复游标，已停止继续加载。" : "");
    } catch (reason) {
      if (workspaceRef.current !== requestWorkspaceId) return;
      setReleasesAppendError(reason instanceof Error ? reason.message : "无法加载更多发布记录。");
    } finally {
      if (workspaceRef.current === requestWorkspaceId) setReleasesLoadingMore(false);
    }
  }, [api, releasesLoadingMore, releasesNextCursor, workspaceId]);

  const publishProposal = useCallback(async (proposalId: string) => {
    const requestWorkspaceId = workspaceId;
    const release = await api.publishRelease(requestWorkspaceId, proposalId);
    if (workspaceRef.current !== requestWorkspaceId) return release;
    setReleases((current) => [release, ...current.filter((item) => item.id !== release.id)]);
    setReleaseDetails((current) => ({ ...current, [release.id]: release }));
    setReleaseDetailStates((current) => ({ ...current, [release.id]: { state: "ready", error: "" } }));
    refreshReleases();
    return release;
  }, [api, refreshReleases, workspaceId]);

  const rollback = useCallback(async (releaseId: string) => {
    const requestWorkspaceId = workspaceId;
    const release = await api.rollbackRelease(requestWorkspaceId, releaseId);
    if (workspaceRef.current !== requestWorkspaceId) return release;
    setReleases((current) => [release, ...current.filter((item) => item.id !== release.id)]);
    setReleaseDetails((current) => ({ ...current, [release.id]: release }));
    setReleaseDetailStates((current) => ({ ...current, [release.id]: { state: "ready", error: "" } }));
    refreshReleases();
    return release;
  }, [api, refreshReleases, workspaceId]);

  const refreshModelConfig = useCallback(() => setRefreshVersion((version) => version + 1), []);

  const createProvider = useCallback(async (input: CreateGovernanceModelProviderRequest) => {
    const detail = await api.createModelProvider(workspaceId, input);
    setModelProviders((current) => [...current, detail]);
    return detail;
  }, [api, workspaceId]);

  const updateProvider = useCallback(async (providerId: string, input: UpdateGovernanceModelProviderRequest) => {
    const detail = await api.updateModelProvider(workspaceId, providerId, input);
    setModelProviders((current) => current.map((item) => item.provider.id === providerId ? detail : item));
    return detail;
  }, [api, workspaceId]);

  const createModelSetting = useCallback(async (input: CreateGovernanceModelSettingRequest) => {
    const setting = await api.createModelSetting(workspaceId, input);
    setModelProviders((current) => current.map((item) => item.provider.id === input.providerId ? { ...item, models: [...item.models, setting] } : item));
    return setting;
  }, [api, workspaceId]);

  const updateModelSetting = useCallback(async (settingId: string, input: UpdateGovernanceModelSettingRequest) => {
    const setting = await api.updateModelSetting(workspaceId, settingId, input);
    setModelProviders((current) => current.map((item) => ({ ...item, models: item.models.map((model) => model.id === settingId ? setting : model) })));
    return setting;
  }, [api, workspaceId]);

  const setDefaultModelSetting = useCallback(async (settingId: string) => {
    const setting = await api.setDefaultModelSetting(workspaceId, settingId);
    setModelProviders((current) => current.map((item) => ({
      ...item,
      models: item.models.map((model) => model.kind === setting.kind ? { ...model, isDefault: model.id === settingId } : model),
    })));
    return setting;
  }, [api, workspaceId]);

  const generateProposal = useCallback(async (input: GovernanceGenerateProposalRequest) => {
    const generated = await api.generateProposal(workspaceId, input);
    refreshProposals();
    return generated;
  }, [api, refreshProposals, workspaceId]);

  const resolveRevisionLabel = useCallback(async (assetId: string, revisionId: string, fallback: string) => {
    const sequenceSuffix = revisionId.match(/@(\d+)$/);
    if (sequenceSuffix) return `@${sequenceSuffix[1]}`;
    if (!revisionId || revisionId.startsWith("rev_") === false) return fallback;
    if (revisionLabels.current[revisionId]) return revisionLabels.current[revisionId];
    const requestKey = `${assetId}:${revisionId}`;
    if (!revisionRequests.current[requestKey]) {
      revisionRequests.current[requestKey] = (async () => {
        let cursor: string | undefined;
        const seenCursors = new Set<string>();
        do {
          const page = await listAssetRevisions(workspaceId, assetId, cursor);
          for (const revision of page.items) {
            revisionLabels.current[revision.id] = `@${revision.sequence}`;
          }
          if (revisionLabels.current[revisionId] || !page.page.nextCursor || seenCursors.has(page.page.nextCursor)) break;
          cursor = page.page.nextCursor;
          seenCursors.add(cursor);
        } while (cursor);
      })().catch(() => undefined);
    }
    await revisionRequests.current[requestKey];
    return revisionLabels.current[revisionId] ?? revisionId;
  }, [workspaceId]);

  const value = useMemo<GovernanceRuntimeValue>(() => ({
    workspaceId,
    proposals,
    proposalDetails,
    proposalsState,
    proposalsError,
    releases,
    releaseDetails,
    releaseDetailStates,
    releasesState,
    releasesError,
    releasesTotal,
    releasesNextCursor,
    releasesLoadingMore,
    releasesAppendError,
    batches,
    batchesState,
    batchesError,
    modelProviders,
    modelProvidersState,
    modelProvidersError,
    decisions,
    validationRuns,
    reviews,
    workflowStateByAsset,
    refreshProposals,
    refreshReleases,
    loadMoreReleases,
    refreshBatches,
    refreshModelConfig,
    refreshAll: refreshProposals,
    loadProposal,
    loadValidationRuns,
    loadPolicyDecision,
    loadBatchDetail,
    loadReleaseDetail,
    submitProposal,
    createAndSubmitProposal,
    reviewProposal,
    assembleBatches,
    confirmBatch,
    publishProposal,
    rollback,
    createProvider,
    updateProvider,
    createModelSetting,
    updateModelSetting,
    setDefaultModelSetting,
    generateProposal,
    resolveRevisionLabel,
  }), [
    assembleBatches, batches, batchesError, batchesState, confirmBatch, createAndSubmitProposal, createModelSetting, createProvider, decisions,
    generateProposal, loadBatchDetail, loadMoreReleases, loadPolicyDecision, loadProposal, loadReleaseDetail, loadValidationRuns, modelProviders, modelProvidersError,
    modelProvidersState, proposalDetails, proposals, proposalsError, proposalsState, publishProposal, refreshBatches, refreshModelConfig, refreshProposals,
    refreshReleases, releaseDetails, releaseDetailStates, releases, releasesAppendError, releasesError, releasesLoadingMore, releasesNextCursor, releasesState, releasesTotal, resolveRevisionLabel, reviewProposal, rollback, reviews,
    setDefaultModelSetting, submitProposal, updateModelSetting, updateProvider, validationRuns, workflowStateByAsset, workspaceId,
  ]);

  return <GovernanceRuntimeContext.Provider value={value}>{children}</GovernanceRuntimeContext.Provider>;
}

export function useGovernanceRuntime() {
  const value = useContext(GovernanceRuntimeContext);
  if (!value) throw new Error("useGovernanceRuntime must be used inside GovernanceRuntimeProvider");
  return value;
}
