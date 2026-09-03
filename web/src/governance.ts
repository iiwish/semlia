import { createSemliaClient, type components } from "@semlia/sdk-typescript";

export type GovernanceProposalSummary = components["schemas"]["GovernanceProposalSummary"];
export type GovernanceProposalDetail = components["schemas"]["GovernanceProposalDetail"];
export type GovernanceProposalState = components["schemas"]["GovernanceProposalState"];
export type GovernanceChangeSetItem = components["schemas"]["GovernanceChangeSetItem"];
export type GovernanceChangeSetItemRecord = components["schemas"]["GovernanceChangeSetItemRecord"];
export type GovernanceTargetObjectType = components["schemas"]["GovernanceTargetObjectType"];
export type GovernanceValidationRun = components["schemas"]["GovernanceValidationRun"];
export type GovernanceValidationSeverity = components["schemas"]["GovernanceValidationSeverity"];
export type GovernancePolicyDecision = components["schemas"]["GovernancePolicyDecision"];
export type GovernanceReview = components["schemas"]["GovernanceReview"];
export type GovernanceReviewCommandRequest = components["schemas"]["GovernanceReviewCommandRequest"];
export type GovernanceReviewBatch = components["schemas"]["GovernanceReviewBatch"];
export type GovernanceReviewBatchDetail = components["schemas"]["GovernanceReviewBatchDetail"];
export type GovernanceRelease = components["schemas"]["GovernanceRelease"];
export type GovernanceReleaseDetail = components["schemas"]["GovernanceReleaseDetail"];
export type GovernanceModelProviderDetail = components["schemas"]["GovernanceModelProviderDetail"];
export type GovernanceModelProvider = components["schemas"]["GovernanceModelProvider"];
export type GovernanceModelSetting = components["schemas"]["GovernanceModelSetting"];
export type GovernanceModelProtocol = components["schemas"]["GovernanceModelProtocol"];
export type GovernanceModelKind = components["schemas"]["GovernanceModelKind"];
export type GovernanceAgentRun = components["schemas"]["GovernanceAgentRun"];
export type GovernanceGeneratedProposal = components["schemas"]["GovernanceGeneratedProposal"];
export type CreateGovernanceProposalRequest = components["schemas"]["CreateGovernanceProposalRequest"];
export type CreateGovernanceModelProviderRequest = components["schemas"]["CreateGovernanceModelProviderRequest"];
export type UpdateGovernanceModelProviderRequest = components["schemas"]["UpdateGovernanceModelProviderRequest"];
export type CreateGovernanceModelSettingRequest = components["schemas"]["CreateGovernanceModelSettingRequest"];
export type UpdateGovernanceModelSettingRequest = components["schemas"]["UpdateGovernanceModelSettingRequest"];
export type GovernanceGenerateProposalRequest = components["schemas"]["GovernanceGenerateProposalRequest"];

export const GOVERNANCE_ERROR_CODES = {
  SEPARATION_OF_DUTY: "SEPARATION_OF_DUTY",
  NOT_FOUND: "NOT_FOUND",
  CONFLICT: "CONFLICT",
  INVALID_ARGUMENT: "INVALID_ARGUMENT",
  NO_SUBSTANTIVE_CHANGE: "NO_SUBSTANTIVE_CHANGE",
  BLOCKED_BY_FINDINGS: "BLOCKED_BY_FINDINGS",
  NO_APPROVING_REVIEW: "NO_APPROVING_REVIEW",
  PROPOSAL_NOT_IN_REVIEW: "PROPOSAL_NOT_IN_REVIEW",
  PROVIDER_UNAVAILABLE: "PROVIDER_UNAVAILABLE",
  PROVIDER_UNSUPPORTED: "PROVIDER_UNSUPPORTED",
  AI_OUTPUT_INVALID: "AI_OUTPUT_INVALID",
  NO_MATCHING_GRANT: "NO_MATCHING_GRANT",
} as const;

/** A governance API failure. `code` is the stable wire code when the API answered. */
export class GovernanceApiError extends Error {
  readonly code: string;
  readonly details: Record<string, unknown>;

  constructor(message: string, code = "REQUEST_FAILED", details: Record<string, unknown> = {}) {
    super(message);
    this.name = "GovernanceApiError";
    this.code = code;
    this.details = details;
  }
}

const client = createSemliaClient({ baseUrl: "" });

function failure(error: unknown, fallbackMessage: string): GovernanceApiError {
  if (error && typeof error === "object" && "code" in error && typeof error.code === "string") {
    const message = "message" in error && typeof error.message === "string" ? error.message : fallbackMessage;
    const details = "details" in error && error.details && typeof error.details === "object" && !Array.isArray(error.details) ? error.details as Record<string, unknown> : {};
    return new GovernanceApiError(message, error.code, details);
  }
  if (error instanceof GovernanceApiError) return error;
  return new GovernanceApiError(fallbackMessage);
}

// --- Proposals ---

export async function listProposals(workspaceId: string, signal?: AbortSignal): Promise<GovernanceProposalSummary[]> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/proposals", {
    params: { path: { workspaceId }, query: { limit: 100 } },
    signal,
  });
  if (!response.data) throw failure(response.error, "提案列表加载失败。");
  return response.data.items;
}

export async function createProposal(workspaceId: string, input: CreateGovernanceProposalRequest): Promise<GovernanceProposalDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/proposals", {
    params: { path: { workspaceId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "提案创建失败。");
  return response.data;
}

export async function getProposal(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernanceProposalDetail> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}", {
    params: { path: { workspaceId, proposalId } },
    signal,
  });
  if (!response.data) throw failure(response.error, "提案详情加载失败。");
  return response.data;
}

export async function submitProposal(workspaceId: string, proposalId: string): Promise<GovernanceProposalDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/submit", {
    params: { path: { workspaceId, proposalId } },
  });
  if (!response.data) throw failure(response.error, "提案提交失败。");
  return response.data;
}

export async function listValidationRuns(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernanceValidationRun[]> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/validation-runs", {
    params: { path: { workspaceId, proposalId } },
    signal,
  });
  if (!response.data) throw failure(response.error, "验证运行加载失败。");
  return response.data.items;
}

/** Returns the latest policy decision, or null while validation has not completed yet (404). */
export async function getPolicyDecision(workspaceId: string, proposalId: string, signal?: AbortSignal): Promise<GovernancePolicyDecision | null> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/policy-decision", {
    params: { path: { workspaceId, proposalId } },
    signal,
  });
  if (response.data) return response.data;
  if (response.error && typeof response.error === "object" && "code" in response.error && response.error.code === GOVERNANCE_ERROR_CODES.NOT_FOUND) return null;
  throw failure(response.error, "策略决策加载失败。");
}

export async function createReview(workspaceId: string, proposalId: string, input: GovernanceReviewCommandRequest): Promise<GovernanceReview> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/proposals/{proposalId}/reviews", {
    params: { path: { workspaceId, proposalId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "评审记录失败。");
  return response.data;
}

// --- Review batches ---

export async function listReviewBatches(workspaceId: string, signal?: AbortSignal): Promise<GovernanceReviewBatch[]> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/review-batches", {
    params: { path: { workspaceId }, query: { limit: 50 } },
    signal,
  });
  if (!response.data) throw failure(response.error, "评审批次加载失败。");
  return response.data.items;
}

export async function assembleReviewBatches(workspaceId: string): Promise<GovernanceReviewBatch[]> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/review-batches", {
    params: { path: { workspaceId } },
    body: undefined,
  });
  if (!response.data) throw failure(response.error, "评审批次汇编失败。");
  return response.data.items;
}

export async function getReviewBatch(workspaceId: string, batchId: string, signal?: AbortSignal): Promise<GovernanceReviewBatchDetail> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/review-batches/{batchId}", {
    params: { path: { workspaceId, batchId } },
    signal,
  });
  if (!response.data) throw failure(response.error, "评审批次详情加载失败。");
  return response.data;
}

export async function confirmReviewBatch(workspaceId: string, batchId: string, input: GovernanceReviewCommandRequest): Promise<GovernanceReviewBatchDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/review-batches/{batchId}/confirm", {
    params: { path: { workspaceId, batchId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "评审批次确认失败。");
  return response.data;
}

// --- Releases ---

export async function listReleases(workspaceId: string, signal?: AbortSignal): Promise<GovernanceRelease[]> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/releases", {
    params: { path: { workspaceId }, query: { limit: 50 } },
    signal,
  });
  if (!response.data) throw failure(response.error, "发布记录加载失败。");
  return response.data.items;
}

export async function publishRelease(workspaceId: string, proposalId: string): Promise<GovernanceReleaseDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/releases", {
    params: { path: { workspaceId } },
    body: { proposalId },
  });
  if (!response.data) throw failure(response.error, "发布失败。");
  return response.data;
}

export async function getRelease(workspaceId: string, releaseId: string, signal?: AbortSignal): Promise<GovernanceReleaseDetail> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/releases/{releaseId}", {
    params: { path: { workspaceId, releaseId } },
    signal,
  });
  if (!response.data) throw failure(response.error, "发布详情加载失败。");
  return response.data;
}

export async function rollbackRelease(workspaceId: string, releaseId: string): Promise<GovernanceReleaseDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/releases/{releaseId}/rollback", {
    params: { path: { workspaceId, releaseId } },
  });
  if (!response.data) throw failure(response.error, "回滚失败。");
  return response.data;
}

// --- Model configuration ---

export async function listModelProviders(workspaceId: string, signal?: AbortSignal): Promise<GovernanceModelProviderDetail[]> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/governance/model-providers", {
    params: { path: { workspaceId } },
    signal,
  });
  if (!response.data) throw failure(response.error, "模型配置加载失败。");
  return response.data.items;
}

export async function createModelProvider(workspaceId: string, input: CreateGovernanceModelProviderRequest): Promise<GovernanceModelProviderDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/model-providers", {
    params: { path: { workspaceId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "模型供应商创建失败。");
  return response.data;
}

export async function updateModelProvider(workspaceId: string, providerId: string, input: UpdateGovernanceModelProviderRequest): Promise<GovernanceModelProviderDetail> {
  const response = await client.PUT("/api/v1/workspaces/{workspaceId}/governance/model-providers/{providerId}", {
    params: { path: { workspaceId, providerId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "模型供应商更新失败。");
  return response.data;
}

export async function createModelSetting(workspaceId: string, input: CreateGovernanceModelSettingRequest): Promise<GovernanceModelSetting> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/model-settings", {
    params: { path: { workspaceId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "模型条目创建失败。");
  return response.data;
}

export async function updateModelSetting(workspaceId: string, settingId: string, input: UpdateGovernanceModelSettingRequest): Promise<GovernanceModelSetting> {
  const response = await client.PUT("/api/v1/workspaces/{workspaceId}/governance/model-settings/{settingId}", {
    params: { path: { workspaceId, settingId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "模型条目更新失败。");
  return response.data;
}

export async function setDefaultModelSetting(workspaceId: string, settingId: string): Promise<GovernanceModelSetting> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/model-settings/{settingId}/set-default", {
    params: { path: { workspaceId, settingId } },
  });
  if (!response.data) throw failure(response.error, "默认模型切换失败。");
  return response.data;
}

// --- Generation ---

export async function generateProposal(workspaceId: string, input: GovernanceGenerateProposalRequest): Promise<GovernanceGeneratedProposal> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/governance/agent-runs/generate-proposal", {
    params: { path: { workspaceId } },
    body: input,
  });
  if (!response.data) throw failure(response.error, "AI 提案生成失败。");
  return response.data;
}
