const typeIdBrand: unique symbol = Symbol("SemliaTypeId");

export type ResourcePrefix = "wsp" | "ast" | "rev" | "rel" | "ont" | "evd" | "rls" | "run" | "evt";
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

const suffixPattern = "[0-7][0-9a-hjkmnp-tv-z]{25}";

export function parseTypeId<Prefix extends ResourcePrefix>(prefix: Prefix, value: string): TypeId<Prefix> {
  if (!new RegExp(`^${prefix}_${suffixPattern}$`).test(value)) {
    throw new TypeError(`invalid ${prefix} TypeID`);
  }
  return value as TypeId<Prefix>;
}
