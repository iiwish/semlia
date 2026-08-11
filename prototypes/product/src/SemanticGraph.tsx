import { ArrowRight, Database, FileCheck2, Gauge, Users } from "lucide-react";

interface SemanticGraphProps {
  mode: "coverage" | "lineage";
  assetName?: string;
  upstream?: string[];
  downstream?: string[];
  consumerName?: string;
}

export function SemanticGraph({ mode, assetName = "净收入", upstream = ["支付订单", "退款金额"], downstream = ["区域达成率"], consumerName = "Fluxale" }: SemanticGraphProps) {
  if (mode === "coverage") {
    return (
      <div className="semantic-map" role="img" aria-label="语义覆盖关系图">
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
            <text x="54" y="179" className="map-kicker">SOURCE</text>
            <text x="54" y="207" className="map-title">Cube Core</text>
            <text x="54" y="226" className="map-meta">3 models · synced</text>
          </g>
          <g className="map-node domain-node">
            <rect x="328" y="49" width="138" height="86" rx="8" />
            <text x="346" y="77" className="map-kicker">DOMAIN</text>
            <text x="346" y="105" className="map-title">电商经营</text>
            <text x="346" y="124" className="map-meta">58 assets · 96%</text>
          </g>
          <g className="map-node domain-node attention-node">
            <rect x="328" y="243" width="138" height="86" rx="8" />
            <text x="346" y="271" className="map-kicker">DOMAIN</text>
            <text x="346" y="299" className="map-title">客户增长</text>
            <text x="346" y="318" className="map-meta">41 assets · 84%</text>
          </g>
          <g className="map-node release-node">
            <rect x="618" y="145" width="138" height="86" rx="8" />
            <text x="636" y="173" className="map-kicker">RELEASE</text>
            <text x="636" y="201" className="map-title">2026.08.3</text>
            <text x="636" y="220" className="map-meta">148 immutable assets</text>
          </g>
          <g className="map-node consumer-node">
            <rect x="792" y="145" width="92" height="86" rx="8" />
            <text x="808" y="173" className="map-kicker">BOUND</text>
            <text x="808" y="201" className="map-title">3</text>
            <text x="808" y="220" className="map-meta">consumers</text>
          </g>
        </svg>
        <div className="map-legend" aria-hidden="true">
          <span><Database size={14} />数据源</span>
          <ArrowRight size={14} />
          <span><Gauge size={14} />语义域</span>
          <ArrowRight size={14} />
          <span><FileCheck2 size={14} />发布版本</span>
          <ArrowRight size={14} />
          <span><Users size={14} />消费者</span>
        </div>
      </div>
    );
  }

  return (
    <div className="lineage-map" role="img" aria-label={`${assetName} 血缘关系图`}>
      <svg viewBox="0 0 800 330" aria-hidden="true">
        <defs>
          <marker id="lineage-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" className="map-arrow" />
          </marker>
        </defs>
        <path className="map-link trace-link" markerEnd="url(#lineage-arrow)" d="M166 80 C250 80 250 155 320 155" />
        <path className="map-link trace-link trace-delay-1" markerEnd="url(#lineage-arrow)" d="M166 246 C250 246 250 175 320 175" />
        <path className="map-link trace-link trace-delay-2" markerEnd="url(#lineage-arrow)" d="M480 165 C548 165 548 78 626 78" />
        <path className="map-link trace-link trace-delay-3" markerEnd="url(#lineage-arrow)" d="M480 165 C548 165 548 248 626 248" />
        <g className="map-node source-node">
          <rect x="32" y="42" width="134" height="76" rx="8" />
          <text x="48" y="69" className="map-kicker">UPSTREAM</text>
          <text x="48" y="94" className="map-title">{upstream[0] ?? "原始事实"}</text>
          <text x="48" y="109" className="map-meta">metric · stable</text>
        </g>
        <g className="map-node source-node">
          <rect x="32" y="208" width="134" height="76" rx="8" />
          <text x="48" y="235" className="map-kicker">UPSTREAM</text>
          <text x="48" y="260" className="map-title">{upstream[1] ?? "共享维度"}</text>
          <text x="48" y="275" className="map-meta">measure · stable</text>
        </g>
        <g className="map-node release-node selected-node">
          <rect x="320" y="121" width="160" height="88" rx="8" />
          <text x="338" y="150" className="map-kicker">SELECTED ASSET</text>
          <text x="338" y="179" className="map-title">{assetName}</text>
          <text x="338" y="198" className="map-meta">published · trusted</text>
        </g>
        <g className="map-node consumer-node">
          <rect x="626" y="40" width="142" height="76" rx="8" />
          <text x="642" y="67" className="map-kicker">DOWNSTREAM</text>
          <text x="642" y="92" className="map-title">{downstream[0] ?? "下游指标"}</text>
          <text x="642" y="107" className="map-meta">metric · 3 consumers</text>
        </g>
        <g className="map-node consumer-node">
          <rect x="626" y="210" width="142" height="76" rx="8" />
          <text x="642" y="237" className="map-kicker">CONSUMER</text>
          <text x="642" y="262" className="map-title">{consumerName}</text>
          <text x="642" y="277" className="map-meta">locked release</text>
        </g>
      </svg>
    </div>
  );
}
