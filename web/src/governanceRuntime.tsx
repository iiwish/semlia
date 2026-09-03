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
  type GovernanceReview,
  type GovernanceReviewBatchDetail,
  type GovernanceReviewBatch,
  type GovernanceReviewCommandRequest,
  type GovernanceValidationRun,
  type UpdateGovernanceModelProviderRequest,
  type UpdateGovernanceModelSettingRequest,
} from "./governance";
import { createGovernanceFixture, type GovernanceFixtureClient } from "./governanceFixture";
import { listAssetRevisions } from "./catalog";
import type { AssetWorkflowState } from "./types";

export type GovernanceDataState = "idle" | "ready" | "error";

/**
 * One governance API surface shared by every M2 view. The real client comes
 * from web/src/governance.ts; the fixture client (VITE_CATALOG_FIXTURE=1)
 * implements the same shapes in memory.
 */
export interface GovernanceApi {
  listProposals(workspaceId: string, signal?: AbortSignal): Promise<GovernanceProposalSummary[]>;
  createProposal(workspaceId: string, input: CreateGovernanceProposalRequest): Promise<GovernanceProposalDetail>;
  getProposal(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernanceProposalDetail>;
  submitProposal(workspaceId: string, proposalId: string): Promise<GovernanceProposalDetail>;
  listValidationRuns(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernanceValidationRun[]>;
  getPolicyDecision(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernancePolicyDecision | null>;
  createReview(workspaceId: string, proposalId: string, input: GovernanceReviewCommandRequest): Promise<GovernanceReview>;
  listReviewBatches(workspaceId: string, signal?: AbortSignal): Promise<GovernanceReviewBatch[]>;
  assembleReviewBatches(workspaceId: string): Promise<GovernanceReviewBatch[]>;
  getReviewBatch(workspaceId: string, batchId: string, signal?: AbortSignal): Promise<GovernanceReviewBatchDetail>;
  confirmReviewBatch(workspaceId: string, batchId: string, input: GovernanceReviewCommandRequest): Promise<GovernanceReviewBatchDetail>;
  listReleases(workspaceId: string, signal?: AbortSignal): Promise<GovernanceRelease[]>;
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

function fixtureApi(inner: GovernanceFixtureClient): GovernanceApi {
  return {
    listProposals: () => inner.listProposals(),
    createProposal: (_workspaceId, input) => inner.createProposal(input),
    getProposal: (_workspaceId, proposalId) => inner.getProposal(proposalId),
    submitProposal: (_workspaceId, proposalId) => inner.submitProposal(proposalId),
    listValidationRuns: (_workspaceId, proposalId) => inner.listValidationRuns(proposalId),
    getPolicyDecision: (_workspaceId, proposalId) => inner.getPolicyDecision(proposalId),
    createReview: (_workspaceId, proposalId, input) => inner.createReview(proposalId, input),
    listReviewBatches: () => inner.listReviewBatches(),
    assembleReviewBatches: () => inner.assembleReviewBatches(),
    getReviewBatch: (_workspaceId, batchId) => inner.getReviewBatch(batchId),
    confirmReviewBatch: (_workspaceId, batchId, input) => inner.confirmReviewBatch(batchId, input),
    listReleases: () => inner.listReleases(),
    publishRelease: (_workspaceId, proposalId) => inner.publishRelease(proposalId),
    getRelease: (_workspaceId, releaseId) => inner.getRelease(releaseId),
    rollbackRelease: (_workspaceId, releaseId) => inner.rollbackRelease(releaseId),
    listModelProviders: () => inner.listModelProviders(),
    createModelProvider: (_workspaceId, input) => inner.createModelProvider(input),
    updateModelProvider: (_workspaceId, providerId, input) => inner.updateModelProvider(providerId, input),
    createModelSetting: (_workspaceId, input) => inner.createModelSetting(input),
    updateModelSetting: (_workspaceId, settingId, input) => inner.updateModelSetting(settingId, input),
    setDefaultModelSetting: (_workspaceId, settingId) => inner.setDefaultModelSetting(settingId),
    generateProposal: (_workspaceId, input) => inner.generateProposal(input),
  };
}

export interface GovernanceRuntimeValue {
  workspaceId: string;
  proposals: GovernanceProposalSummary[];
  proposalsState: GovernanceDataState;
  proposalsError: string;
  releases: GovernanceRelease[];
  releaseDetails: Record<string, GovernanceReleaseDetail>;
  releasesState: GovernanceDataState;
  releasesError: string;
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
  refreshBatches: () => void;
  refreshModelConfig: () => void;
  refreshAll: () => void;
  loadProposal: (proposalId: string) => Promise<GovernanceProposalDetail>;
  loadValidationRuns: (proposalId: string) => Promise<GovernanceValidationRun[]>;
  loadPolicyDecision: (proposalId: string) => Promise<GovernancePolicyDecision | null>;
  loadBatchDetail: (batchId: string) => Promise<GovernanceReviewBatchDetail>;
  loadReleaseDetail: (releaseId: string) => Promise<GovernanceReleaseDetail>;
  submitProposal: (proposalId: string) => Promise<GovernanceProposalDetail>;
  createAndSubmitProposal: (input: CreateGovernanceProposalRequest) => Promise<GovernanceProposalDetail>;
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

export function GovernanceRuntimeProvider({ children, workspaceId, fixture = false }: { children: ReactNode; workspaceId: string; fixture?: boolean }) {
  const api = useMemo<GovernanceApi>(() => (fixture ? fixtureApi(createGovernanceFixture()) : realApi), [fixture]);
  const [proposals, setProposals] = useState<GovernanceProposalSummary[]>([]);
  const [proposalsState, setProposalsState] = useState<GovernanceDataState>("idle");
  const [proposalsError, setProposalsError] = useState("");
  const [releases, setReleases] = useState<GovernanceRelease[]>([]);
  const [releasesState, setReleasesState] = useState<GovernanceDataState>("idle");
  const [releasesError, setReleasesError] = useState("");
  const [releaseDetails, setReleaseDetails] = useState<Record<string, GovernanceReleaseDetail>>({});
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

  const refreshProposals = useCallback(() => setRefreshVersion((version) => version + 1), []);

  useEffect(() => {
    if (!workspaceId) return;
    const controller = new AbortController();
    api.listProposals(workspaceId, controller.signal)
      .then((items) => {
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
    api.listReleases(workspaceId, controller.signal)
      .then(async (items) => {
        const details = await Promise.all(items.map((release) => api.getRelease(workspaceId, release.id).catch(() => undefined)));
        const detailMap: Record<string, GovernanceReleaseDetail> = {};
        for (const detail of details) {
          if (detail) detailMap[detail.id] = detail;
        }
        setReleaseDetails(detailMap);
        setReleases(items);
        setReleasesState("ready");
        setReleasesError("");
      })
      .catch((reason: Error) => {
        if (!controller.signal.aborted) {
          setReleasesState("error");
          setReleasesError(reason.message);
        }
      });
    return () => controller.abort();
  }, [api, refreshVersion, workspaceId]);

  useEffect(() => {
    if (!workspaceId) return;
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
  }, [api, refreshVersion, workspaceId]);

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

  const loadProposal = useCallback(async (proposalId: string) => api.getProposal(workspaceId, proposalId), [api, workspaceId]);

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

  const loadReleaseDetail = useCallback((releaseId: string) => api.getRelease(workspaceId, releaseId), [api, workspaceId]);

  const submitProposal = useCallback((proposalId: string) => api.submitProposal(workspaceId, proposalId), [api, workspaceId]);

  const createAndSubmitProposal = useCallback(async (input: CreateGovernanceProposalRequest) => {
    const draft = await api.createProposal(workspaceId, input);
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

  const publishProposal = useCallback(async (proposalId: string) => {
    const release = await api.publishRelease(workspaceId, proposalId);
    refreshReleases();
    return release;
  }, [api, refreshReleases, workspaceId]);

  const rollback = useCallback(async (releaseId: string) => {
    const release = await api.rollbackRelease(workspaceId, releaseId);
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
      revisionRequests.current[requestKey] = listAssetRevisions(workspaceId, assetId)
        .then((page) => {
          for (const revision of page.items) {
            revisionLabels.current[revision.id] = `@${revision.sequence}`;
          }
        })
        .catch(() => undefined);
    }
    await revisionRequests.current[requestKey];
    return revisionLabels.current[revisionId] ?? revisionId;
  }, [workspaceId]);

  const value = useMemo<GovernanceRuntimeValue>(() => ({
    workspaceId,
    proposals,
    proposalsState,
    proposalsError,
    releases,
    releaseDetails,
    releasesState,
    releasesError,
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
    generateProposal, loadBatchDetail, loadPolicyDecision, loadProposal, loadReleaseDetail, loadValidationRuns, modelProviders, modelProvidersError,
    modelProvidersState, proposals, proposalsError, proposalsState, publishProposal, refreshBatches, refreshModelConfig, refreshProposals,
    refreshReleases, releaseDetails, releases, releasesError, releasesState, resolveRevisionLabel, reviewProposal, rollback, reviews,
    setDefaultModelSetting, submitProposal, updateModelSetting, updateProvider, validationRuns, workflowStateByAsset, workspaceId,
  ]);

  return <GovernanceRuntimeContext.Provider value={value}>{children}</GovernanceRuntimeContext.Provider>;
}

export function useGovernanceRuntime() {
  const value = useContext(GovernanceRuntimeContext);
  if (!value) throw new Error("useGovernanceRuntime must be used inside GovernanceRuntimeProvider");
  return value;
}
