import { useEffect, useState } from "react";
import { getAsset } from "./catalog";
import { publishedReference } from "./KnowledgeReferencePicker";
import type { KnowledgeReference, KnowledgeSpec, KnowledgeVocabulary } from "./knowledge";

// useCurrentPins reads the current published pin of each referenced asset so stale references can be refreshed.
export function useCurrentPins(workspaceId: string | undefined, assetIds: string[]) {
  const key = [...new Set(assetIds)].sort().join(",");
  const [pins, setPins] = useState<Record<string, KnowledgeReference | null>>({});
  useEffect(() => {
    if (!workspaceId || !key) return;
    const controller = new AbortController();
    void Promise.all(key.split(",").map(async (id) => {
      try { return [id, publishedReference(await getAsset(workspaceId, id, controller.signal)) ?? null] as const; }
      catch { return [id, null] as const; }
    })).then((entries) => { if (!controller.signal.aborted) setPins(Object.fromEntries(entries)); });
    return () => controller.abort();
  }, [workspaceId, key]);
  return pins;
}

export function isStale(ref: KnowledgeReference, pins: Record<string, KnowledgeReference | null>) {
  const current = pins[ref.assetId];
  return !!current && current.revisionId !== ref.revisionId;
}

export function refresh(ref: KnowledgeReference, pins: Record<string, KnowledgeReference | null>): KnowledgeReference {
  const current = pins[ref.assetId];
  return current ? { ...current, ...(ref.memberId ? { memberId: ref.memberId } : {}) } : ref;
}

// vocabularyProblems reports what the server would reject, so the author fixes it before saving.
export function vocabularyProblems(vocabulary: KnowledgeVocabulary, spec: KnowledgeSpec): string[] {
  const problems: string[] = [];
  if (vocabulary.confusableWith.some((item) => !item.ref.assetId)) problems.push("易混淆知识需要选择已发布知识。");
  if (vocabulary.confusableWith.some((item) => !item.distinction.trim())) problems.push("每个易混淆知识都需要写明区分说明。");
  if (vocabulary.sampleQuestions.some((item) => !item.question.trim())) problems.push("典型问题不能为空。");
  if (vocabulary.sampleQuestions.some((item) => item.expects?.refs.some((ref) => !ref.assetId))) problems.push("期望召回的知识需要选择已发布知识，或移除该项。");
  if ((spec.members ?? []).some((member) => member.valueAliases?.some((entry) => !entry.value.trim() || entry.aliases.length === 0))) problems.push("每个取值别名都需要规范取值和至少一个对应说法。");
  return problems;
}

export function vocabularyReferences(vocabulary: KnowledgeVocabulary): KnowledgeReference[] {
  return [...vocabulary.confusableWith.map((item) => item.ref), ...vocabulary.sampleQuestions.flatMap((item) => item.expects?.refs ?? [])];
}

// refreshVocabularyPins points every stale vocabulary reference at its asset's current published revision.
export function refreshVocabularyPins(vocabulary: KnowledgeVocabulary, pins: Record<string, KnowledgeReference | null>): KnowledgeVocabulary {
  return {
    ...vocabulary,
    confusableWith: vocabulary.confusableWith.map((item) => ({ ...item, ref: refresh(item.ref, pins) })),
    sampleQuestions: vocabulary.sampleQuestions.map((item) => item.expects ? { ...item, expects: { refs: item.expects.refs.map((ref) => refresh(ref, pins)) } } : item),
  };
}


// useAssetTitles resolves display names of referenced assets the caller can read; unreadable ones stay unnamed.
export function useAssetTitles(workspaceId: string | undefined, assetIds: string[]) {
  const key = [...new Set(assetIds.filter(Boolean))].sort().join(",");
  const [titles, setTitles] = useState<Record<string, string>>({});
  useEffect(() => {
    if (!workspaceId || !key) return;
    const controller = new AbortController();
    void Promise.all(key.split(",").map(async (id) => {
      try { return [id, (await getAsset(workspaceId, id, controller.signal)).title] as const; }
      catch { return [id, ""] as const; }
    })).then((entries) => { if (!controller.signal.aborted) setTitles(Object.fromEntries(entries.filter(([, title]) => title))); });
    return () => controller.abort();
  }, [workspaceId, key]);
  return titles;
}
