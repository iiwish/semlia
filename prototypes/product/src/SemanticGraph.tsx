import { ArrowRight, Database, FileCheck2, Gauge, Users } from "lucide-react";
import type { AssetRelation, AssetType } from "./types";

interface SemanticGraphProps {
  mode: "coverage" | "lineage";
  assetName?: string;
  upstream?: string[];
  downstream?: string[];
  consumerName?: string;
  assetType?: AssetType;
  assetRevision?: string;
  ontologyRevision?: string;
  relations?: AssetRelation[];
}

const relationLabels: Record<AssetRelation["type"], string> = {
  measures: "衡量",
  describes: "描述",
  depends_on: "依赖",
  derived_from: "派生自",
  filters_by: "按其筛选",
  synonym_of: "同义于",
  contains: "包含",
};

function graphLabel(value: string, length = 15) {
  return value.length > length ? `${value.slice(0, length)}…` : value;
}

function graphPositions(count: number) {
  if (count <= 1) return [160];
  if (count === 2) return [92, 228];
  return [52, 160, 268];
}

export function SemanticGraph({ mode, assetName = "净收入", upstream = ["支付订单", "退款金额"], downstream = ["区域达成率"], consumerName = "Fluxale", assetType = "指标", assetRevision = "@12", ontologyRevision = "ontology:commerce@7", relations = [] }: SemanticGraphProps) {
  if (mode === "coverage") {
    return (
      <div className="semantic-map" role="img" aria-label="物理映射关系图">
        <svg viewBox="0 0 900 390" aria-hidden="true">
          <defs>
            <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" className="map-arrow" />
            </marker>
          </defs>
          <path className="map-link trace-link" markerEnd="url(#arrow)" d="M166 194 C240 194 240 92 328 92" />
          <path className="map-link trace-link trace-delay-1" markerEnd="url(#arrow)" d="M166 194 C240 194 240 286 328 286" />
          <path className="map-link trace-link trace-delay-2" markerEnd="url(#arrow)" d="M466 92 C548 92 548 154 618 154" />
          <path className="map-link trace-link trace-delay-3" markerEnd="url(#arrow)" d="M466 286 C548 286 548 222 618 222" />
          <path className="map-link trace-link trace-delay-4" markerEnd="url(#arrow)" d="M756 188 C800 188 816 188 852 188" />
          <g className="map-node source-node">
            <rect x="36" y="151" width="130" height="86" rx="8" />
            <text x="54" y="179" className="map-kicker">数据源</text>
            <text x="54" y="207" className="map-title">src-ecommerce</text>
            <text x="54" y="226" className="map-meta">revision 9f2e8a</text>
          </g>
          <g className="map-node domain-node">
            <rect x="328" y="49" width="138" height="86" rx="8" />
            <text x="346" y="77" className="map-kicker">物理对象</text>
            <text x="346" y="105" className="map-title">analytics.orders</text>
            <text x="346" y="124" className="map-meta">42 个字段 · 当前</text>
          </g>
          <g className="map-node domain-node attention-node">
            <rect x="328" y="243" width="138" height="86" rx="8" />
            <text x="346" y="271" className="map-kicker">物理对象</text>
            <text x="346" y="299" className="map-title">dim_region</text>
            <text x="346" y="318" className="map-meta">8 个字段 · 需重验</text>
          </g>
          <g className="map-node release-node">
            <rect x="618" y="145" width="138" height="86" rx="8" />
            <text x="636" y="173" className="map-kicker">绑定与 Join</text>
            <text x="636" y="201" className="map-title">Binding Registry</text>
            <text x="636" y="220" className="map-meta">139 已验证 · 4 待重验</text>
          </g>
          <g className="map-node consumer-node">
            <rect x="792" y="145" width="92" height="86" rx="8" />
            <text x="808" y="173" className="map-kicker">语义资产</text>
            <text x="808" y="201" className="map-title">148</text>
            <text x="808" y="220" className="map-meta">个稳定身份</text>
          </g>
        </svg>
        <div className="map-legend" aria-hidden="true">
          <span><Database size={14} />SourceRevision</span>
          <ArrowRight size={14} />
          <span><Gauge size={14} />物理对象</span>
          <ArrowRight size={14} />
          <span><FileCheck2 size={14} />Binding / JoinContract</span>
          <ArrowRight size={14} />
          <span><Users size={14} />语义资产</span>
        </div>
      </div>
    );
  }

  const semanticUpstream = relations.filter((relation) => relation.type === "depends_on" || relation.type === "derived_from");
  const semanticRelated = relations.filter((relation) => relation.type !== "depends_on" && relation.type !== "derived_from");
  const leftNodes = [
    ...semanticUpstream.map((relation) => ({ id: relation.id, label: relation.targetName, relation: relation.type === "derived_from" ? "派生输入" : "依赖输入", meta: `${relation.type} · ${relation.release}` })),
    ...upstream.filter((name) => !semanticUpstream.some((relation) => relation.targetName === name)).map((name, index) => ({ id: `upstream-${index}`, label: name, relation: "上游输入", meta: "已解析依赖" })),
  ].slice(0, 3);
  const rightNodes = [
    ...semanticRelated.map((relation) => ({ id: relation.id, label: relation.targetName, relation: relationLabels[relation.type], meta: `${relation.type} · ${relation.release}` })),
    ...downstream.filter((name) => !semanticRelated.some((relation) => relation.targetName === name)).map((name, index) => ({ id: `downstream-${index}`, label: name, relation: "影响", meta: "下游资产依赖当前资产" })),
  ].slice(0, 3);
  const leftPositions = graphPositions(leftNodes.length);
  const rightPositions = graphPositions(rightNodes.length);
  const markerId = `lineage-arrow-${assetName.replace(/[^a-zA-Z0-9]/g, "") || "asset"}`;

  return (
    <figure className="lineage-map ontology-lineage-map" aria-labelledby="ontology-lineage-caption">
      <svg viewBox="0 0 960 372" role="img" aria-label={`${assetName} 的本体关系血缘图`}>
        <defs>
          <marker id={markerId} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" className="ontology-lineage-arrow" />
          </marker>
        </defs>
        <text x="28" y="24" className="ontology-column-label">上游语义</text>
        <text x="390" y="24" className="ontology-column-label">当前资产</text>
        <text x="744" y="24" className="ontology-column-label">相关与下游</text>
        {leftNodes.map((node, index) => {
          const y = leftPositions[index];
          return <g key={node.id}>
            <path className={`ontology-lineage-link trace-delay-${Math.min(index, 4)}`} markerEnd={`url(#${markerId})`} d={`M228 ${y + 36} C300 ${y + 36} 302 186 370 186`} />
            <text x="272" y={(y + 186) / 2 + 14} className="ontology-edge-label">{node.relation}</text>
            <g className="lineage-node lineage-node-upstream"><title>{node.label} · {node.meta}</title><rect x="28" y={y} width="200" height="72" rx="7" /><text x="46" y={y + 23} className="lineage-node-kicker">语义资产</text><text x="46" y={y + 45} className="lineage-node-title">{graphLabel(node.label)}</text><text x="46" y={y + 62} className="lineage-node-meta">{graphLabel(node.meta, 23)}</text></g>
          </g>;
        })}
        <g className="lineage-node lineage-node-current"><rect x="370" y="137" width="220" height="98" rx="7" /><text x="390" y="164" className="lineage-node-kicker">{assetType} · 当前 revision</text><text x="390" y="193" className="lineage-node-title lineage-node-title-current">{graphLabel(assetName, 17)}</text><text x="390" y="216" className="lineage-node-meta">{assetRevision} · {graphLabel(ontologyRevision, 24)}</text></g>
        {rightNodes.map((node, index) => {
          const y = rightPositions[index];
          return <g key={node.id}>
            <path className={`ontology-lineage-link trace-delay-${Math.min(index + leftNodes.length, 4)}`} markerEnd={`url(#${markerId})`} d={`M590 186 C658 186 660 ${y + 36} 732 ${y + 36}`} />
            <text x="642" y={(y + 186) / 2 + 14} className="ontology-edge-label">{node.relation}</text>
            <g className="lineage-node lineage-node-related"><title>{node.label} · {node.meta}</title><rect x="732" y={y} width="200" height="72" rx="7" /><text x="750" y={y + 23} className="lineage-node-kicker">{node.relation === "影响" ? "下游影响" : "本体关系"}</text><text x="750" y={y + 45} className="lineage-node-title">{graphLabel(node.label)}</text><text x="750" y={y + 62} className="lineage-node-meta">{graphLabel(node.meta, 23)}</text></g>
          </g>;
        })}
      </svg>
      <figcaption id="ontology-lineage-caption"><span><i className="ontology-legend-upstream" />上游依赖</span><span><i className="ontology-legend-current" />当前资产</span><span><i className="ontology-legend-related" />本体关系与影响</span><code>{ontologyRevision}</code><span className="sr-only">消费者示例：{consumerName}</span></figcaption>
    </figure>
  );
}
