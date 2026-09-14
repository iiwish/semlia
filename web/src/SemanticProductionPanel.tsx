import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Check, CheckCircle2, ChevronRight, Database, FileCheck2, GitBranch, LoaderCircle, Plus, RefreshCw, RotateCcw, Save, Search, ShieldCheck, Sparkles, Trash2, X } from "lucide-react";
import { useCan, useResourceCan } from "./authorization";
import { getAsset, listAssets, type CatalogAsset } from "./catalog";
import { useCatalogRuntime } from "./catalogRuntime";
import { decideSemanticCandidate, DiscoveryApiError, getSemanticCandidate, listSourceRuns, type SemanticCandidate } from "./discovery";
import { listModelProviders, type GovernanceModelSetting } from "./governance";
import { readCandidateRecovery } from "./candidateRecovery";
import { useSemanticProduction } from "./semanticProductionRuntime";
import { buildProductionInput, getProductionSnapshot, kindLabels, listProductionMembers, makeAssetTarget, physicalReference, productionAPI, ProductionApiError, productionMessage, progressLabels, targetChanges, type AssetContent, type BusinessRuleWitness, type GenerationResult, type PhysicalReference, type ProductionContent, type ProductionDraft, type ProductionOperation, type ProductionTarget, type SnapshotMember, type SourceSnapshot, type TargetKind } from "./semanticProduction";

type Member = SnapshotMember & { snapshotId: string };
type Permissions = { edit: boolean; validate: boolean; review: boolean; publish: boolean; rollback: boolean };
type Props = { candidateId?: string; operationId?: string; onBack?: () => void; onOpenProposal: (id: string) => void; onOpenAsset?: (id: string) => void; onOperationSelected?: (id: string, releaseId?: string) => void };
export function SemanticProductionWorkspace(props: Props) {
  const { workspaceId } = useCatalogRuntime();
  const actor = useResourceCan("asset.read", { type: "workspace", id: workspaceId });
  const permissions = { edit: useCan("asset.propose"), validate: useCan("validation.run"), review: useCan("proposal.review"), publish: useCan("release.publish"), rollback: useCan("release.rollback") };
  return <SemanticProductionPanel key={`${workspaceId}:${actor.principalId}:${actor.authorizationVersion}`} {...props} workspaceId={workspaceId} principalId={actor.principalId} identityKey={`${actor.principalId}:${actor.authorizationVersion}`} permissions={permissions} />;
}

export function SemanticProductionPanel({ workspaceId, principalId, identityKey, candidateId, operationId, permissions, onBack, onOpenProposal, onOpenAsset, onOperationSelected }: Props & { workspaceId: string; principalId: string; identityKey: string; permissions: Permissions }) {
  const runtime = useSemanticProduction({ workspaceId, identityKey, candidateId, operationId });
  const [candidate, setCandidate] = useState<SemanticCandidate | null>(null);
  const [snapshots, setSnapshots] = useState<SourceSnapshot[]>([]);
  const [members, setMembers] = useState<Member[]>([]);
  const [cursors, setCursors] = useState<Record<string, string | null>>({});
  const [sourceError, setSourceError] = useState("");
  const [sourceLoading, setSourceLoading] = useState(Boolean(candidateId));
  const [sourceRetry, setSourceRetry] = useState(0);
  const [draft, setDraft] = useState<ProductionDraft | null>(null);
  const [dirty, setDirty] = useState(false);
  const [draftKey, setDraftKey] = useState("");
  const [selectedKey, setSelectedKey] = useState("");
  const [stage, setStage] = useState<"model" | "validation" | "release">("model");
  const [newKind, setNewKind] = useState<TargetKind>("semantic_asset");
  const [localError, setLocalError] = useState("");
  const [matching, setMatching] = useState(false);
  const [sourceExpanded, setSourceExpanded] = useState(false);
  const [applyRunId, setApplyRunId] = useState<string>();
  const [confirmPublish, setConfirmPublish] = useState(false);
  const [rollbackReason, setRollbackReason] = useState("");
  const [dismissReason, setDismissReason] = useState("");
  const [legacyNotice] = useState(() => {
    if (!candidateId) return null;
    try { const record = readCandidateRecovery(`${workspaceId}:${principalId}`, candidateId); return record ? { proposalId: record.proposalId, message: "此候选有旧版浏览器恢复记录。仅保留旧提案入口，不自动重建或关联。" } : null; }
    catch { return { proposalId: "", message: "旧版浏览器恢复记录不可读。新生产状态以服务器查询为准。" }; }
  });
  const firstFocus = useRef<HTMLButtonElement>(null);
  const acceptedSave = useRef("");
  useEffect(() => { firstFocus.current?.focus(); }, []);
  const detail = runtime.detail;
  const primaryTitle = detail?.targets.find((target) => detail.input.candidates.some((pin) => pin.primaryTargetKey === target.localKey))?.declaration.title ?? detail?.targets.find((target) => target.declaration.kind === "semantic_asset")?.declaration.title ?? detail?.targets[0]?.declaration.title;
  const loadedKey = detail ? `${detail.summary.id}:${detail.version}` : "";
  const sourceSignature = detail?.input.snapshots.map((pin) => `${pin.sourceId}:${pin.snapshotId}`).join("|") ?? "";

  useEffect(() => {
    const controller = new AbortController();
    if (!candidateId && !sourceSignature) return;
    void (async () => {
      await Promise.resolve();
      if (controller.signal.aborted) return;
      setSourceLoading(true); setSourceError("");
      let selectedSnapshots: SourceSnapshot[];
      if (sourceSignature) {
        selectedSnapshots = await Promise.all(sourceSignature.split("|").map((pair) => { const [sourceId, snapshotId] = pair.split(":"); return getProductionSnapshot(workspaceId, sourceId, snapshotId, controller.signal); }));
      } else {
        const fresh = await getSemanticCandidate(workspaceId, candidateId!, controller.signal);
        const snapshotId = await candidateSnapshotId(workspaceId, fresh, controller.signal);
        const snapshot = await getProductionSnapshot(workspaceId, fresh.sourceConnectionId, snapshotId, controller.signal);
        if (snapshot.sourceRevisionId !== fresh.sourceRevisionId) throw new Error("候选与来源快照不匹配。");
        selectedSnapshots = [snapshot];
        if (!controller.signal.aborted) setCandidate(fresh);
      }
      const pages = await Promise.all(selectedSnapshots.map((snapshot) => listProductionMembers(workspaceId, snapshot.sourceId, snapshot.id, undefined, controller.signal)));
      if (controller.signal.aborted) return;
      setSnapshots(selectedSnapshots);
      setMembers(pages.flatMap((page) => page.items.map((item) => ({ ...item, snapshotId: page.snapshotId }))));
      setCursors(Object.fromEntries(pages.map((page) => [page.snapshotId, page.nextCursor])));
    })().catch((reason) => { if (!controller.signal.aborted) { setSnapshots([]); setMembers([]); setSourceError(productionMessage(reason)); } }).finally(() => { if (!controller.signal.aborted) setSourceLoading(false); });
    return () => controller.abort();
  }, [candidateId, sourceSignature, workspaceId, sourceRetry]);

  useEffect(() => {
    if (!detail || loadedKey === draftKey) return;
    if (dirty && draftKey && loadedKey !== acceptedSave.current) return;
    setDraft({ input: detail.input, targets: detail.targets.map((target) => target.declaration) });
    setDraftKey(loadedKey); setDirty(false); setApplyRunId(undefined); setConfirmPublish(false);
    setSelectedKey((current) => detail.targets.some((target) => target.localKey === current) ? current : detail.targets[0]?.localKey ?? "");
  }, [detail, dirty, draftKey, loadedKey]);
  const selectedOperationId = detail?.summary.id;
  const selectedReleaseId = runtime.release?.id;
  useEffect(() => { if (selectedOperationId) onOperationSelected?.(selectedOperationId, selectedReleaseId); }, [selectedOperationId, selectedReleaseId, onOperationSelected]);

  const selectedTarget = draft?.targets.find((target) => target.localKey === selectedKey);
  const locked = runtime.busy || runtime.uncertain || runtime.readFailed || !permissions.edit || Boolean(detail?.summary.releaseId) || (Boolean(operationId) && !detail);
  const canSave = !locked && Boolean(draft?.targets.length) && !sourceError && !sourceLoading && (dirty || !detail);
  const saved = Boolean(detail && !dirty && draftKey === loadedKey);
  const isContributor = detail?.summary.createdBy === principalId || runtime.rules.some((rule) => rule.event.principalId === principalId) || detail?.generationApplications.some((application) => application.actorPrincipalId === principalId);

  const start = () => {
    if (!candidate || !snapshots[0] || runtime.items.length || runtime.loading || runtime.error) return;
    const address = typeof candidate.proposalInput.qualifiedName === "string" ? candidate.proposalInput.qualifiedName.replace(/[^a-zA-Z0-9_.]/g, "_") : "";
    const target = makeAssetTarget("primary", candidate.title, address.includes(".") ? address : `semantic.${address || "untitled"}`, candidate.candidateKind === "join" ? "entity" : candidate.candidateKind, principalId);
    try { setDraft({ input: buildProductionInput(snapshots[0], candidate, [target.localKey], target.localKey), targets: [target] }); setSelectedKey(target.localKey); setDirty(true); setLocalError(""); }
    catch (reason) { setLocalError(productionMessage(reason)); }
  };
  const updateTarget = (target: ProductionTarget) => {
    if (!draft) return;
    const current = draft.targets.find((item) => item.localKey === target.localKey);
    const next = target.intent === "update" && current ? { ...target, changes: targetChanges(baseContent(current), target.content) } : target;
    setDraft({ ...draft, targets: draft.targets.map((item) => item.localKey === next.localKey ? next : item) }); setDirty(true); setConfirmPublish(false);
  };
  const addTarget = () => {
    if (!draft) return;
    const key = `object_${crypto.randomUUID().slice(0, 8)}`;
    const title = newKind === "semantic_asset" ? "新语义资产" : kindLabels[newKind];
    const firstAsset = draft.targets.find((target) => target.kind === "semantic_asset" && target.intent === "create");
    const dataset = members.find((member) => member.kind === "dataset");
    const fields = members.filter((member) => member.kind === "field" && member.parentObjectId === dataset?.objectId);
    const physical = dataset ? physicalReference(dataset.snapshotId, dataset) : { snapshotId: "", kind: "dataset" as const, objectId: "", revisionId: "" };
    const asset = { localKey: firstAsset?.localKey ?? "" };
    const content: ProductionContent = newKind === "physical_binding" ? { asset, dataset: physical } : newKind === "model_grain" ? { asset, expression: "", fields: [] } : newKind === "entity_key" ? { asset, fields: [], uniqueness: "exact" } : { leftDataset: physical, rightDataset: physical, pairs: fields[0] ? [{ left: physicalReference(fields[0].snapshotId, fields[0]), right: physicalReference(fields[0].snapshotId, fields[0]) }] : [], joinType: "left", cardinality: "many_to_one", expression: "" };
    const target: ProductionTarget = newKind === "semantic_asset" ? makeAssetTarget(key, title, `semantic.${key}`, "entity", principalId) : { intent: "create", kind: newKind, localKey: key, identityKey: `${firstAsset?.intent === "create" ? firstAsset.identityKey : "semantic"}.${key}`, title, content, changes: [], evidenceIds: [] };
    const targets = [...draft.targets, target];
    setDraft({ ...draft, targets, input: { ...draft.input, candidates: draft.input.candidates.map((pin) => ({ ...pin, targetKeys: targets.map((item) => item.localKey) })) } });
    setSelectedKey(key); setDirty(true);
  };
  const removeTarget = () => {
    if (!draft || !selectedTarget || draft.targets.length < 2 || draft.input.candidates.some((pin) => pin.primaryTargetKey === selectedTarget.localKey)) return;
    const targets = draft.targets.filter((target) => target.localKey !== selectedKey);
    setDraft({ ...draft, targets, input: { ...draft.input, candidates: draft.input.candidates.map((pin) => ({ ...pin, targetKeys: pin.targetKeys.filter((key) => key !== selectedKey) })) } }); setSelectedKey(targets[0].localKey); setDirty(true);
  };
  const save = async () => {
    if (!draft || !canSave) return;
    const body = structuredClone(draft);
    await runtime.write(detail ? "保存纠正版本" : "保存生产草稿", async (key) => {
      const result = detail?.summary.frozen
        ? await productionAPI.create(workspaceId, { ...body, supersedesOperationId: detail.summary.id }, key)
        : detail
          ? await productionAPI.replace(workspaceId, detail.summary.id, { ...body, expectedVersion: Number(draftKey.split(":").at(-1)), ...(applyRunId ? { suggestionRunId: applyRunId } : {}) }, key)
          : await productionAPI.create(workspaceId, body, key);
      acceptedSave.current = `${result.operationId}:${result.version}`;
      return result;
    });
  };
  const commandPin = detail ? { expectedVersion: detail.version, setDigest: detail.setDigest } : null;
  const validation = detail?.activeValidation;
  const validReference = validation && validation.status !== "not_requested" && validation.validationDigest ? { attemptNo: validation.attemptNo, validationDigest: validation.validationDigest } : null;
  const onValidate = (reason: string) => {
    if (!detail || !commandPin || !saved) return;
    void runtime.write("重新验证", (key) => productionAPI.validate(workspaceId, detail.summary.id, { ...commandPin, previousAttemptNo: detail.activeValidation.status === "not_requested" ? 0 : detail.activeValidation.attemptNo, reason }, key));
  };
  const onReview = (decision: "approve" | "reject", note: string) => {
    if (!detail || !commandPin || !validReference || !saved) return;
    void runtime.write(decision === "approve" ? "集合审核" : "拒绝集合", (key) => productionAPI.review(workspaceId, detail.summary.id, { ...commandPin, validation: validReference, proposalIds: detail.targets.flatMap((target) => target.proposalId ? [target.proposalId] : []), decision, note }, key));
  };
  const onRule = (targetKey: string, declaration?: string) => {
    if (!detail || !commandPin || !saved) return;
    void runtime.write(declaration ? "确认业务规则" : "撤销业务规则", (key) => productionAPI.recordRule(workspaceId, detail.summary.id, declaration ? { ...commandPin, targetKey, action: "confirm", declaration } : { ...commandPin, targetKey, action: "revoke" }, key));
  };
  const loadMoreMembers = async (snapshot: SourceSnapshot) => {
    if (!cursors[snapshot.id] || sourceLoading) return;
    setSourceLoading(true); setSourceError("");
    try { const page = await listProductionMembers(workspaceId, snapshot.sourceId, snapshot.id, cursors[snapshot.id]!); setMembers((current) => [...current, ...page.items.map((item) => ({ ...item, snapshotId: snapshot.id }))]); setCursors((current) => ({ ...current, [snapshot.id]: page.nextCursor })); }
    catch (reason) { setSourceError(productionMessage(reason)); } finally { setSourceLoading(false); }
  };
  const backToRecords = () => {
    runtime.clearSelection(); setDraft(null); setDraftKey(""); setDirty(false); setSelectedKey("");
    onOperationSelected?.("");
  };
  const dismissCandidate = () => {
    if (!candidate || candidate.status !== "pending" || runtime.items.length || legacyNotice || !permissions.edit || !dismissReason.trim()) return;
    const reason = dismissReason.trim();
    void runtime.write("忽略候选", async (key) => {
      try {
        await decideSemanticCandidate(workspaceId, candidate.id, { action: "dismiss", reason, idempotencyKey: key });
        setCandidate(await getSemanticCandidate(workspaceId, candidate.id));
        return {};
      } catch (error) {
        if (error instanceof DiscoveryApiError) throw new ProductionApiError(error.message, error.code, error.status);
        throw error;
      }
    });
  };

  return <section className="view production-workspace" aria-label="语义生产">
    <header className="production-heading"><div><button ref={firstFocus} className="ingestion-back" onClick={onBack ?? backToRecords}><ArrowLeft size={15} />{onBack ? "返回来源与候选" : "生产记录"}</button><h2>{candidate?.title ?? primaryTitle ?? "语义生产"}</h2><span className="production-muted">{detail ? `v${detail.version} · ${progressLabels[detail.summary.progress]}` : candidate ? "来源候选 · 尚未发布" : "服务器生产记录"}</span></div><div className="production-actions"><button className="icon-button" title="核对服务器状态" aria-label="核对服务器状态" disabled={runtime.busy || runtime.loading} onClick={() => void runtime.refresh()}><RefreshCw size={16} /></button>{draft && <button className="primary-button" disabled={!canSave} onClick={() => void save()}><Save size={15} />{detail ? "保存纠正版本" : "保存生产草稿"}</button>}</div></header>
    {runtime.error && <div className="production-alert" role="alert">{runtime.error}{runtime.uncertain && <button className="secondary-button" disabled={runtime.busy} onClick={() => void runtime.retry()}><RotateCcw size={14} />重试同一请求</button>}</div>}
    {runtime.notice && <p role="status" className="production-notice">{runtime.notice}</p>}
    {localError && <p role="alert" className="production-alert">{localError}</p>}
    {dirty && detail?.summary.frozen && <p className="production-notice">当前记录已冻结。保存会创建后继纠正记录，保留原始建议与审核历史；新记录必须重新确认、验证与审核。</p>}
    {dirty && draftKey && loadedKey && draftKey !== loadedKey && <p role="alert" className="production-alert">服务器出现新版本，本地纠正已保留。<button className="secondary-button" onClick={() => { setDirty(false); setDraftKey(""); }}>放弃本地纠正并读取新版本</button></p>}
    {legacyNotice && <p className="production-notice">{legacyNotice.message}{legacyNotice.proposalId && <button className="ingestion-link" onClick={() => onOpenProposal(legacyNotice.proposalId)}>打开旧提案<ChevronRight size={14} /></button>}</p>}
    {runtime.loading && !detail && <p role="status"><LoaderCircle className="is-spinning" size={16} />正在核对生产状态</p>}
    {!detail && !draft && <>
      {runtime.items.length > 0 && <section className="production-records" aria-label="可恢复的生产记录">{runtime.items.map((item) => <button className="production-record" key={item.id} disabled={runtime.busy} onClick={() => void runtime.open(item.id)}><FileCheck2 size={18} /><span><strong>{progressLabels[item.progress]} · v{item.currentVersion}</strong><small>{item.targetCount} 个对象 · {new Date(item.updatedAt).toLocaleString("zh-CN")}</small><code>{item.id}</code></span><ChevronRight size={16} /></button>)}</section>}
      {runtime.nextCursor && <button className="secondary-button" onClick={() => void runtime.loadMore()}>更多生产记录</button>}
      {!runtime.loading && !runtime.error && !runtime.items.length && !candidateId && <p className="production-empty">暂无生产记录</p>}
      {candidate && !runtime.items.length && <section className="production-start"><h3>候选建模</h3><p className="production-muted">{candidate.candidateKind} · {candidate.status === "pending" ? "待处理" : candidate.status === "converted" ? "已有提案" : "已忽略"}</p><button className="primary-button" disabled={candidate.status !== "pending" || sourceLoading || Boolean(sourceError) || runtime.loading || runtime.busy || runtime.uncertain || Boolean(runtime.error) || !permissions.edit} onClick={start}><Plus size={16} />新建语义资产</button>{candidate.proposalId && <button className="secondary-button" onClick={() => onOpenProposal(candidate.proposalId!)}>打开已有提案</button>}{candidate.status === "pending" && <details><summary>忽略此候选</summary><label className="production-field"><span>忽略依据</span><textarea aria-label="忽略依据" value={dismissReason} disabled={runtime.busy || runtime.uncertain || Boolean(legacyNotice)} onChange={(event) => setDismissReason(event.target.value)} /></label><button className="secondary-button" disabled={!permissions.edit || runtime.loading || runtime.busy || runtime.uncertain || Boolean(runtime.error) || Boolean(legacyNotice) || !dismissReason.trim()} onClick={dismissCandidate}><X size={14} />忽略候选</button></details>}</section>}
    </>}
    {(candidateId || detail) && <section className="production-source" aria-label="固定来源依据"><button className="production-source-toggle" aria-expanded={sourceExpanded} onClick={() => setSourceExpanded((value) => !value)}><Database size={16} /><span>固定来源依据<strong>{snapshots.length} 个快照 · {members.length} 个已加载成员</strong></span><ChevronRight size={15} /></button>{sourceLoading && <span role="status">正在读取来源</span>}{sourceError && <div role="alert" className="production-alert">{sourceError}<button onClick={() => setSourceRetry((value) => value + 1)}>重试来源读取</button></div>}{sourceExpanded && <><div className="production-source-pins">{snapshots.map((snapshot) => <article key={snapshot.id}><strong>{snapshot.historyQuality === "verified" ? "历史可验证" : "历史不可验证"} · {snapshot.coverageStatus === "complete" ? "覆盖完整" : "覆盖不完整"}</strong><code>{snapshot.id}</code><small>{snapshot.coverage.map((unit) => `${unit.key}: ${unit.status}`).join(" · ")}</small>{cursors[snapshot.id] && <button className="secondary-button" disabled={sourceLoading} onClick={() => void loadMoreMembers(snapshot)}>加载更多来源成员</button>}</article>)}</div><div className="production-member-list">{members.map((member) => <div key={memberKey(member)}><span>{member.kind}</span><strong>{member.name}</strong><small>{member.locator}</small></div>)}</div>{candidate && <details><summary>候选原始依据</summary><pre>{JSON.stringify({ sourceRevisionId: candidate.sourceRevisionId, proposalInput: candidate.proposalInput, evidence: candidate.evidence }, null, 2)}</pre></details>}</>}</section>}
    {draft && <>
      <div className="production-tabs" role="tablist" aria-label="生产阶段">{([["model", "建模与依据", GitBranch], ["validation", "验证与审核", ShieldCheck], ["release", "发布与恢复", FileCheck2]] as const).map(([value, label, Icon]) => <button key={value} role="tab" aria-selected={stage === value} disabled={value !== "model" && !detail} onClick={() => setStage(value)}><Icon size={15} />{label}</button>)}{dirty && <span className="production-dirty">未保存的纠正</span>}</div>
      {stage === "model" && <>
        <div className="production-model-layout"><aside className="production-targets" aria-label="生产对象"><div className="production-add"><select aria-label="新增对象类型" value={newKind} disabled={locked} onChange={(event) => setNewKind(event.target.value as TargetKind)}>{Object.entries(kindLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select><button className="icon-button" title="新增对象" aria-label="新增对象" disabled={locked || draft.targets.length >= 32} onClick={addTarget}><Plus size={16} /></button></div>{draft.targets.map((target) => <button key={target.localKey} className={`production-target${selectedKey === target.localKey ? " is-selected" : ""}`} aria-current={selectedKey === target.localKey ? "true" : undefined} onClick={() => setSelectedKey(target.localKey)}><span><strong>{target.title}</strong><small>{kindLabels[target.kind]} · {target.intent === "create" ? "新建" : "变更"}</small></span><ChevronRight size={14} /></button>)}</aside>
        {selectedTarget && <section className="production-editor" aria-label={`编辑 ${selectedTarget.title}`}><header><h3>{selectedTarget.title}</h3><div className="production-actions">{!detail && selectedTarget.kind === "semantic_asset" && <button className="secondary-button" disabled={locked} onClick={() => setMatching(true)}><Search size={14} />匹配已有资产</button>}<button className="icon-button" title="移除对象" aria-label="移除对象" disabled={locked || draft.targets.length < 2 || draft.input.candidates.some((pin) => pin.primaryTargetKey === selectedKey)} onClick={removeTarget}><Trash2 size={15} /></button></div></header><ProductionTargetEditor target={selectedTarget} targets={draft.targets} members={members} disabled={locked} onChange={updateTarget} />
          {selectedTarget.kind === "semantic_asset" && <BusinessRuleConfirmation key={`${draftKey}:${selectedKey}`} target={selectedTarget} witness={runtime.rules.find((rule) => rule.event.targetKey === selectedKey)} disabled={!saved || locked || detail?.targets.find((target) => target.localKey === selectedKey)?.outcome === "no_change"} onRecord={onRule} />}
          <details className="production-technical"><summary>版本、变更与引用</summary><pre>{JSON.stringify(selectedTarget, null, 2)}</pre>{detail?.targets.find((target) => target.localKey === selectedKey)?.proposalId && <button className="ingestion-link" onClick={() => onOpenProposal(detail.targets.find((target) => target.localKey === selectedKey)!.proposalId!)}>核对关联提案<ChevronRight size={14} /></button>}</details>
        </section>}</div>
        {detail && <GenerationSection workspaceId={workspaceId} detail={detail} generations={runtime.generations} disabled={!saved || locked || Boolean(detail.summary.frozen)} write={runtime.write} onApply={(run) => { if (run.status !== "succeeded" || !run.output || !detail || run.inputVersion !== detail.version) return; setDraft({ input: detail.input, targets: structuredClone((run.output as { targets: ProductionTarget[] }).targets) }); setApplyRunId(run.runId); setDirty(true); setSelectedKey((run.output as { targets: ProductionTarget[] }).targets[0]?.localKey ?? ""); }} />}
        {applyRunId && <p className="production-notice">AI 建议待保存；人工纠正将与原始输出分别归因。</p>}
        {detail && <footer className="production-footer"><span>{detail.targets.length} 个对象 · {detail.summary.frozen ? "版本已冻结" : "草稿"}</span><button className="primary-button" disabled={!saved || locked || detail.summary.frozen || detail.summary.progress === "no_change" || !permissions.validate} onClick={() => { void runtime.write("冻结并提交验证", (key) => productionAPI.submit(workspaceId, detail.summary.id, commandPin!, key)); setStage("validation"); }}><ShieldCheck size={15} />冻结并提交验证</button></footer>}
      </>}
      {stage === "validation" && detail && <ProductionValidationView detail={detail} dirty={!saved} busy={runtime.busy || runtime.uncertain} canValidate={permissions.validate} canReview={permissions.review && !isContributor} onValidate={onValidate} onReview={onReview} />}
      {stage === "release" && detail && <section className="production-release" aria-label="集合发布"><header><h3>{runtime.release ? `发布 #${runtime.release.sequence}` : "集合发布"}</h3><span>{progressLabels[detail.summary.progress]}</span></header><div className="production-publish-objects">{detail.targets.map((target) => <article key={target.localKey}><span>{kindLabels[target.declaration.kind]}</span><strong>{target.declaration.title}</strong><small>{target.outcome === "no_change" ? "无实质变化" : target.proposalState ?? "状态未提供"}</small>{runtime.release && target.declaration.kind === "semantic_asset" && onOpenAsset && <button className="ingestion-link" onClick={() => onOpenAsset(target.targetId)}>打开知识资产<ArrowRight size={14} /></button>}</article>)}</div>
        {!runtime.release && <><label className="production-check"><input type="checkbox" checked={confirmPublish} disabled={!saved || runtime.busy} onChange={(event) => setConfirmPublish(event.target.checked)} /><span>已核对整个集合及当前验证审核</span></label><button className="primary-button" disabled={!saved || runtime.busy || runtime.uncertain || !permissions.publish || Boolean(isContributor) || detail.summary.progress !== "ready_to_publish" || !confirmPublish || !validReference} onClick={() => void runtime.write("发布集合", (key) => productionAPI.publish(workspaceId, detail.summary.id, { ...commandPin!, validation: validReference!, expectedHead: detail.baselineHead }, key))}><CheckCircle2 size={15} />发布整个集合</button></>}
        {runtime.release && <><dl className="production-release-pins"><dt>发布标识</dt><dd><code>{runtime.release.id}</code></dd><dt>内容摘要</dt><dd><code>{runtime.release.afterManifest.digest}</code></dd><dt>内容投影</dt><dd>{({ pending: "同步中", ready: "已同步", failed: "同步失败" } as const)[runtime.release.projectionStatus]}</dd><dt>回滚深度</dt><dd>{runtime.release.protection.rollbackDepth}</dd></dl><label className="production-field"><span>回滚原因</span><textarea aria-label="回滚原因" value={rollbackReason} onChange={(event) => setRollbackReason(event.target.value)} /></label><button className="secondary-button" disabled={!permissions.rollback || runtime.busy || runtime.uncertain || !rollbackReason.trim()} onClick={() => { const release = runtime.release!; void runtime.write("回滚发布", (key) => productionAPI.rollback(workspaceId, release.id, { ...commandPin!, expectedHead: { presence: "present", releaseId: release.id, manifestDigest: release.afterManifest.digest }, reason: rollbackReason.trim() }, key)); }}><RotateCcw size={15} />创建回滚版本</button><details><summary>不可变发布清单与审核归因</summary><pre>{JSON.stringify(runtime.release, null, 2)}</pre></details></>}
      </section>}
    </>}
    {matching && selectedTarget && <MatchAssetDialog workspaceId={workspaceId} target={selectedTarget} onClose={() => setMatching(false)} onMatch={(target) => { setDraft((current) => current ? { ...current, targets: current.targets.map((item) => item.localKey === target.localKey ? target : item) } : current); setDirty(true); setMatching(false); }} />}
  </section>;
}

function baseContent(target: ProductionTarget): ProductionContent {
  const base = structuredClone(target.content) as unknown as Record<string, unknown>;
  if (target.intent === "update") for (const change of target.changes) {
    if (change.op === "add") delete base[change.fieldPath];
    else base[change.fieldPath] = change.beforeValue;
  }
  return base as unknown as ProductionContent;
}
async function candidateSnapshotId(workspaceId: string, candidate: SemanticCandidate, signal: AbortSignal): Promise<string> {
  let cursor: string | undefined;
  for (let page = 0; page < 100; page++) {
    const runs = await listSourceRuns(workspaceId, candidate.sourceConnectionId, { cursor, signal });
    const run = runs.items.find((item) => item.id === candidate.discoveryRunId);
    if (run) { if (run.snapshotId) return run.snapshotId; throw new Error("发现运行没有可验证的固定快照，不能开始生产。"); }
    cursor = runs.nextCursor;
    if (!cursor) break;
  }
  throw new Error("无法从来源运行历史定位候选快照，尚未创建生产记录。");
}
function memberKey(member: Member | PhysicalReference) { return `${member.snapshotId}:${member.objectId}:${member.revisionId}`; }
function PhysicalSelect({ label, value, members, kind, onChange, optional = false }: { label: string; value?: PhysicalReference; members: Member[]; kind: "dataset" | "field"; onChange: (value: PhysicalReference | undefined) => void; optional?: boolean }) {
  const options = members.filter((member) => member.kind === kind);
  return <label className="production-field"><span>{label}</span><select aria-label={label} value={value ? memberKey(value) : ""} onChange={(event) => { const member = options.find((item) => memberKey(item) === event.target.value); onChange(member ? physicalReference(member.snapshotId, member) : undefined); }}><option value="">{optional ? "无" : "选择固定版本"}</option>{value && !options.some((member) => memberKey(member) === memberKey(value)) && <option value={memberKey(value)}>未加载的固定引用 · {value.objectId}</option>}{options.map((member) => <option key={memberKey(member)} value={memberKey(member)}>{member.name}</option>)}</select></label>;
}
export function ProductionTargetEditor({ target, targets, members, disabled, onChange }: { target: ProductionTarget; targets: ProductionTarget[]; members: Member[]; disabled: boolean; onChange: (target: ProductionTarget) => void }) {
  const content = target.content;
  const patch = (values: Record<string, unknown>) => onChange({ ...target, content: { ...content, ...values } as ProductionContent });
  const textField = (label: string, field: string, multiline = false, nullable = false) => {
    const values = content as unknown as Record<string, unknown>;
    const props = { "aria-label": label, value: typeof values[field] === "string" ? values[field] as string : "", onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => patch({ [field]: nullable && !event.target.value.trim() ? null : event.target.value }) };
    return <label className="production-field"><span>{label}</span>{multiline ? <textarea {...props} /> : <input {...props} />}</label>;
  };
  const assetRef = "asset" in content ? content.asset : undefined;
  const semanticSelect = <label className="production-field"><span>关联语义对象</span><select aria-label="关联语义对象" value={assetRef && "localKey" in assetRef ? assetRef.localKey : "published"} onChange={(event) => patch({ asset: { localKey: event.target.value } })}><option value="">选择本集合对象</option>{assetRef && !("localKey" in assetRef) && <option value="published">已发布引用 · {assetRef.targetId}</option>}{targets.filter((item) => item.kind === "semantic_asset" && item.intent === "create").map((item) => <option key={item.localKey} value={item.localKey}>{item.title}</option>)}</select></label>;
  const fields = "fields" in content ? content.fields : [];
  return <fieldset className="production-form" disabled={disabled}>
    <label className="production-field"><span>对象名称</span><input aria-label="对象名称" value={target.title} onChange={(event) => onChange({ ...target, title: event.target.value })} /></label>
    {target.intent === "create" && target.kind !== "semantic_asset" && <label className="production-field"><span>业务标识</span><input aria-label="业务标识" value={target.identityKey} onChange={(event) => onChange({ ...target, identityKey: event.target.value })} /></label>}
    {target.kind === "semantic_asset" && <><label className="production-field"><span>资产地址</span><input aria-label="资产地址" disabled={disabled || target.intent === "update"} value={(content as AssetContent).address} onChange={(event) => onChange({ ...target, ...(target.intent === "create" ? { identityKey: event.target.value } : {}), content: { ...content, address: event.target.value } as AssetContent })} /></label><label className="production-field"><span>语义类型</span><select aria-label="语义类型" value={(content as AssetContent).assetType} disabled={disabled || target.intent === "update"} onChange={(event) => patch({ assetType: event.target.value })}>{Object.entries({ concept: "业务概念", entity: "业务实体", semantic_model: "语义模型", dimension: "维度", measure: "度量", metric: "指标", segment: "分群" }).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>{textField("显示名称", "displayName")}{textField("业务定义", "definition", true, true)}{textField("适用范围", "scope", true, true)}<p className="production-muted">负责人：<code>{(content as AssetContent).ownerPrincipalId}</code></p></>}
    {target.kind === "physical_binding" && "dataset" in content && <>{semanticSelect}<PhysicalSelect label="绑定数据集" value={content.dataset} members={members} kind="dataset" onChange={(value) => { if (value) patch({ dataset: value }); }} /><PhysicalSelect label="绑定字段" value={content.field} members={members.filter((member) => member.parentObjectId === content.dataset.objectId)} kind="field" optional onChange={(value) => { const next = { ...content }; if (value) next.field = value; else delete next.field; onChange({ ...target, content: next }); }} />{textField("转换表达式", "transform", true)}</>}
    {(target.kind === "model_grain" || target.kind === "entity_key") && <>{semanticSelect}{target.kind === "model_grain" ? textField("粒度表达式", "expression", true) : <label className="production-field"><span>唯一性</span><select aria-label="唯一性" value={"uniqueness" in content ? content.uniqueness : "exact"} onChange={(event) => patch({ uniqueness: event.target.value })}><option value="exact">严格唯一</option><option value="deduplicated">去重后唯一</option></select></label>}<div className="production-field"><span>{target.kind === "entity_key" ? "键字段" : "粒度字段"}</span><div className="production-field-choices">{members.filter((member) => member.kind === "field").map((member) => <label key={memberKey(member)}><input type="checkbox" checked={fields.some((field) => memberKey(field) === memberKey(member))} onChange={(event) => patch({ fields: event.target.checked ? [...fields, physicalReference(member.snapshotId, member)] : fields.filter((field) => memberKey(field) !== memberKey(member)) })} />{[members.find((parent) => parent.snapshotId === member.snapshotId && parent.objectId === member.parentObjectId)?.name, member.name].filter(Boolean).join(".")}</label>)}</div></div></>}
    {target.kind === "join_contract" && "pairs" in content && <><PhysicalSelect label="左侧数据集" value={content.leftDataset} members={members} kind="dataset" onChange={(value) => { if (value) patch({ leftDataset: value, pairs: [] }); }} /><PhysicalSelect label="右侧数据集" value={content.rightDataset} members={members} kind="dataset" onChange={(value) => { if (value) patch({ rightDataset: value, pairs: [] }); }} /><label className="production-field"><span>连接方式</span><select aria-label="连接方式" value={content.joinType} onChange={(event) => patch({ joinType: event.target.value })}>{["inner", "left", "right", "full"].map((value) => <option key={value}>{value}</option>)}</select></label><label className="production-field"><span>基数</span><select aria-label="基数" value={content.cardinality} onChange={(event) => patch({ cardinality: event.target.value })}>{["one_to_one", "one_to_many", "many_to_one", "many_to_many"].map((value) => <option key={value}>{value}</option>)}</select></label>{textField("连接表达式", "expression", true)}<div className="production-pairs">{content.pairs.map((pair, index) => <div key={index}><PhysicalSelect label={`左字段 ${index + 1}`} value={pair.left} members={members.filter((member) => member.parentObjectId === content.leftDataset.objectId)} kind="field" onChange={(value) => { if (value) patch({ pairs: content.pairs.map((item, i) => i === index ? { ...item, left: value } : item) }); }} /><PhysicalSelect label={`右字段 ${index + 1}`} value={pair.right} members={members.filter((member) => member.parentObjectId === content.rightDataset.objectId)} kind="field" onChange={(value) => { if (value) patch({ pairs: content.pairs.map((item, i) => i === index ? { ...item, right: value } : item) }); }} /><button className="icon-button" title="移除字段对" aria-label={`移除字段对 ${index + 1}`} onClick={() => patch({ pairs: content.pairs.filter((_, i) => i !== index) })}><X size={14} /></button></div>)}<button className="secondary-button" onClick={() => { const left = members.find((member) => member.kind === "field" && member.parentObjectId === content.leftDataset.objectId), right = members.find((member) => member.kind === "field" && member.parentObjectId === content.rightDataset.objectId); if (left && right) patch({ pairs: [...content.pairs, { left: physicalReference(left.snapshotId, left), right: physicalReference(right.snapshotId, right) }] }); }}><Plus size={14} />添加字段对</button></div>{textField("关系依据", "notes", true)}</>}
  </fieldset>;
}

function BusinessRuleConfirmation({ target, witness, disabled, onRecord }: { target: ProductionTarget; witness?: BusinessRuleWitness; disabled: boolean; onRecord: (key: string, statement?: string) => void }) {
  const [statement, setStatement] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const content = target.content as AssetContent;
  return <section className="production-rule" aria-label="业务规则确认"><h4>业务规则确认 <span>{witness?.valid ? "有效" : "未确认"}</span></h4>{witness && <p className="production-muted">{witness.declaration ?? witness.event.evidenceId ?? "确认已撤销"}<small>主体 {witness.event.principalId} · v{witness.event.productionVersion}</small></p>}{witness?.valid ? <button className="secondary-button" disabled={disabled} onClick={() => onRecord(target.localKey)}><RotateCcw size={14} />撤销确认</button> : <><label className="production-field"><span>业务规则声明</span><textarea aria-label="业务规则声明" disabled={disabled} value={statement} onChange={(event) => { setStatement(event.target.value); setConfirmed(false); }} /></label><label className="production-check"><input type="checkbox" aria-label="确认声明支持当前定义与范围" disabled={disabled || !statement.trim()} checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /><span>确认声明支持当前定义与范围</span></label><button className="secondary-button" disabled={disabled || !confirmed || !statement.trim() || !content.definition?.trim() || !content.scope?.trim()} onClick={() => { onRecord(target.localKey, statement.trim()); setConfirmed(false); }}><Check size={14} />记录业务确认</button></>}</section>;
}

export function ProductionValidationView({ detail, dirty, busy, canValidate, canReview, onValidate, onReview }: { detail: ProductionOperation; dirty: boolean; busy: boolean; canValidate: boolean; canReview: boolean; onValidate: (reason: string) => void; onReview: (decision: "approve" | "reject", note: string) => void }) {
  const [note, setNote] = useState("");
  const validation = detail.activeValidation;
  const result = validation.status === "not_requested" ? null : validation;
  const active = result?.status === "queued" || result?.status === "running";
  return <section className="production-validation" aria-label="集合验证与审核"><header><h3>冻结版本 <span>v{detail.version}</span></h3><span>{result ? ({ queued: "排队中", running: "验证中", succeeded: "验证通过", failed: "验证失败" } as const)[result.status] : "尚未验证"}</span></header>{dirty && <p role="status" className="production-alert">有未保存的纠正，当前验证与审核不能用于这份编辑内容。</p>}<code className="production-digest">{detail.setDigest}</code>{detail.unresolvedCodes.length > 0 && <p className="production-alert">{detail.unresolvedCodes.join(" · ")}</p>}<div className="production-checks">{result?.checks.map((check) => <article key={check.runId}><header><strong>{check.validatorId}</strong><span>{check.status === "succeeded" ? "通过" : check.status === "failed" ? "失败" : "进行中"}</span></header><small>{detail.targets.find((target) => target.proposalId === check.proposalId)?.declaration.title ?? check.proposalId} · {check.validatorVersion}</small>{check.results.map((finding, index) => <p key={index} className={finding.severity === "blocker" ? "production-blocker" : ""}><strong>{finding.code}</strong><span>{finding.message}</span></p>)}</article>)}</div><label className="production-field"><span>审核或重验说明</span><textarea aria-label="审核或重验说明" value={note} disabled={busy} onChange={(event) => setNote(event.target.value)} /></label><footer className="production-footer"><button className="secondary-button" disabled={dirty || busy || !canValidate || active || !detail.summary.frozen || !note.trim()} onClick={() => onValidate(note.trim())}><RefreshCw size={14} />重新验证</button><div className="production-actions"><button className="secondary-button" disabled={dirty || busy || !canReview || active || !result?.validationDigest || !note.trim()} onClick={() => onReview("reject", note.trim())}>拒绝集合</button><button className="primary-button" disabled={dirty || busy || !canReview || result?.status !== "succeeded" || !result.validationDigest || !note.trim()} onClick={() => onReview("approve", note.trim())}><ShieldCheck size={15} />批准整个集合</button></div></footer></section>;
}

function GenerationSection({ workspaceId, detail, generations, disabled, write, onApply }: { workspaceId: string; detail: ProductionOperation; generations: GenerationResult[]; disabled: boolean; write: ReturnType<typeof useSemanticProduction>["write"]; onApply: (run: GenerationResult) => void }) {
  const [models, setModels] = useState<GovernanceModelSetting[]>([]);
  const [modelId, setModelId] = useState("");
  const [instruction, setInstruction] = useState("");
  const [maxTokens, setMaxTokens] = useState(2048);
  const [maxCost, setMaxCost] = useState(1000000);
  const [error, setError] = useState("");
  useEffect(() => { const controller = new AbortController(); listModelProviders(workspaceId, controller.signal).then((providers) => { if (!controller.signal.aborted) { const values = providers.filter((provider) => provider.provider.enabled).flatMap((provider) => provider.models).filter((model) => model.enabled && model.kind === "llm"); setModels(values); setModelId(values.find((model) => model.isDefault)?.id ?? values[0]?.id ?? ""); } }).catch((reason) => { if (!controller.signal.aborted) setError(productionMessage(reason)); }); return () => controller.abort(); }, [workspaceId]);
  const model = models.find((value) => value.id === modelId);
  return <section className="production-generation" aria-label="AI 建模建议"><header><h3><Sparkles size={16} />AI 建模建议</h3><span className="production-muted">建议不等于业务确认</span></header>{error && <p role="alert">{error}</p>}<div className="production-generation-form"><label className="production-field"><span>生成模型</span><select aria-label="生成模型" disabled={disabled} value={modelId} onChange={(event) => setModelId(event.target.value)}><option value="">选择模型</option>{models.map((value) => <option key={value.id} value={value.id}>{value.model}</option>)}</select></label><label className="production-field"><span>输出 token 上限</span><input type="number" min={1} max={16384} aria-label="输出 token 上限" disabled={disabled} value={maxTokens} onChange={(event) => setMaxTokens(Number(event.target.value))} /></label><label className="production-field"><span>费用上限（微美元）</span><input type="number" min={0} max={1000000000} aria-label="费用上限（微美元）" disabled={disabled} value={maxCost} onChange={(event) => setMaxCost(Number(event.target.value))} /></label></div><label className="production-field"><span>建模要求</span><textarea aria-label="建模要求" disabled={disabled} value={instruction} onChange={(event) => setInstruction(event.target.value)} /></label><button className="secondary-button" disabled={disabled || !model?.generationConfigRevision || maxTokens < 1 || maxTokens > 16384 || maxCost < 0 || generations.some((run) => run.status === "queued" || run.status === "running")} onClick={() => void write("生成建议", (key) => productionAPI.generate(workspaceId, detail.summary.id, { expectedVersion: detail.version, inputDigest: detail.inputDigest, modelSettingId: model!.id, modelConfigRevision: model!.generationConfigRevision!, instruction, maxOutputTokens: maxTokens, maxCostMicros: maxCost }, key))}><Sparkles size={15} />生成结构化建议</button><div className="production-generation-runs">{generations.map((run) => <article key={run.runId}><header><strong>{run.model}</strong><span>{({ queued: "排队中", running: "生成中", succeeded: "建议已保存", failed: "生成失败", cancelled: "已取消", outcome_unknown: "调用结果未知" } as const)[run.status]}</span></header><p className="production-muted">{run.providerMode === "protocol_stub" ? "协议替身 · 合成验收响应" : "实际模型"} · 实际费用{run.costMicros === null ? "未知" : ` ${run.costMicros} 微美元`} · 输入 v{run.inputVersion}</p>{run.errorCode && <p role="alert">{run.errorCode}</p>}{run.status === "succeeded" && <><button className="secondary-button" disabled={disabled || run.inputVersion !== detail.version} onClick={() => onApply(run)}>应用到编辑区<ArrowRight size={14} /></button><details><summary>原始结构化建议</summary><pre>{JSON.stringify(run.output, null, 2)}</pre></details></>}</article>)}</div>{detail.generationApplications.length > 0 && <details><summary>AI 应用与人工纠正归因</summary><pre>{JSON.stringify(detail.generationApplications, null, 2)}</pre></details>}</section>;
}

export function MatchAssetDialog({ workspaceId, target, onClose, onMatch }: { workspaceId: string; target: ProductionTarget; onClose: () => void; onMatch: (target: ProductionTarget) => void }) {
  const [search, setSearch] = useState("");
  const [assets, setAssets] = useState<CatalogAsset[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const dialog = ref.current?.closest('[role="dialog"]');
    const trap = (event: Event) => {
      const key = event as KeyboardEvent;
      if (key.key !== "Tab" || !dialog) return;
      const controls = [...dialog.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], summary, [tabindex="0"]')].filter((node) => !node.hidden && node.getAttribute("aria-hidden") !== "true");
      const first = controls[0], last = controls.at(-1);
      if (key.shiftKey && document.activeElement === first) { key.preventDefault(); last?.focus(); }
      else if (!key.shiftKey && document.activeElement === last) { key.preventDefault(); first?.focus(); }
    };
    dialog?.addEventListener("keydown", trap);
    ref.current?.focus();
    return () => { dialog?.removeEventListener("keydown", trap); if (previous?.isConnected) previous.focus(); };
  }, []);
  useEffect(() => { const controller = new AbortController(); const timer = window.setTimeout(() => { listAssets(workspaceId, search, "", undefined, controller.signal).then((page) => { if (!controller.signal.aborted) { setAssets(page.items); setError(""); } }).catch((reason) => { if (!controller.signal.aborted) setError(productionMessage(reason)); }); }, 200); return () => { window.clearTimeout(timer); controller.abort(); }; }, [search, workspaceId]);
  const match = async (id: string) => {
    setBusy(true); setError("");
    try {
      const asset = await getAsset(workspaceId, id);
      const revision = asset.currentRevision;
      if (!revision) throw new Error("目标资产没有当前版本。");
      const content = revision.content as unknown as AssetContent;
      if (!content.address || !content.assetType || !("definition" in content) || !("scope" in content) || !content.ownerPrincipalId) throw new Error("目标资产不是可复用的结构化生产内容，不能自动猜测基础定义。");
      onMatch({ intent: "update", kind: "semantic_asset", localKey: target.localKey, title: target.title, targetId: id, baseRevisionId: revision.id, content: structuredClone(content), changes: [], evidenceIds: target.evidenceIds });
    } catch (reason) { setError(productionMessage(reason)); } finally { setBusy(false); }
  };
  return <div className="dialog-backdrop"><section className="review-dialog production-match-dialog" role="dialog" aria-modal="true" aria-label="匹配已有语义资产" onKeyDown={(event) => { if (event.key === "Escape" && !busy) onClose(); }}><header><h2>匹配已有语义资产</h2><button className="icon-button" aria-label="关闭匹配" disabled={busy} onClick={onClose}><X size={16} /></button></header><div className="dialog-body"><label className="production-field"><span>搜索资产</span><input ref={ref} aria-label="搜索匹配资产" value={search} onChange={(event) => setSearch(event.target.value)} /></label>{error && <p role="alert">{error}</p>}{assets.map((asset) => <button className="production-record" disabled={busy} key={asset.id} onClick={() => void match(asset.id)}><span><strong>{asset.title}</strong><small>{asset.address}</small></span><ChevronRight size={15} /></button>)}</div></section></div>;
}
