import type { GovernanceApi } from "../governanceRuntime";
import { createGovernanceFixture, type GovernanceFixtureClient } from "./governanceFixture";

export function createGovernanceFixtureApi(inner: GovernanceFixtureClient = createGovernanceFixture()): GovernanceApi {
  return {
    listProposals: () => inner.listProposals(),
    createProposal: (_workspaceId, input) => inner.createProposal(input),
    getProposal: (_workspaceId, proposalId) => inner.getProposal(proposalId),
    submitProposal: (_workspaceId, proposalId) => inner.submitProposal(proposalId),
    listValidationRuns: (_workspaceId, proposalId) => inner.listValidationRuns(proposalId),
    getPolicyDecision: (_workspaceId, proposalId) => inner.getPolicyDecision(proposalId),
    createReview: (_workspaceId, proposalId, input) => inner.createReview(proposalId, input),
    listReviews: (_workspaceId, proposalId) => inner.listReviews(proposalId),
    listReviewBatches: () => inner.listReviewBatches(),
    assembleReviewBatches: () => inner.assembleReviewBatches(),
    getReviewBatch: (_workspaceId, batchId) => inner.getReviewBatch(batchId),
    confirmReviewBatch: (_workspaceId, batchId, input) => inner.confirmReviewBatch(batchId, input),
    listReleases: async () => {
      const items = await inner.listReleases();
      return { items, page: { limit: items.length, total: items.length } };
    },
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
