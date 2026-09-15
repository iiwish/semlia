import type { components } from "@semlia/sdk-typescript";

export type ViewId = "ask" | "sources" | "overview" | "assets" | "releases" | "settings";
export type NavigateToView = (view: ViewId, contextIndex?: number) => void;

export type AssetType = "业务概念" | "业务实体" | "语义模型" | "维度" | "度量" | "指标" | "分群";
export type AssetStatus = "已发布" | "需关注" | "草稿" | "待确认";
export type ReadinessState = "passed" | "warning" | "not_applicable";
export type EvidenceAuthority = "DECLARED" | "CONSTRAINED" | "DERIVED" | "OBSERVED" | "INFERRED";

export type AssetLifecycleState = "active" | "deprecated" | "retired" | "draft" | "archived";
export type AssetWorkflowState = "draft" | "proposed" | "in_review" | "released" | "unknown";
export type DeploymentState = "unreleased" | "staging" | "production" | "unknown";
export type AssetHealthState = "healthy" | "warning" | "blocked";
export type CompatibilityState = "compatible" | "conditional" | "breaking" | "not_evaluated";
export type OntologyRelationPlane = "taxonomy" | "semantic" | "dependency";
export type OntologyAssertionState = "asserted" | "inferred" | "candidate" | "deprecated";
export type AssetAuthorityAvailability = components["schemas"]["CatalogAuthorityAvailability"];
export type AssetAuthoritySection = components["schemas"]["CatalogAuthoritySection"];
export type AssetAuthoritySectionKind = AssetAuthoritySection["kind"];

export interface SemanticAssetIdentity {
  workspaceId: string;
  assetId: string;
  namespace: string;
  key: string;
  type: AssetType;
  domain: string;
  lifecycleState: AssetLifecycleState;
}

interface BaseAssetTypeSpec {
  expression: string;
  grain: string;
  defaultTime: string;
  unit: string;
  aggregation: string;
}

export interface MetricSpec extends BaseAssetTypeSpec {
  kind: "metric";
  metricKind: "simple" | "ratio" | "derived" | "cumulative" | "conversion" | "undeclared";
  allowedDimensions: string[];
  comparisonSemantics: string;
}

export interface MeasureSpec extends BaseAssetTypeSpec {
  kind: "measure";
  additivity: "additive" | "semi_additive" | "non_additive" | "undeclared";
  nullHandling: string;
}

export interface DimensionSpec extends BaseAssetTypeSpec {
  kind: "dimension";
  valueType: string;
  nullSemantics: string;
  hierarchy: string[];
}

export interface EntitySpec extends BaseAssetTypeSpec {
  kind: "entity";
  entityKeys: string[];
  identityPolicy: string;
  lifecycle: string;
}

export interface SemanticModelSpec extends BaseAssetTypeSpec {
  kind: "model";
  primaryEntity: string;
  publicMembers: string[];
  joinPathPolicy: string;
}

export interface SegmentSpec extends BaseAssetTypeSpec {
  kind: "segment";
  baseEntity: string;
  refreshPolicy: string;
  effectiveTime: string;
}

export interface ConceptSpec extends BaseAssetTypeSpec {
  kind: "concept";
  conceptClass: string;
  disambiguationRule: string;
}

export type AssetTypeSpec = MetricSpec | MeasureSpec | DimensionSpec | EntitySpec | SemanticModelSpec | SegmentSpec | ConceptSpec;

export interface AssetRevisionRecord {
  revisionId: string;
  assetId: string;
  sequence: number;
  schemaVersion: string;
  contentHash: string;
  recordedAt: string;
  validFrom: string;
  workflowState: AssetWorkflowState;
  name: string;
  aliases: string[];
  definition: string;
  includes: string[];
  excludes: string[];
  examples: string[];
  typeSpec: AssetTypeSpec;
}

export interface EnvironmentDeployment {
  environment: "production" | "staging" | "unknown";
  state: DeploymentState;
  releaseId?: string;
  revisionId?: string;
  deployedAt?: string;
}

export interface QualitySnapshot {
  snapshotId: string;
  state: AssetHealthState;
  policyVersion: string;
  evaluatedAt: string;
  blockingCount: number;
  warningCount: number;
}

export interface SemanticClaim {
  claimId: string;
  assetRevisionId: string;
  fieldPath: string;
  label: string;
  value: string;
}

export interface KnowledgeRevisionRequest {
  assetId: string;
  fieldPath: string;
  claimId?: string;
  origin: "ask" | "definition" | "evidence";
  context?: string;
}

export interface KnowledgeRevisionSubmission {
  assetId: string;
  baseRevision: string;
  title: string;
  summary: string;
  reason: string;
  changes: Proposal["changes"];
}

export interface EvidenceArtifact {
  evidenceId: string;
  sourceRevision: string;
  authority: EvidenceAuthority;
  observedAt: string;
  scope: string;
  digest: string;
}

export interface ClaimEvidenceLink {
  claimId: string;
  evidenceId: string;
  polarity: "supports" | "contradicts";
  strength: "primary" | "corroborating";
}

export interface ValidationRunRecord {
  runId: string;
  validationSpecId: string;
  validatorVersion: string;
  assetRevisionId: string;
  executedAt: string;
  state: "passed" | "warning" | "failed";
  reproducible: boolean;
}

export interface ConsumerBindingRecord {
  bindingId: string;
  consumerId: string;
  assetId: string;
  environment: "production" | "staging";
  versionConstraint: string;
  resolvedRevisionId: string;
  compatibility: CompatibilityState;
}

export interface LLMWikiContext {
  contextId: string;
  assetRevisionId: string;
  canonicalSummary: string;
  retrievalTerms: string[];
  disambiguationRules: string[];
  groundedQuestions: string[];
  compiledAt: string;
}

export interface OntologyRelationConstraint {
  relationType: AssetRelation["type"];
  label: string;
  inverseLabel: string;
  sourceTypes: AssetType[];
  targetTypes: AssetType[];
  cardinality: "one_to_one" | "one_to_many" | "many_to_one" | "many_to_many";
  reasoning: "directed" | "symmetric";
  validationState: "valid" | "warning";
  validationDetail: string;
}

export interface OntologyContext {
  ontologyId: string;
  revisionId: string;
  previousRevisionId?: string;
  domainPath: string[];
  assetId: string;
  parentConcepts: string[];
  relatedConcepts: string[];
  relationConstraints: OntologyRelationConstraint[];
  consistencyState: "consistent" | "warning";
  consistencyIssues: string[];
  revisionDelta: {
    addedRelations: number;
    removedRelations: number;
    changedConstraints: number;
  };
  publishedIn?: string;
}

export interface Evidence {
  id: string;
  kind: string;
  label: string;
  source: string;
  verifiedAt: string;
  authority: EvidenceAuthority;
  supports: string;
  state: "已验证" | "需确认";
}

export interface Consumer {
  name: string;
  kind: string;
  binding: string;
  lastResolved: string;
}

export interface AssetReadinessGate {
  id: "identity" | "ownership" | "definition" | "relations" | "mapping" | "evidence" | "validation" | "compatibility";
  label: string;
  state: ReadinessState;
  detail: string;
}

export interface AssetRelation {
  id: string;
  type: "measures" | "describes" | "depends_on" | "derived_from" | "filters_by" | "synonym_of" | "contains" | "broader_than" | "narrower_than" | "equivalent_to" | "disjoint_with";
  plane: OntologyRelationPlane;
  assertionState: OntologyAssertionState;
  targetId: string;
  targetName: string;
  direction: "outgoing" | "incoming";
  evidence: string;
  release: string;
}

export interface PhysicalBinding {
  id: string;
  sourceRevision: string;
  dataset: string;
  expression: string;
  grain: string;
  defaultTime: string;
  adapter: string;
  state: "已验证" | "需重验";
}

export interface JoinContract {
  id: string;
  target: string;
  keys: string;
  cardinality: "one_to_one" | "many_to_one" | "one_to_many";
  grainImpact: string;
  guardrail: string;
  state: "已发布" | "需关注";
}

export interface Asset {
  // This is a denormalized detail projection. Canonical identity, revision,
  // deployment and quality records remain independently addressable.
  id: string;
  detailLoaded?: boolean;
  authoritySections?: AssetAuthoritySection[];
  revision: string;
  namespace: string;
  key: string;
  name: string;
  aliases: string[];
  type: AssetType;
  status: AssetStatus;
  owner: string;
  maintainer: string;
  domain: string;
  definition: string;
  formula: string;
  grain: string;
  defaultTime: string;
  unit: string;
  aggregation: string;
  source: string;
  updatedAt: string;
  release: string;
  tags: string[];
  includes: string[];
  excludes: string[];
  examples: string[];
  readiness: AssetReadinessGate[];
  relations: AssetRelation[];
  bindings: PhysicalBinding[];
  joinContracts: JoinContract[];
  validations: Validation[];
  evidence: Evidence[];
  upstream: string[];
  downstream: string[];
  consumers: Consumer[];
  identity: SemanticAssetIdentity;
  revisionRecord: AssetRevisionRecord;
  deployment: EnvironmentDeployment;
  qualitySnapshot: QualitySnapshot;
  claims: SemanticClaim[];
  evidenceArtifacts: EvidenceArtifact[];
  evidenceLinks: ClaimEvidenceLink[];
  validationRuns: ValidationRunRecord[];
  consumerBindings: ConsumerBindingRecord[];
  wikiContext: LLMWikiContext;
  ontologyContext: OntologyContext;
}

export type ValidationState = "passed" | "warning" | "failed";

export interface Validation {
  name: string;
  state: ValidationState;
  detail: string;
}

export interface Proposal {
  id: string;
  title: string;
  assetName: string;
  assetId: string;
  author: string;
  createdAt: string;
  risk: "低风险" | "中风险" | "高风险" | "待评估";
  summary: string;
  changes: Array<{
    field: string;
    before: string;
    after: string;
  }>;
  validations: Validation[];
  impact: string[];
}

export interface AssetVersionRelease {
  id: string;
  assetId: string;
  assetKey: string;
  assetName: string;
  assetType: AssetType;
  revision: string;
  previousRevision: string;
  state: "Stable" | "Superseded";
  publishedAt: string;
  publisher: string;
  summary: string;
  changes: string[];
  consumers: Consumer[];
}

export type PermissionAction =
  | "workspace.read"
  | "workspace.manage"
  | "member.read"
  | "member.manage"
  | "group.manage"
  | "role.read"
  | "role.manage"
  | "role.assign"
  | "authorization.inspect"
  | "asset.read"
  | "asset.propose"
  | "asset.edit"
  | "evidence.read"
  | "proposal.review"
  | "validation.run"
  | "release.publish"
  | "release.rollback"
  | "source.read"
  | "source.manage"
  | "ingestion.run"
  | "binding.read"
  | "binding.manage"
  | "semantic.resolve"
  | "semantic.execute"
  | "audit.read"
  | "runtime.read"
  | "runtime.manage";

export type AuthorizationPrincipalKind = "user" | "group" | "service_account" | "api_client" | "agent";

export type AuthorizationScopeType = "workspace" | "domain" | "asset" | "source" | "environment" | "release" | "consumer";

export interface AuthorizationScope {
  type: AuthorizationScopeType;
  id: string;
  label: string;
  domainId?: string;
  protected?: boolean;
}

export interface AuthorizationResource {
  type: AuthorizationScopeType;
  id: string;
  domainId?: string;
  environment?: string;
  protected?: boolean;
}

export interface AuthorizationPrincipal {
  id: string;
  kind: AuthorizationPrincipalKind;
  name: string;
  detail: string;
  status: "active" | "suspended" | "revoked";
}

export interface AuthorizationRole {
  id: string;
  name: string;
  description: string;
  category: "system" | "custom";
  permissions: PermissionAction[];
  version?: number;
  workspaceId?: string;
  createdAt?: string;
  baseRoleId?: string;
  incompatibleRoleIds?: string[];
}

export interface AuthorizationBinding {
  id: string;
  workspaceId?: string;
  principalId: string;
  roleId: string;
  roleVersion?: number;
  scope: AuthorizationScope;
  assignedBy: string;
  assignedAt: string;
  expiresAt?: string;
  expiredAt?: string;
  revokedAt?: string;
  revokedBy?: string;
  revocationReason?: string;
  status: "active" | "expired" | "revoked";
  version?: number;
}

export interface CapabilitySession {
  principalId: string;
  version: string;
  capabilities: PermissionAction[];
}

export type AuthorizationReasonCode =
  | "ROLE_GRANT"
  | "SESSION_CAPABILITY"
  | "NO_MATCHING_GRANT"
  | "PRINCIPAL_INACTIVE"
  | "SEPARATION_OF_DUTY";

export interface AuthorizationDecision {
  allowed: boolean;
  action: PermissionAction;
  principalId: string;
  reasonCode: AuthorizationReasonCode;
  explanation: string;
  authorizationVersion: string;
  roleId?: string;
  bindingId?: string;
  scope?: AuthorizationScope;
}

export interface SeparationOfDutyConflict {
  code: "PROTECTED_REVIEW_PUBLISH_CONFLICT";
  message: string;
  blocking: boolean;
  conflictingBindingId: string;
}

export interface PermissionGroup {
  id: string;
  label: string;
  description: string;
  actions: PermissionAction[];
}
