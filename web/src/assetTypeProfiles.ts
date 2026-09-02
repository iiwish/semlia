import type { Asset, AssetType } from "./types";

export type AssetImplementationMode = "required" | "optional" | "not_applicable";
export type AssetRuleSeverity = "blocker" | "warning";
export type AssetRuleState = "passed" | "warning" | "failed" | "not_applicable";
export type AssetEmptyStateKind = "missing" | "optional" | "not_applicable";

export interface AssetEmptyStateConfig {
  kind: AssetEmptyStateKind;
  title: string;
  detail: string;
  action?: string;
}

interface AssetTypeRule {
  id: string;
  label: string;
  severity: AssetRuleSeverity;
  description: string;
  remediation: string;
  evaluate: (asset: Asset) => { state: AssetRuleState; detail: string };
}

export interface AssetTypeValidationResult extends Omit<AssetTypeRule, "evaluate"> {
  state: AssetRuleState;
  detail: string;
}

export interface AssetTypeProfile {
  type: AssetType;
  definitionTitle: string;
  definitionFocus: string;
  ontologyFocus: string;
  implementationFocus: string;
  trustFocus: string;
  deliveryFocus: string;
  requiredFields: string[];
  implementationMode: AssetImplementationMode;
  emptyStates: {
    relations: AssetEmptyStateConfig;
    bindings: AssetEmptyStateConfig;
    joins: AssetEmptyStateConfig;
    evidence: AssetEmptyStateConfig;
    consumers: AssetEmptyStateConfig;
  };
  validationRules: AssetTypeRule[];
}

function evaluatedRule(
  id: string,
  label: string,
  severity: AssetRuleSeverity,
  description: string,
  remediation: string,
  test: (asset: Asset) => boolean,
  passedDetail: string,
  failedDetail: string,
): AssetTypeRule {
  return {
    id,
    label,
    severity,
    description,
    remediation,
    evaluate: (asset) => {
      const passed = test(asset);
      return {
        state: passed ? "passed" : severity === "blocker" ? "failed" : "warning",
        detail: passed ? passedDetail : failedDetail,
      };
    },
  };
}

const hasOwner = (asset: Asset) => asset.owner.trim().length > 0 && asset.owner !== "待指定";
const hasDefinition = (asset: Asset) => asset.revisionRecord.definition.trim().length >= 12;
const hasValidatedBinding = (asset: Asset) => asset.bindings.some((binding) => binding.state === "已验证");
const hasPublishedJoin = (asset: Asset) => asset.joinContracts.length === 0 || asset.joinContracts.every((contract) => contract.state === "已发布");
const hasNoFailedRun = (asset: Asset) => asset.validationRuns.every((run) => run.state !== "failed");
const hasEvidence = (asset: Asset) => asset.evidenceLinks.length > 0 && asset.claims.length > 0;

const profiles: Record<AssetType, AssetTypeProfile> = {
  业务概念: {
    type: "业务概念",
    definitionTitle: "术语与业务边界",
    definitionFocus: "固定规范术语、概念分类、消歧规则、正反例和可回答问题。",
    ontologyFocus: "检查上位、下位、相关、同义及替代概念，避免循环和跨域歧义。",
    implementationFocus: "纯业务概念不要求物理实现；出现绑定后才进入执行治理。",
    trustFocus: "以业务声明、术语来源、冲突处理和本体一致性证明含义可信。",
    deliveryFocus: "通过 LLM Wiki、稳定 key 和语义检索上下文向人和 Agent 交付。",
    requiredFields: ["规范定义", "概念分类", "别名与消歧", "正反例", "业务 owner"],
    implementationMode: "not_applicable",
    emptyStates: {
      relations: { kind: "missing", title: "尚未接入领域本体", detail: "补充上位、相关或同义概念后，解析器才能稳定处理跨域含义。", action: "建立本体关系" },
      bindings: { kind: "not_applicable", title: "该概念不需要物理实现", detail: "当前资产只表达受治理含义，不参与数据查询和数值执行。" },
      joins: { kind: "not_applicable", title: "业务概念不需要 JoinContract", detail: "只有进入可执行模型或绑定后才需要生产连接契约。" },
      evidence: { kind: "missing", title: "缺少概念权威来源", detail: "至少关联一项业务声明、术语规范或经确认的领域文档。", action: "补充权威来源" },
      consumers: { kind: "optional", title: "尚无已注册使用方", detail: "概念仍可通过知识检索使用；注册绑定后可以跟踪版本兼容性。", action: "注册使用方" },
    },
    validationRules: [
      evaluatedRule("CONCEPT-001", "定义与责任完整", "blocker", "规范定义和业务责任必须明确。", "补充规范定义并指定业务 owner。", (asset) => hasDefinition(asset) && hasOwner(asset), "定义与 owner 已满足发布要求", "定义过短或业务 owner 未指定"),
      evaluatedRule("CONCEPT-002", "别名可消歧", "blocker", "同领域别名必须能够稳定解析。", "补充别名、语义域和消歧规则。", (asset) => asset.aliases.length > 0 && asset.wikiContext.disambiguationRules.length > 0, "别名与消歧规则已声明", "缺少别名或消歧规则"),
      evaluatedRule("CONCEPT-003", "本体一致性", "blocker", "概念层级和关系不能存在未解决冲突。", "修复循环、同义冲突或非法关系类型。", (asset) => asset.ontologyContext.consistencyState === "consistent", "本体 revision 一致性通过", "本体存在待确认关系"),
      evaluatedRule("CONCEPT-004", "解释样例覆盖", "warning", "正反例和典型问题提高人机理解一致性。", "补充正例、反例和典型问题。", (asset) => asset.examples.length > 0 && asset.includes.length > 0 && asset.excludes.length > 0, "边界与典型问题已覆盖", "正反例或典型问题覆盖不足"),
    ],
  },
  业务实体: {
    type: "业务实体",
    definitionTitle: "身份与生命周期契约",
    definitionFocus: "固定业务键、身份合并策略、实体粒度、生命周期和有效时间。",
    ontologyFocus: "检查实体关系、被描述关系，以及模型、维度和分群的引用。",
    implementationFocus: "治理主绑定、实体键、去重策略、实体解析和安全 JoinContract。",
    trustFocus: "以键唯一性、身份合并、生命周期和连接基数验证实体稳定性。",
    deliveryFocus: "通过稳定实体身份向模型、Agent 和下游应用提供版本化解析。",
    requiredFields: ["业务键", "身份策略", "实体粒度", "生命周期", "主绑定"],
    implementationMode: "required",
    emptyStates: {
      relations: { kind: "missing", title: "尚未建立实体关系", detail: "补充实体间关系或被哪些模型、维度描述，才能形成完整领域上下文。", action: "建立实体关系" },
      bindings: { kind: "missing", title: "实体还不能被解析", detail: "需要绑定稳定业务键和来源 revision，才能用于生产连接。", action: "创建实体绑定" },
      joins: { kind: "missing", title: "尚未声明生产连接", detail: "至少为主要事实模型声明键、基数、粒度影响和扇出防护。", action: "创建 JoinContract" },
      evidence: { kind: "missing", title: "缺少身份治理证据", detail: "需要唯一性、空值、合并规则或主数据声明证据。", action: "补充身份验证" },
      consumers: { kind: "optional", title: "尚无实体消费者", detail: "实体可以先发布；建立消费者绑定后跟踪身份兼容性。", action: "创建消费者绑定" },
    },
    validationRules: [
      evaluatedRule("ENTITY-001", "实体身份可解析", "blocker", "实体必须声明业务键并具有已验证主绑定。", "补充实体键、主绑定和唯一性检查。", (asset) => asset.revisionRecord.typeSpec.kind === "entity" && asset.revisionRecord.typeSpec.entityKeys.length > 0 && hasValidatedBinding(asset), "实体键与主绑定已验证", "实体键缺失或主绑定未验证"),
      evaluatedRule("ENTITY-002", "身份策略完整", "blocker", "身份合并、去重和生命周期必须可解释。", "补充 identity policy 与生命周期规则。", (asset) => asset.revisionRecord.typeSpec.kind === "entity" && asset.revisionRecord.typeSpec.identityPolicy.length > 0 && asset.revisionRecord.typeSpec.lifecycle.length > 0, "身份策略与生命周期已声明", "身份策略或生命周期缺失"),
      evaluatedRule("ENTITY-003", "连接基数安全", "blocker", "实体加入事实模型时必须保持声明粒度。", "补充或修复 JoinContract 的键、基数和防护。", hasPublishedJoin, "JoinContract 均已发布", "存在未发布或需关注的 JoinContract"),
      evaluatedRule("ENTITY-004", "实体关系覆盖", "warning", "实体应连接描述维度、模型或业务关系。", "补充直接实体关系和领域说明。", (asset) => asset.relations.length > 0, "实体已连接领域关系", "尚无可审计实体关系"),
    ],
  },
  语义模型: {
    type: "语义模型",
    definitionTitle: "模型成员与查询边界",
    definitionFocus: "固定基础实体、模型粒度、公开成员、默认时间和查询边界。",
    ontologyFocus: "检查模型包含、依赖、成员归属和跨模型可达关系。",
    implementationFocus: "治理成员映射、默认 Join 路径、适配器、过滤策略与扇出防护。",
    trustFocus: "以成员解析、模型编译、Join 安全和查询回归证明可执行性。",
    deliveryFocus: "作为公开查询表面向问答、BI、API 和 Agent 提供稳定成员集合。",
    requiredFields: ["基础实体", "模型粒度", "公开成员", "默认时间", "Join 路径策略"],
    implementationMode: "required",
    emptyStates: {
      relations: { kind: "missing", title: "模型尚无公开成员", detail: "模型必须连接至少一个实体、度量、维度或指标。", action: "添加公开成员" },
      bindings: { kind: "missing", title: "模型尚未绑定执行表面", detail: "补充数据集、模型粒度和执行适配器后才能编译。", action: "创建模型绑定" },
      joins: { kind: "optional", title: "模型不跨实体连接", detail: "当前模型在单一基础实体内执行，因此不需要 JoinContract。" },
      evidence: { kind: "missing", title: "缺少模型编译证据", detail: "至少运行成员解析、编译和 Join 路径验证。", action: "运行模型验证" },
      consumers: { kind: "optional", title: "尚无模型消费者", detail: "模型可先发布；消费者绑定会固定公开成员和版本约束。", action: "创建消费者绑定" },
    },
    validationRules: [
      evaluatedRule("MODEL-001", "基础实体与成员完整", "blocker", "模型必须声明基础实体和至少一个公开成员。", "补充 primary entity 与 public members。", (asset) => asset.revisionRecord.typeSpec.kind === "model" && asset.revisionRecord.typeSpec.primaryEntity.length > 0 && asset.revisionRecord.typeSpec.publicMembers.length > 0, "基础实体与公开成员已声明", "基础实体或公开成员缺失"),
      evaluatedRule("MODEL-002", "执行表面可编译", "blocker", "模型必须绑定可解析的数据集和适配器。", "创建并验证 PhysicalBinding。", hasValidatedBinding, "模型绑定已验证", "没有已验证模型绑定"),
      evaluatedRule("MODEL-003", "Join 路径受控", "blocker", "跨实体路径必须由已发布 JoinContract 约束。", "修复默认 Join 路径或发布连接契约。", hasPublishedJoin, "默认 Join 路径受已发布契约保护", "存在未发布连接契约"),
      evaluatedRule("MODEL-004", "默认时间明确", "warning", "公开查询模型应提供默认时间语义。", "声明默认时间或明确标记不适用。", (asset) => asset.defaultTime.trim().length > 0, "默认时间已声明", "默认时间尚未声明"),
    ],
  },
  维度: {
    type: "维度",
    definitionTitle: "值域、层级与空值语义",
    definitionFocus: "固定值类型、值域、编码、层级、空值含义、共享范围和有效时间。",
    ontologyFocus: "检查维度描述对象、父子层级、共享关系和被哪些模型使用。",
    implementationFocus: "治理字段映射、代码表、层级键、编码转换和时间版本策略。",
    trustFocus: "以编码唯一性、层级无环、值域覆盖和空值规则证明描述稳定。",
    deliveryFocus: "向模型和消费者提供可复用的筛选、分组、显示和层级能力。",
    requiredFields: ["值类型", "空值语义", "值域或开放声明", "描述对象", "字段绑定"],
    implementationMode: "required",
    emptyStates: {
      relations: { kind: "missing", title: "维度尚未描述任何对象", detail: "至少关联一个实体或语义模型，避免形成孤立枚举。", action: "关联描述对象" },
      bindings: { kind: "missing", title: "维度没有字段映射", detail: "补充来源字段、编码规则和值域验证后才能生产使用。", action: "创建维度绑定" },
      joins: { kind: "optional", title: "维度不参与跨模型连接", detail: "当前维度在所属模型内直接可用，不需要额外 JoinContract。" },
      evidence: { kind: "missing", title: "缺少值域治理证据", detail: "需要值类型、编码唯一性、层级或业务声明证据。", action: "验证值域" },
      consumers: { kind: "optional", title: "尚无直接消费者", detail: "维度可能经由语义模型间接消费；注册直接绑定后跟踪兼容性。", action: "注册使用方" },
    },
    validationRules: [
      evaluatedRule("DIMENSION-001", "责任与值类型明确", "blocker", "共享维度必须有业务 owner 和明确值类型。", "指定 owner 并声明 value type。", (asset) => hasOwner(asset) && asset.revisionRecord.typeSpec.kind === "dimension" && asset.revisionRecord.typeSpec.valueType.length > 0, "owner 与值类型已声明", "owner 未指定或值类型缺失"),
      evaluatedRule("DIMENSION-002", "字段映射有效", "blocker", "维度必须具有已验证字段绑定。", "修复字段映射、编码重复或来源 revision。", hasValidatedBinding, "维度字段绑定已验证", "没有已验证字段绑定"),
      evaluatedRule("DIMENSION-003", "层级与值域可解释", "warning", "层级或开放值域必须有明确声明。", "补充 hierarchy、值域或开放值域说明。", (asset) => asset.revisionRecord.typeSpec.kind === "dimension" && asset.revisionRecord.typeSpec.hierarchy.length > 0, "层级或值域范围已声明", "层级与值域范围均不明确"),
      evaluatedRule("DIMENSION-004", "空值语义明确", "blocker", "未知、缺失和不适用不能被混为一个成员。", "声明 null semantics 和默认显示策略。", (asset) => asset.revisionRecord.typeSpec.kind === "dimension" && asset.revisionRecord.typeSpec.nullSemantics.length > 0, "空值语义已声明", "空值语义缺失"),
    ],
  },
  度量: {
    type: "度量",
    definitionTitle: "聚合与可加性契约",
    definitionFocus: "固定基础表达式、聚合、单位、可加性、粒度、默认时间和空值处理。",
    ontologyFocus: "检查所属模型、描述粒度、组成指标和派生度量。",
    implementationFocus: "治理来源字段、数值转换、过滤、聚合和时间绑定。",
    trustFocus: "以表达式编译、数值类型、聚合行为和边界样本证明计算稳定。",
    deliveryFocus: "作为可复用基础事实向指标、模型和查询规划器提供受控计算。",
    requiredFields: ["数值表达式", "聚合函数", "单位", "可加性", "粒度与时间"],
    implementationMode: "required",
    emptyStates: {
      relations: { kind: "missing", title: "度量尚未归属语义模型", detail: "关联所属模型或组成指标，才能解释该数值在哪个业务上下文中成立。", action: "关联语义模型" },
      bindings: { kind: "missing", title: "度量还不能执行", detail: "补充数值表达式、来源字段、粒度和适配器。", action: "创建度量绑定" },
      joins: { kind: "optional", title: "度量在单一模型内计算", detail: "当前表达式不跨模型连接，因此不需要 JoinContract。" },
      evidence: { kind: "missing", title: "缺少计算验证证据", detail: "至少提供表达式编译、类型检查或结果样本。", action: "运行度量验证" },
      consumers: { kind: "optional", title: "尚无直接消费者", detail: "度量通常由指标间接消费；直接绑定不是发布前置条件。" },
    },
    validationRules: [
      evaluatedRule("MEASURE-001", "数值契约完整", "blocker", "表达式、聚合和单位必须同时明确。", "补充 expression、aggregation 和 unit。", (asset) => asset.formula.length > 0 && asset.aggregation.length > 0 && asset.unit.length > 0, "表达式、聚合与单位完整", "数值契约字段缺失"),
      evaluatedRule("MEASURE-002", "物理表达式可执行", "blocker", "度量必须具有已验证的数值绑定。", "创建或修复 PhysicalBinding。", hasValidatedBinding, "度量绑定已验证", "没有已验证度量绑定"),
      evaluatedRule("MEASURE-003", "可加性与空值明确", "blocker", "聚合行为必须解释跨时间与跨实体的可加性。", "补充 additivity 和 null handling。", (asset) => asset.revisionRecord.typeSpec.kind === "measure" && asset.revisionRecord.typeSpec.nullHandling.length > 0, "可加性与空值规则已声明", "可加性或空值规则缺失"),
      evaluatedRule("MEASURE-004", "计算证据覆盖", "warning", "高复用度量应具有编译或结果证据。", "运行编译、类型或回归验证。", hasEvidence, "度量具有字段级证据", "尚无字段级计算证据"),
    ],
  },
  指标: {
    type: "指标",
    definitionTitle: "业务衡量与比较契约",
    definitionFocus: "固定业务目标、组成度量、公式、时间窗口、允许维度、比较语义和格式。",
    ontologyFocus: "检查组成度量、依赖指标、允许维度、业务目标和下游影响。",
    implementationFocus: "治理可执行表达式、时间字段、结果粒度、依赖解析和安全 Join。",
    trustFocus: "以依赖完整性、公式编译、结果回归和消费者兼容性证明业务衡量可信。",
    deliveryFocus: "通过稳定 key、release 和消费者 Binding 向问答、BI、API 与 Agent 交付。",
    requiredFields: ["业务定义", "指标公式", "时间窗口", "允许维度", "比较语义"],
    implementationMode: "required",
    emptyStates: {
      relations: { kind: "missing", title: "指标尚无组成关系", detail: "至少关联组成度量、依赖指标或允许维度，避免成为不可解释公式。", action: "建立指标依赖" },
      bindings: { kind: "missing", title: "指标还不能执行", detail: "补充表达式、结果粒度、时间字段和执行适配器。", action: "创建指标绑定" },
      joins: { kind: "optional", title: "指标不需要跨模型连接", detail: "当前公式的全部依赖可在同一模型内安全解析。" },
      evidence: { kind: "missing", title: "缺少指标可信证据", detail: "至少提供 owner 声明、公式编译或结果回归证据。", action: "补充指标证据" },
      consumers: { kind: "optional", title: "尚无生产消费者", detail: "指标可以先发布；绑定消费者后跟踪版本兼容和真实解析活动。", action: "创建消费者绑定" },
    },
    validationRules: [
      evaluatedRule("METRIC-001", "公式依赖可解析", "blocker", "指标必须有公式，并关联组成度量或语义模型。", "补充公式和类型化依赖关系。", (asset) => asset.formula.length > 0 && asset.relations.length > 0, "公式与依赖关系完整", "公式或组成依赖缺失"),
      evaluatedRule("METRIC-002", "时间与维度兼容", "blocker", "时间语义和允许维度必须明确。", "声明默认时间、允许维度和比较语义。", (asset) => asset.defaultTime.length > 0 && asset.revisionRecord.typeSpec.kind === "metric" && asset.revisionRecord.typeSpec.allowedDimensions.length > 0, "时间与维度范围已声明", "时间或允许维度缺失"),
      evaluatedRule("METRIC-003", "指标实现可执行", "blocker", "生产指标必须具有已验证绑定。", "创建或修复指标 PhysicalBinding。", hasValidatedBinding, "指标绑定已验证", "没有已验证指标绑定"),
      evaluatedRule("METRIC-004", "确定性验证无失败", "blocker", "发布不能包含失败的编译、回归或兼容性验证。", "修复失败运行并使用相同 validator 版本重跑。", hasNoFailedRun, "当前验证运行没有失败项", "存在失败验证运行"),
    ],
  },
  分群: {
    type: "分群",
    definitionTitle: "成员判定与有效时间",
    definitionFocus: "固定基础实体、成员条件、包含排除、有效时间、刷新策略和互斥关系。",
    ontologyFocus: "检查过滤依据、依赖指标、包含分群、互斥分群和下游用途。",
    implementationFocus: "治理布尔表达式、实体绑定、刷新任务、物化策略和成员变化。",
    trustFocus: "以布尔确定性、实体粒度、刷新新鲜度、规模基线和漂移验证证明成员资格。",
    deliveryFocus: "向运营、营销和 Agent 提供带有效时间与版本约束的成员判定。",
    requiredFields: ["基础实体", "布尔规则", "有效时间", "刷新策略", "包含与排除"],
    implementationMode: "required",
    emptyStates: {
      relations: { kind: "missing", title: "分群尚未关联基础实体", detail: "必须连接基础实体和判定依据，才能解释成员属于谁以及为什么入群。", action: "关联基础实体" },
      bindings: { kind: "missing", title: "分群规则还不能执行", detail: "补充布尔表达式、实体键、快照时间和刷新方式。", action: "创建分群绑定" },
      joins: { kind: "optional", title: "分群在基础实体内判定", detail: "当前规则不跨实体扩张成员，因此不需要 JoinContract。" },
      evidence: { kind: "missing", title: "缺少阈值与稳定性证据", detail: "需要业务声明、规模基线或成员漂移验证。", action: "补充分群证据" },
      consumers: { kind: "optional", title: "尚无分群消费者", detail: "候选分群可以在发布前为空；发布后应注册用途与版本约束。", action: "注册分群用途" },
    },
    validationRules: [
      evaluatedRule("SEGMENT-001", "基础实体与规则完整", "blocker", "分群必须声明基础实体并返回确定布尔结果。", "补充 base entity 和布尔表达式。", (asset) => asset.revisionRecord.typeSpec.kind === "segment" && asset.revisionRecord.typeSpec.baseEntity.length > 0 && asset.formula.length > 0, "基础实体与成员条件已声明", "基础实体或成员条件缺失"),
      evaluatedRule("SEGMENT-002", "实体粒度可执行", "blocker", "成员规则必须绑定稳定实体键和快照时间。", "修复 PhysicalBinding、实体键或时间字段。", hasValidatedBinding, "分群绑定已验证", "没有已验证分群绑定"),
      evaluatedRule("SEGMENT-003", "有效时间与刷新明确", "blocker", "分群必须声明成员资格何时生效及如何刷新。", "补充 effective time 与 refresh policy。", (asset) => asset.revisionRecord.typeSpec.kind === "segment" && asset.revisionRecord.typeSpec.effectiveTime.length > 0 && asset.revisionRecord.typeSpec.refreshPolicy.length > 0, "有效时间与刷新策略已声明", "有效时间或刷新策略缺失"),
      evaluatedRule("SEGMENT-004", "阈值具有业务证据", "warning", "仅有观察数据不足以成为稳定业务阈值。", "补充 DECLARED 或 CONSTRAINED 权威证据。", (asset) => asset.evidence.some((evidence) => evidence.authority === "DECLARED" || evidence.authority === "CONSTRAINED"), "阈值具有声明或约束证据", "当前阈值只有观察或推断证据"),
    ],
  },
};

export function assetTypeProfileFor(asset: Asset): AssetTypeProfile {
  return profiles[asset.type];
}

export function evaluateAssetTypeRules(asset: Asset): AssetTypeValidationResult[] {
  return assetTypeProfileFor(asset).validationRules.map(({ evaluate, ...rule }) => ({ ...rule, ...evaluate(asset) }));
}

export const assetTypeProfiles = profiles;
