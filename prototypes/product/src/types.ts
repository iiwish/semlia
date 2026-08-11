export type ViewId = "sources" | "overview" | "assets" | "proposals" | "releases" | "consumers";

export type AssetType = "指标" | "维度" | "实体";
export type AssetStatus = "已发布" | "需关注" | "草稿";

export interface Evidence {
  id: string;
  kind: string;
  label: string;
  source: string;
  verifiedAt: string;
}

export interface Consumer {
  name: string;
  kind: string;
  binding: string;
  lastResolved: string;
}

export interface Asset {
  id: string;
  key: string;
  name: string;
  type: AssetType;
  status: AssetStatus;
  trustScore: number;
  owner: string;
  domain: string;
  definition: string;
  formula: string;
  grain: string;
  source: string;
  updatedAt: string;
  release: string;
  tags: string[];
  evidence: Evidence[];
  upstream: string[];
  downstream: string[];
  consumers: Consumer[];
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
  risk: "低风险" | "中风险" | "高风险";
  summary: string;
  changes: Array<{
    field: string;
    before: string;
    after: string;
  }>;
  validations: Validation[];
  impact: string[];
}

export interface Release {
  id: string;
  state: "Stable" | "Superseded";
  publishedAt: string;
  publisher: string;
  assetCount: number;
  summary: string;
  changes: string[];
  consumers: Consumer[];
}
