/*
 * Fixture harness for the M2 governance surfaces. Only reachable through the
 * e2e fixture flag (VITE_CATALOG_FIXTURE=1) — production failures never fall
 * back to this module. The fixture implements the same operation shapes as
 * web/src/governance.ts over in-memory state seeded from the accepted
 * session experience, so the governed journey is exercisable in e2e.
 */
import {
  GovernanceApiError,
  type GovernanceGenerateProposalRequest,
  type GovernanceGeneratedProposal,
  type GovernanceModelProviderDetail,
  type GovernanceModelSetting,
  type GovernancePolicyDecision,
  type GovernanceProposalDetail,
  type GovernanceProposalSummary,
  type GovernanceReleaseDetail,
  type GovernanceReview,
  type GovernanceReviewBatchDetail,
  type GovernanceReviewCommandRequest,
  type GovernanceValidationRun,
  type CreateGovernanceModelProviderRequest,
  type CreateGovernanceModelSettingRequest,
  type CreateGovernanceProposalRequest,
  type UpdateGovernanceModelProviderRequest,
  type UpdateGovernanceModelSettingRequest,
} from "./governance";
import { assetVersionReleases, assets as fixtureAssets } from "./data";

const FIXTURE_TIMESTAMP = "2026-09-03T09:00:00Z";

interface FixtureProposal {
  detail: GovernanceProposalDetail;
  runs: GovernanceValidationRun[];
  decision: GovernancePolicyDecision | null;
}

interface FixtureState {
  proposals: Map<string, FixtureProposal>;
  reviews: GovernanceReview[];
  batches: GovernanceReviewBatchDetail[];
  releases: GovernanceReleaseDetail[];
  providers: GovernanceModelProviderDetail[];
  counters: { proposal: number; change: number; review: number; batch: number; release: number; run: number; provider: number; setting: number };
}

function digest(counter: number): string {
  return `f1xture${String(counter).padStart(4, "0")}a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a9b8c7d6`.slice(0, 64);
}

function apiError(code: string, message: string): GovernanceApiError {
  return new GovernanceApiError(message, code, {});
}

function revisionSequenceOf(revisionId: string): number {
  const match = revisionId.match(/@(\d+)$/);
  return match ? Number(match[1]) : 0;
}

const validatorForField: Record<string, string> = {
  "Schema references": "schema",
  "Policy G1": "reference",
  "Consumer impact": "reference",
  "Cube compile": "structural",
  "Result regression": "structural",
};

const severityForState: Record<string, "info" | "warning" | "blocker"> = {
  passed: "info",
  warning: "warning",
  failed: "blocker",
};

function seedValidationRuns(state: FixtureState, proposal: { id: string; validations: ReadonlyArray<{ name: string; state: "passed" | "warning" | "failed"; detail: string }> }): GovernanceValidationRun[] {
  const byValidator = new Map<string, Array<{ name: string; state: "passed" | "warning" | "failed"; detail: string }>>();
  for (const validation of proposal.validations) {
    const validatorId = validatorForField[validation.name] ?? "structural";
    byValidator.set(validatorId, [...(byValidator.get(validatorId) ?? []), validation]);
  }
  return [...byValidator.entries()].map(([validatorId, findings]) => {
    state.counters.run += 1;
    return {
      id: `val_fixture_${String(state.counters.run).padStart(4, "0")}`,
      proposalId: proposal.id,
      validatorId,
      validatorVersion: "2026.08.4",
      status: "succeeded" as const,
      startedAt: FIXTURE_TIMESTAMP,
      finishedAt: FIXTURE_TIMESTAMP,
      results: findings.map((finding) => ({
        id: `vlr_fixture_${String(state.counters.run).padStart(4, "0")}_${finding.name.replace(/\s+/g, "").toLowerCase()}`,
        severity: severityForState[finding.state],
        code: `${validatorId.toUpperCase()}_COMPLETED`,
        message: finding.detail,
        inputDigest: digest(state.counters.run),
        createdAt: FIXTURE_TIMESTAMP,
      })),
    };
  });
}

function riskForProposal(proposal: { risk: string }): GovernancePolicyDecision {
  const levels: Record<string, { riskLevel: "low" | "medium" | "high"; routing: "expert" | "batch"; rule: string; reason: string; explanation: string }> = {
    高风险: { riskLevel: "high", routing: "expert", rule: "rule.g1.high_breakage", reason: "BREAKAGE_GUARDED", explanation: "变更涉及破坏性口径或别名移除，需要专家评审。", },
    中风险: { riskLevel: "medium", routing: "expert", rule: "rule.g1.computation_change", reason: "COMPUTATION_CHANGED", explanation: "计算表达式发生变化，进入专家评审通道。", },
    低风险: { riskLevel: "low", routing: "batch", rule: "rule.g1.low_evidence", reason: "EVIDENCE_ONLY", explanation: "低风险证据补充，进入批量确认通道。", },
  };
  const level = levels[proposal.risk] ?? levels["低风险"];
  return {
    ruleVersion: "2026.08.4",
    inputsDigest: digest(proposal.risk.length * 7 + 13),
    riskLevel: level.riskLevel,
    routing: level.routing,
    matchedRuleId: level.rule,
    reasonCode: level.reason,
    explanation: level.explanation,
    matchedInputFields: ["diff_category", "risk_signal"],
    createdAt: FIXTURE_TIMESTAMP,
  };
}

function seedProposalFromSession(state: FixtureState, session: (typeof sessionProposals)[number]): FixtureProposal {
  state.counters.proposal += 1;
  const asset = fixtureAssets.find((item) => item.id === session.assetId);
  const baseRevisionId = asset?.revisionRecord.revisionId ?? `${session.assetId}@1`;
  const detail: GovernanceProposalDetail = {
    id: session.id,
    targetObjectType: "semantic_asset",
    targetObjectId: session.assetId,
    assetId: session.assetId,
    baseRevisionId,
    state: "in_review",
    title: session.title,
    summary: session.summary,
    reason: session.summary,
    createdBy: session.author,
    submittedAt: FIXTURE_TIMESTAMP,
    createdAt: FIXTURE_TIMESTAMP,
    updatedAt: FIXTURE_TIMESTAMP,
    changeSet: session.changes.map((change) => {
      state.counters.change += 1;
      return {
        id: `chg_fixture_${String(state.counters.change).padStart(4, "0")}`,
        fieldPath: change.field,
        op: "update" as const,
        beforeValue: change.before,
        afterValue: change.after,
        beforeDigest: digest(state.counters.change * 2),
        afterDigest: digest(state.counters.change * 2 + 1),
        createdAt: FIXTURE_TIMESTAMP,
      };
    }),
  };
  const runs = seedValidationRuns(state, { id: session.id, validations: session.validations });
  return { detail, runs, decision: riskForProposal(session) };
}

const sessionProposals = [
  {
    id: "PROP-128",
    assetId: "metric_01J4AOV8C6MW3B7K9P2D",
    author: "Semlia AI",
    title: "统一客单价的退款订单处理口径",
    summary: "排除完全退款订单，并将分母绑定到支付订单数的已发布 revision。",
    risk: "中风险",
    changes: [
      { field: "definition", before: "净收入除以支付成功订单数。", after: "净收入除以支付成功且未完全退款的订单数。" },
      { field: "spec.expression", before: "net_revenue / orders.count", after: "net_revenue / paid_order_count" },
    ],
    validations: [
      { name: "Schema references", state: "passed" as const, detail: "2 个引用均解析到已发布资产" },
      { name: "Cube compile", state: "passed" as const, detail: "commerce 模型编译通过 · 4.8s" },
      { name: "Result regression", state: "warning" as const, detail: "西南区域变化 +1.8%，需要负责人确认" },
      { name: "Policy G1", state: "passed" as const, detail: "需要 1 名资产负责人审核" },
    ],
  },
  {
    id: "PROP-126",
    assetId: "metric_01J4NETREVENUE8W4Q9D7K2",
    author: "林悦",
    title: "为净收入补充财务口径证据",
    summary: "附加 FY2026 财务确认规则，不改变计算结果。",
    risk: "低风险",
    changes: [{ field: "evidence", before: "EVD-2038", after: "EVD-2038, EVD-2048" }],
    validations: [
      { name: "Schema references", state: "passed" as const, detail: "证据引用有效" },
      { name: "Policy G1", state: "passed" as const, detail: "低风险证据补充" },
    ],
  },
  {
    id: "PROP-121",
    assetId: "dimension_01J4REGION3X7Q8K2M5V",
    author: "Semlia AI",
    title: "废弃旧区域编码别名",
    summary: "删除 region_code_v1 别名，3 个消费者仍在使用。",
    risk: "高风险",
    changes: [{ field: "aliases", before: "region_code, region_code_v1", after: "region_code" }],
    validations: [
      { name: "Consumer impact", state: "failed" as const, detail: "3 个消费者尚未迁移" },
      { name: "Policy G1", state: "failed" as const, detail: "破坏性变更不允许直接发布" },
    ],
  },
] as const;

function createInitialReleases(state: FixtureState): GovernanceReleaseDetail[] {
  const usedIds = new Set<string>();
  return assetVersionReleases.map((release, index) => {
    state.counters.release += 1;
    const sequence = assetVersionReleases.length - index;
    const asset = fixtureAssets.find((item) => item.id === release.assetId);
    const sequenceMatch = release.revision.match(/@(\d+)/);
    const revisionSequence = sequenceMatch ? Number(sequenceMatch[1]) : asset?.revisionRecord.sequence ?? 1;
    let releaseId = release.id;
    while (usedIds.has(releaseId)) releaseId = `${release.id}-${sequence}`;
    usedIds.add(releaseId);
    return {
      id: releaseId,
      sequence,
      state: "published" as const,
      manifestDigest: digest(sequence * 11 + 5),
      originProposalId: undefined,
      publishedBy: release.publisher,
      publishedAt: FIXTURE_TIMESTAMP,
      createdAt: FIXTURE_TIMESTAMP,
      manifest: {
        assets: [{ assetId: release.assetId, revisionId: `${release.assetId}@${revisionSequence}`, compatibility: null, position: 0 }],
        objects: [],
      },
    };
  });
}

function createInitialProviders(state: FixtureState): GovernanceModelProviderDetail[] {
  const provider = (input: { kind: "llm" | "embedding"; protocol: "openai" | "anthropic" | "gemini" | "openai_compatible"; displayName: string; baseUrl?: string; credentialEnv: string; models: Array<{ model: string; capability: string; tokenLimit: number; isDefault: boolean; embeddingDimension?: number }> }): GovernanceModelProviderDetail => {
    state.counters.provider += 1;
    const providerId = `prv_fixture_${String(state.counters.provider).padStart(3, "0")}`;
    return {
      provider: {
        id: providerId,
        protocol: input.protocol,
        displayName: input.displayName,
        baseUrl: input.baseUrl,
        credentialEnv: input.credentialEnv,
        credentialRevision: digest(state.counters.provider * 17 + 3),
        enabled: true,
        createdAt: FIXTURE_TIMESTAMP,
        updatedAt: FIXTURE_TIMESTAMP,
      },
      models: input.models.map((model) => {
        state.counters.setting += 1;
        return {
          id: `mdl_fixture_${String(state.counters.setting).padStart(3, "0")}`,
          providerId,
          kind: input.kind,
          model: model.model,
          enabled: true,
          isDefault: model.isDefault,
          capability: model.capability,
          tokenLimit: model.tokenLimit,
          embeddingDimension: model.embeddingDimension,
          createdAt: FIXTURE_TIMESTAMP,
          updatedAt: FIXTURE_TIMESTAMP,
        };
      }),
    };
  };
  return [
    provider({ kind: "llm", protocol: "openai", displayName: "OpenAI Enterprise", credentialEnv: "SEMLIA_OPENAI_API_KEY", models: [
      { model: "gpt-4.1", capability: "视觉 · 推理", tokenLimit: 128000, isDefault: true },
      { model: "o3", capability: "推理", tokenLimit: 200000, isDefault: false },
    ] }),
    provider({ kind: "llm", protocol: "anthropic", displayName: "Anthropic", credentialEnv: "SEMLIA_ANTHROPIC_API_KEY", models: [
      { model: "claude-sonnet-4-20250514", capability: "视觉 · 推理", tokenLimit: 200000, isDefault: false },
    ] }),
    provider({ kind: "embedding", protocol: "openai", displayName: "OpenAI Enterprise Embedding", credentialEnv: "SEMLIA_EMBEDDING_API_KEY", models: [
      { model: "text-embedding-3-large", capability: "多语言检索", tokenLimit: 8191, isDefault: true, embeddingDimension: 3072 },
    ] }),
    provider({ kind: "embedding", protocol: "openai_compatible", displayName: "内部向量服务", baseUrl: "https://models.example.internal/v1", credentialEnv: "SEMLIA_INTERNAL_EMBEDDING_KEY", models: [
      { model: "bge-m3", capability: "多语言 · 稠密检索", tokenLimit: 8192, isDefault: false, embeddingDimension: 1024 },
    ] }),
  ];
}

function createInitialState(): FixtureState {
  const state: FixtureState = {
    proposals: new Map(),
    reviews: [],
    batches: [],
    releases: [],
    providers: [],
    counters: { proposal: 0, change: 0, review: 0, batch: 0, release: 0, run: 0, provider: 0, setting: 0 },
  };
  for (const session of sessionProposals) {
    const proposal = seedProposalFromSession(state, session);
    state.proposals.set(session.id, proposal);
  }
  state.releases = createInitialReleases(state);
  state.providers = createInitialProviders(state);
  return state;
}

export interface GovernanceFixtureClient {
  listProposals: () => Promise<GovernanceProposalSummary[]>;
  createProposal: (input: CreateGovernanceProposalRequest) => Promise<GovernanceProposalDetail>;
  getProposal: (proposalId: string) => Promise<GovernanceProposalDetail>;
  submitProposal: (proposalId: string) => Promise<GovernanceProposalDetail>;
  listValidationRuns: (proposalId: string) => Promise<GovernanceValidationRun[]>;
  getPolicyDecision: (proposalId: string) => Promise<GovernancePolicyDecision | null>;
  createReview: (proposalId: string, input: GovernanceReviewCommandRequest) => Promise<GovernanceReview>;
  listReviewBatches: () => Promise<GovernanceReviewBatchDetail[]>;
  assembleReviewBatches: () => Promise<GovernanceReviewBatchDetail[]>;
  getReviewBatch: (batchId: string) => Promise<GovernanceReviewBatchDetail>;
  confirmReviewBatch: (batchId: string, input: GovernanceReviewCommandRequest) => Promise<GovernanceReviewBatchDetail>;
  listReleases: () => Promise<GovernanceReleaseDetail[]>;
  publishRelease: (proposalId: string) => Promise<GovernanceReleaseDetail>;
  getRelease: (releaseId: string) => Promise<GovernanceReleaseDetail>;
  rollbackRelease: (releaseId: string) => Promise<GovernanceReleaseDetail>;
  listModelProviders: () => Promise<GovernanceModelProviderDetail[]>;
  createModelProvider: (input: CreateGovernanceModelProviderRequest) => Promise<GovernanceModelProviderDetail>;
  updateModelProvider: (providerId: string, input: UpdateGovernanceModelProviderRequest) => Promise<GovernanceModelProviderDetail>;
  createModelSetting: (input: CreateGovernanceModelSettingRequest) => Promise<GovernanceModelSetting>;
  updateModelSetting: (settingId: string, input: UpdateGovernanceModelSettingRequest) => Promise<GovernanceModelSetting>;
  setDefaultModelSetting: (settingId: string) => Promise<GovernanceModelSetting>;
  generateProposal: (input: GovernanceGenerateProposalRequest) => Promise<GovernanceGeneratedProposal>;
}

export function createGovernanceFixture(): GovernanceFixtureClient {
  const state = createInitialState();

  const requireProposal = (proposalId: string): FixtureProposal => {
    const proposal = state.proposals.get(proposalId);
    if (!proposal) throw apiError("NOT_FOUND", `提案 ${proposalId} 不存在。`);
    return proposal;
  };

  const touch = (proposal: FixtureProposal, patch: Partial<GovernanceProposalDetail>) => {
    proposal.detail = { ...proposal.detail, ...patch, updatedAt: new Date().toISOString() };
  };

  const runsForNewChangeSet = (proposalId: string, changeSet: GovernanceProposalDetail["changeSet"]): GovernanceValidationRun[] => {
    const expressionChange = changeSet.some((item) => item.fieldPath.includes("spec.expression"));
    const breakingChange = changeSet.some((item) => item.fieldPath.includes("aliases"));
    const findings: Array<{ name: string; state: "passed" | "warning" | "failed"; detail: string }> = [
      { name: "Schema references", state: "passed", detail: "变更字段引用均解析到已发布来源 revision" },
      { name: "Policy G1", state: "passed", detail: "修订原因与审核范围已记录" },
      expressionChange
        ? { name: "Result regression", state: "warning", detail: "计算表达式变化，回归样本需要负责人确认" }
        : { name: "Result regression", state: "passed", detail: "没有计算结果变化" },
    ];
    if (breakingChange) findings.push({ name: "Consumer impact", state: "failed", detail: "存在尚未迁移的消费者" });
    return seedValidationRuns(state, { id: proposalId, validations: findings });
  };

  const decisionForChangeSet = (changeSet: GovernanceProposalDetail["changeSet"]): GovernancePolicyDecision => {
    const breaking = changeSet.some((item) => item.fieldPath.includes("aliases"));
    const expression = changeSet.some((item) => item.fieldPath.includes("spec.expression"));
    if (breaking) return riskForProposal({ risk: "高风险" });
    if (expression) return riskForProposal({ risk: "中风险" });
    return riskForProposal({ risk: "低风险" });
  };

  return {
    async listProposals() {
      return [...state.proposals.values()].map((proposal) => proposal.detail);
    },

    async createProposal(input) {
      if (!input.changeSet.length) throw apiError("NO_SUBSTANTIVE_CHANGE", "变更集合没有实质差异。");
      state.counters.proposal += 1;
      const proposalId = `prp_fixture_${String(state.counters.proposal).padStart(4, "0")}`;
      const changeSet = input.changeSet.map((item) => {
        state.counters.change += 1;
        return {
          id: `chg_fixture_${String(state.counters.change).padStart(4, "0")}`,
          fieldPath: item.fieldPath,
          op: item.op,
          beforeValue: item.beforeValue,
          afterValue: item.afterValue,
          beforeDigest: item.beforeDigest ?? digest(state.counters.change * 2),
          afterDigest: item.afterDigest ?? digest(state.counters.change * 2 + 1),
          createdAt: FIXTURE_TIMESTAMP,
        };
      });
      const proposal: FixtureProposal = {
        detail: {
          id: proposalId,
          targetObjectType: input.targetObjectType,
          targetObjectId: input.targetObjectId,
          assetId: input.targetObjectType === "semantic_asset" ? input.targetObjectId : undefined,
          baseRevisionId: input.baseRevisionId,
          state: "draft",
          title: input.title,
          summary: input.summary ?? "",
          reason: input.reason ?? "",
          agentRunId: input.agentAttribution?.agentRunId,
          createdBy: input.createdBy ?? "catalog-web",
          createdAt: FIXTURE_TIMESTAMP,
          updatedAt: FIXTURE_TIMESTAMP,
          changeSet,
        },
        runs: [],
        decision: null,
      };
      state.proposals.set(proposalId, proposal);
      return proposal.detail;
    },

    async getProposal(proposalId) {
      return requireProposal(proposalId).detail;
    },

    async submitProposal(proposalId) {
      const proposal = requireProposal(proposalId);
      if (proposal.detail.state !== "draft") throw apiError("CONFLICT", "只有草稿状态的提案可以提交。");
      proposal.runs = runsForNewChangeSet(proposalId, proposal.detail.changeSet);
      proposal.decision = decisionForChangeSet(proposal.detail.changeSet);
      touch(proposal, { state: "in_review", submittedAt: FIXTURE_TIMESTAMP });
      return proposal.detail;
    },

    async listValidationRuns(proposalId) {
      return requireProposal(proposalId).runs;
    },

    async getPolicyDecision(proposalId) {
      return requireProposal(proposalId).decision;
    },

    async createReview(proposalId, input) {
      const proposal = requireProposal(proposalId);
      if (proposal.detail.state !== "in_review") throw apiError("CONFLICT", "只有审核中的提案可以记录评审。");
      if (!input.reason.trim()) throw apiError("INVALID_ARGUMENT", "评审意见为必填项。");
      state.counters.review += 1;
      const review: GovernanceReview = {
        id: `rvw_fixture_${String(state.counters.review).padStart(4, "0")}`,
        proposalId,
        reviewerPrincipalId: "prn_fixture_reviewer",
        channel: "expert",
        decision: input.decision === "approve" ? "approved" : "rejected",
        note: input.reason,
        createdAt: FIXTURE_TIMESTAMP,
      };
      state.reviews.push(review);
      if (input.decision === "reject") touch(proposal, { state: "rejected", decidedAt: FIXTURE_TIMESTAMP });
      return review;
    },

    async listReviewBatches() {
      return state.batches.filter((batch) => batch.status === "open");
    },

    async assembleReviewBatches() {
      const eligible = [...state.proposals.values()].filter((proposal) =>
        proposal.detail.state === "in_review" && proposal.decision?.routing === "batch" && !state.batches.some((batch) => batch.members.some((member) => member.proposalId === proposal.detail.id && !member.splitOut)));
      if (eligible.length === 0) return [];
      state.counters.batch += 1;
      const batch: GovernanceReviewBatchDetail = {
        id: `rvb_fixture_${String(state.counters.batch).padStart(4, "0")}`,
        status: "open",
        groupingRule: { targetObjectType: "semantic_asset", diffCategory: "definition", matchedRuleId: eligible[0].decision?.matchedRuleId ?? "rule.g1.low_evidence" },
        policyVersion: "2026.08.4",
        memberCount: eligible.length,
        createdBy: "catalog-web",
        createdAt: FIXTURE_TIMESTAMP,
        maxRiskProposalId: eligible[0].detail.id,
        members: eligible.map((proposal) => ({
          proposalId: proposal.detail.id,
          addedReason: {
            matchedRuleId: proposal.decision?.matchedRuleId ?? "rule.g1.low_evidence",
            riskLevel: proposal.decision?.riskLevel ?? "low",
            reasonCode: proposal.decision?.reasonCode ?? "EVIDENCE_ONLY",
            ruleVersion: proposal.decision?.ruleVersion ?? "2026.08.4",
            inputsDigest: proposal.decision?.inputsDigest ?? digest(7),
          },
          sample: true,
          splitOut: false,
          createdAt: FIXTURE_TIMESTAMP,
        })),
        samples: [],
        exclusions: [],
      };
      batch.samples = batch.members.filter((member) => member.sample);
      state.batches.push(batch);
      return [batch];
    },

    async getReviewBatch(batchId) {
      const batch = state.batches.find((item) => item.id === batchId);
      if (!batch) throw apiError("NOT_FOUND", `评审批次 ${batchId} 不存在。`);
      return batch;
    },

    async confirmReviewBatch(batchId, input) {
      const batch = state.batches.find((item) => item.id === batchId);
      if (!batch) throw apiError("NOT_FOUND", `评审批次 ${batchId} 不存在。`);
      if (batch.status !== "open") throw apiError("CONFLICT", "评审批次已经确认。");
      if (!input.reason.trim()) throw apiError("INVALID_ARGUMENT", "评审意见为必填项。");
      for (const member of batch.members.filter((item) => !item.splitOut)) {
        state.counters.review += 1;
        const proposal = state.proposals.get(member.proposalId);
        state.reviews.push({
          id: `rvw_fixture_${String(state.counters.review).padStart(4, "0")}`,
          proposalId: member.proposalId,
          reviewerPrincipalId: "prn_fixture_reviewer",
          channel: "batch",
          decision: input.decision === "approve" ? "approved" : "rejected",
          note: input.reason,
          createdAt: FIXTURE_TIMESTAMP,
        });
        if (input.decision === "reject" && proposal) touch(proposal, { state: "rejected", decidedAt: FIXTURE_TIMESTAMP });
      }
      batch.status = input.decision === "approve" ? "confirmed" : "rejected";
      batch.decidedBy = "prn_fixture_reviewer";
      batch.decidedAt = FIXTURE_TIMESTAMP;
      return batch;
    },

    async listReleases() {
      return [...state.releases].sort((left, right) => right.sequence - left.sequence);
    },

    async publishRelease(proposalId) {
      const proposal = requireProposal(proposalId);
      if (proposal.detail.state !== "in_review") throw apiError("PROPOSAL_NOT_IN_REVIEW", "只有审核中的提案可以发布。");
      const approved = state.reviews.some((review) => review.proposalId === proposalId && review.decision === "approved");
      if (!approved) throw apiError("NO_APPROVING_REVIEW", "提案至少需要一个批准评审才能发布。");
      const nextSequence = Math.max(0, ...state.releases.map((release) => release.sequence)) + 1;
      state.counters.release += 1;
      const asset = fixtureAssets.find((item) => item.id === proposal.detail.targetObjectId);
      const nextRevisionSequence = (asset?.revisionRecord.sequence ?? 0) + 1;
      const release: GovernanceReleaseDetail = {
        id: `rls_fixture_${String(state.counters.release).padStart(4, "0")}`,
        sequence: nextSequence,
        state: "published",
        manifestDigest: digest(nextSequence * 13 + 7),
        originProposalId: proposalId,
        publishedBy: "catalog-web",
        publishedAt: FIXTURE_TIMESTAMP,
        createdAt: FIXTURE_TIMESTAMP,
        manifest: {
          assets: [{ assetId: proposal.detail.targetObjectId, revisionId: `${proposal.detail.targetObjectId}@${nextRevisionSequence}`, compatibility: null, position: 0 }],
          objects: [],
        },
      };
      state.releases.push(release);
      touch(proposal, { state: "released", decidedAt: FIXTURE_TIMESTAMP });
      return release;
    },

    async getRelease(releaseId) {
      const release = state.releases.find((item) => item.id === releaseId);
      if (!release) throw apiError("NOT_FOUND", `发布记录 ${releaseId} 不存在。`);
      return release;
    },

    async rollbackRelease(releaseId) {
      const target = state.releases.find((item) => item.id === releaseId);
      if (!target) throw apiError("NOT_FOUND", `发布记录 ${releaseId} 不存在。`);
      const latest = Math.max(...state.releases.map((release) => release.sequence));
      if (target.sequence !== latest) throw apiError("RELEASE_NOT_LATEST", "只能回滚最新的发布记录。");
      if (state.releases.some((release) => release.rolledBackToReleaseId === releaseId)) throw apiError("RELEASE_ALREADY_ROLLED_BACK", "该发布记录已经回滚过一次。");
      const nextSequence = latest + 1;
      state.counters.release += 1;
      const manifestAssets = target.manifest.assets.map((asset) => {
        const priorSequence = Math.max(1, revisionSequenceOf(asset.revisionId) - 1);
        return { ...asset, revisionId: `${asset.assetId}@${priorSequence}` };
      });
      const release: GovernanceReleaseDetail = {
        id: `rls_fixture_${String(state.counters.release).padStart(4, "0")}`,
        sequence: nextSequence,
        state: "published",
        manifestDigest: digest(nextSequence * 13 + 7),
        rolledBackToReleaseId: releaseId,
        publishedBy: "catalog-web",
        publishedAt: FIXTURE_TIMESTAMP,
        createdAt: FIXTURE_TIMESTAMP,
        manifest: { assets: manifestAssets, objects: [] },
      };
      state.releases.push(release);
      return release;
    },

    async listModelProviders() {
      return state.providers;
    },

    async createModelProvider(input) {
      if (!/^[A-Z][A-Z0-9_]{0,63}$/.test(input.credentialEnv)) throw apiError("INVALID_ARGUMENT", "凭据环境变量名不符合命名规则。");
      state.counters.provider += 1;
      const providerId = `prv_fixture_${String(state.counters.provider).padStart(3, "0")}`;
      const detail: GovernanceModelProviderDetail = {
        provider: {
          id: providerId,
          protocol: input.protocol,
          displayName: input.displayName,
          baseUrl: input.baseUrl,
          credentialEnv: input.credentialEnv,
          credentialRevision: digest(state.counters.provider * 17 + 3),
          enabled: true,
          createdAt: FIXTURE_TIMESTAMP,
          updatedAt: FIXTURE_TIMESTAMP,
        },
        models: [],
      };
      state.providers.push(detail);
      return detail;
    },

    async updateModelProvider(providerId, input) {
      const found = state.providers.find((item) => item.provider.id === providerId);
      if (!found) throw apiError("NOT_FOUND", `模型供应商 ${providerId} 不存在。`);
      const rotationDigest = input.credential ? digest((state.counters.provider += 1) * 23 + 5) : found.provider.credentialRevision;
      found.provider = {
        ...found.provider,
        displayName: input.displayName,
        baseUrl: input.baseUrl,
        enabled: input.enabled,
        credentialEnv: input.credentialEnv ?? found.provider.credentialEnv,
        credentialRevision: rotationDigest,
        updatedAt: FIXTURE_TIMESTAMP,
      };
      return found;
    },

    async createModelSetting(input) {
      const provider = state.providers.find((item) => item.provider.id === input.providerId);
      if (!provider) throw apiError("NOT_FOUND", `模型供应商 ${input.providerId} 不存在。`);
      if (!provider.provider.enabled) throw apiError("CONFLICT", "供应商已停用，不能添加模型。");
      state.counters.setting += 1;
      const kindOf: GovernanceModelSetting["kind"] = input.kind;
      const sameKind = state.providers.flatMap((item) => item.models).filter((model) => model.kind === kindOf && model.enabled);
      const setting: GovernanceModelSetting = {
        id: `mdl_fixture_${String(state.counters.setting).padStart(3, "0")}`,
        providerId: input.providerId,
        kind: kindOf,
        model: input.model,
        enabled: true,
        isDefault: sameKind.length === 0,
        capability: input.capability,
        tokenLimit: input.tokenLimit,
        embeddingDimension: input.embeddingDimension,
        createdAt: FIXTURE_TIMESTAMP,
        updatedAt: FIXTURE_TIMESTAMP,
      };
      provider.models.push(setting);
      return setting;
    },

    async updateModelSetting(settingId, input) {
      const setting = state.providers.flatMap((item) => item.models).find((model) => model.id === settingId);
      if (!setting) throw apiError("NOT_FOUND", `模型条目 ${settingId} 不存在。`);
      Object.assign(setting, { ...input, updatedAt: FIXTURE_TIMESTAMP });
      return setting;
    },

    async setDefaultModelSetting(settingId) {
      const setting = state.providers.flatMap((item) => item.models).find((model) => model.id === settingId);
      if (!setting) throw apiError("NOT_FOUND", `模型条目 ${settingId} 不存在。`);
      if (!setting.enabled) throw apiError("CONFLICT", "已停用的模型不能设为默认。");
      for (const provider of state.providers) {
        for (const model of provider.models) {
          if (model.kind === setting.kind) model.isDefault = model.id === settingId;
        }
      }
      setting.updatedAt = FIXTURE_TIMESTAMP;
      return setting;
    },

    async generateProposal(input: GovernanceGenerateProposalRequest): Promise<GovernanceGeneratedProposal> {
      const defaultLlm = state.providers
        .filter((provider) => provider.provider.enabled)
        .flatMap((provider) => provider.models)
        .find((model) => model.kind === "llm" && model.enabled && model.isDefault);
      if (!defaultLlm) throw apiError("CONFLICT", "工作区尚未配置可用的默认 LLM 模型。");
      const asset = fixtureAssets.find((item) => item.id === input.targetObjectId);
      const draft = await this.createProposal({
        targetObjectType: input.targetObjectType,
        targetObjectId: input.targetObjectId,
        baseRevisionId: asset?.revisionRecord.revisionId,
        title: `AI 草案：${asset?.name ?? input.targetObjectId}`,
        summary: input.instruction,
        reason: input.instruction,
        changeSet: [{ fieldPath: "definition.boundary", op: "update", beforeValue: asset?.revisionRecord.definition ?? "", afterValue: input.instruction }],
        createdBy: "semlia-agent",
      });
      const agentRun = {
        id: `agr_fixture_${String(state.counters.proposal).padStart(4, "0")}`,
        model: defaultLlm.model,
        configRevision: `${digest(5)}/${defaultLlm.id}`,
        inputHash: digest(state.counters.proposal * 29 + 3),
        status: "succeeded" as const,
        outputDigest: digest(state.counters.proposal * 31 + 9),
        costMicros: 320,
        startedAt: FIXTURE_TIMESTAMP,
        finishedAt: FIXTURE_TIMESTAMP,
        durationMs: 1840,
      };
      draft.agentRunId = agentRun.id;
      return { agentRun, proposal: draft };
    },
  };
}
