const typeIdBrand: unique symbol = Symbol("SemliaTypeId");

export type ResourcePrefix =
  | "wsp"
  | "ast"
  | "rev"
  | "rel"
  | "ont"
  | "evd"
  | "rls"
  | "run"
  | "evt"
  | "src"
  | "srv"
  | "ssnp"
  | "codrev"
  | "linrev"
  | "prodop"
  | "pds"
  | "pdr"
  | "pfd"
  | "pfr"
  | "cod"
  | "lin"
  | "csm"
  | "cbd"
  | "smq"
  | "rsp"
  | "qvr"
  | "ati"
  | "art"
  | "ars"
  | "sch"
  | "occ"
  | "ccd"
  | "whs"
  | "whd";
export type TypeId<Prefix extends ResourcePrefix> = string & { readonly [typeIdBrand]: Prefix };

export type WorkspaceId = TypeId<"wsp">;
export type SemanticAssetId = TypeId<"ast">;
export type AssetRevisionId = TypeId<"rev">;
export type SemanticRelationId = TypeId<"rel">;
export type OntologyRevisionId = TypeId<"ont">;
export type EvidenceArtifactId = TypeId<"evd">;
export type ReleaseId = TypeId<"rls">;
export type RunId = TypeId<"run">;
export type EventId = TypeId<"evt">;
export type SourceConnectionId = TypeId<"src">;
export type SourceRevisionId = TypeId<"srv">;
export type SourceSnapshotId = TypeId<"ssnp">;
export type SourceCodeRevisionId = TypeId<"codrev">;
export type SourceLineageRevisionId = TypeId<"linrev">;
export type ProductionOperationId = TypeId<"prodop">;
export type PhysicalDatasetId = TypeId<"pds">;
export type PhysicalDatasetRevisionId = TypeId<"pdr">;
export type PhysicalFieldId = TypeId<"pfd">;
export type PhysicalFieldRevisionId = TypeId<"pfr">;
export type CodeArtifactId = TypeId<"cod">;
export type LineageEdgeId = TypeId<"lin">;
export type ConsumerId = TypeId<"csm">;
export type ConsumerBindingId = TypeId<"cbd">;
export type SemanticQueryId = TypeId<"smq">;
export type ResolvedSemanticPlanId = TypeId<"rsp">;
export type QueryValidationRunId = TypeId<"qvr">;
export type AttentionItemId = TypeId<"ati">;
export type ArtifactId = TypeId<"art">;
export type ArtifactSetId = TypeId<"ars">;
export type SourceScheduleId = TypeId<"sch">;
export type ScheduleOccurrenceId = TypeId<"occ">;
export type ClientCredentialId = TypeId<"ccd">;
export type WebhookSubscriptionId = TypeId<"whs">;
export type WebhookDeliveryId = TypeId<"whd">;

const suffixPattern = "[0-7][0-9a-hjkmnp-tv-z]{25}";

export function parseTypeId<Prefix extends ResourcePrefix>(prefix: Prefix, value: string): TypeId<Prefix> {
  if (!new RegExp(`^${prefix}_${suffixPattern}$`).test(value)) {
    throw new TypeError(`invalid ${prefix} TypeID`);
  }
  return value as TypeId<Prefix>;
}
