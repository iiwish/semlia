import { GitPullRequestArrow } from "lucide-react";
import { useOptionalCatalogRuntime } from "./catalogRuntime";
import { useAssetTitles } from "./knowledgeVocabulary";
import { emptyVocabulary } from "./knowledge";
import type { Asset, KnowledgeRevisionRequest } from "./types";

// AssetVocabularySection shows the governed wording of the current revision: how people name the knowledge, where its
// boundary lies, what it is confused with and which questions it must answer.
export function AssetVocabularySection({ asset, onStartRevision }: { asset: Asset; onStartRevision: (request: KnowledgeRevisionRequest) => void }) {
  const workspaceId = useOptionalCatalogRuntime()?.workspaceId;
  const vocabulary = asset.vocabulary ?? { ...emptyVocabulary(), aliases: asset.aliases, includes: asset.includes, excludes: asset.excludes, examples: asset.examples };
  const titles = useAssetTitles(workspaceId, vocabulary.confusableWith.map((item) => item.ref.assetId));
  const members = (asset.knowledgeSpec?.members ?? []).filter((member) => member.aliases?.length || member.valueAliases?.length);
  const rows: Array<[string, string[]]> = [
    ["别名", vocabulary.aliases],
    ["口径包含", vocabulary.includes],
    ["明确排除", vocabulary.excludes],
    ["示例", vocabulary.examples],
    ["易混淆知识", vocabulary.confusableWith.map((item) => `${titles[item.ref.assetId] ?? "已发布知识"}：${item.distinction}`)],
    ["典型问题", vocabulary.sampleQuestions.map((item) => item.question)],
    ["属性与取值别名", members.map((member) => [`${member.name || member.id}${member.aliases?.length ? `（${member.aliases.join("、")}）` : ""}`, ...(member.valueAliases ?? []).map((entry) => `${entry.value} ← ${entry.aliases.join("、")}`)].join("；"))],
  ];
  const filled = rows.filter(([, values]) => values.length > 0);
  return <section aria-labelledby={`vocabulary-${asset.id}`}>
    <header><div><h3 id={`vocabulary-${asset.id}`}>业务词汇</h3></div><button className="secondary-button" type="button" onClick={() => onStartRevision({ assetId: asset.id, fieldPath: "vocabulary", origin: "definition" })}><GitPullRequestArrow size={15} />修订业务词汇</button></header>
    {filled.length > 0 ? <dl>{filled.map(([label, values]) => <div key={label}><dt>{label}</dt><dd>{values.join("；")}</dd></div>)}</dl> : <p className="vocabulary-empty">尚未登记别名、易混淆知识或典型问题。问数时只能按名称匹配这项知识。</p>}
  </section>;
}
