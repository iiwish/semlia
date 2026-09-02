import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { CatalogRuntimeProvider } from "./catalogRuntime";
import { ProductApp } from "./ProductApp";
import { assetTypeProfiles, evaluateAssetTypeRules } from "./assetTypeProfiles";
import { assets } from "./data";

function App() {
  return <CatalogRuntimeProvider fixtureAssets={assets}><ProductApp /></CatalogRuntimeProvider>;
}

describe("Semlia product workspace", () => {
  it("uses a three-layer semantic operations workspace", () => {
    render(<App />);

    const primaryNavigation = screen.getByLabelText("Semlia 主功能");
    expect(primaryNavigation).toBeInTheDocument();
    expect(within(primaryNavigation).getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["语义问答", "工作台", "知识资产", "变更与发布", "数据接入"]);
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(context).toBeInTheDocument();
    expect(within(context).queryByText("语义问答", { exact: true })).not.toBeInTheDocument();
    expect(within(context).queryByText("示例工作区")).not.toBeInTheDocument();
    expect(within(context).queryByText("MX", { exact: true })).not.toBeInTheDocument();
    const contextHeader = context.querySelector(".context-panel-header");
    const contextSearch = within(context).getByRole("searchbox", { name: "搜索会话" });
    expect(contextHeader).toHaveTextContent("最近会话");
    expect(context.firstElementChild).toBe(contextHeader);
    expect(contextHeader?.nextElementSibling).toBe(contextSearch.closest(".context-search"));
    expect(screen.getByRole("main")).toHaveClass("workspace-canvas");
  });

  it("searches recent conversations directly from the ask context", async () => {
    const user = userEvent.setup();
    render(<App />);

    const context = screen.getByRole("complementary", { name: "治理上下文" });
    const conversationSearch = within(context).getByRole("searchbox", { name: "搜索会话" });
    await user.keyboard("{Control>}k{/Control}");
    expect(conversationSearch).toHaveFocus();

    await user.type(conversationSearch, "客单价");
    expect(within(context).getByRole("button", { name: /客单价口径/ })).toBeVisible();
    expect(within(context).queryByRole("button", { name: /8 月收入诊断/ })).not.toBeInTheDocument();

    await user.clear(conversationSearch);
    await user.type(conversationSearch, "不存在的会话");
    expect(within(context).getByText("没有匹配会话")).toBeVisible();
    await user.click(within(context).getByRole("button", { name: "清除会话搜索" }));
    expect(within(context).getByRole("button", { name: /8 月收入诊断/ })).toBeVisible();
  });

  it("collapses, reopens and resizes the secondary menu while keeping a single-level page title", async () => {
    const user = userEvent.setup();
    render(<App />);

    const initialContext = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(initialContext).getByRole("button", { name: "收起二级菜单" }));
    expect(screen.queryByRole("complementary", { name: "治理上下文" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "数据接入" }));
    const reopenedContext = screen.getByRole("complementary", { name: "治理上下文" });
    expect(within(reopenedContext).getByText("数据接入", { exact: true })).toBeVisible();
    expect(within(reopenedContext).queryByText(/页面/)).not.toBeInTheDocument();
    expect(document.querySelector(".workspace-breadcrumb")).toHaveTextContent(/^数据来源$/);

    const resizeHandle = within(reopenedContext).getByRole("separator", { name: "调整二级菜单宽度" });
    resizeHandle.focus();
    await user.keyboard("{End}");
    expect(resizeHandle).toHaveAttribute("aria-valuenow", "440");
    expect(document.querySelector(".app-shell")).toHaveStyle("--context-panel-width: 440px");
  });

  it("opens on a traceable semantic answer instead of low-frequency setup", async () => {
    const user = userEvent.setup();
    render(<App />);

    expect(screen.queryByText("原型 · 模拟数据")).not.toBeInTheDocument();
    expect(screen.queryByText(/稳定版 ·/)).not.toBeInTheDocument();
    expect(screen.queryByText("可信语义问答")).not.toBeInTheDocument();
    expect(screen.queryByText("知识索引就绪")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /编辑会话标题：8 月收入诊断/ }));
    const titleInput = screen.getByRole("textbox", { name: "编辑会话标题" });
    await user.clear(titleInput);
    await user.type(titleInput, "华东收入复盘");
    await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: /编辑会话标题：华东收入复盘/ })).toBeVisible();
    expect(screen.queryByRole("region", { name: "数据库到 LLM 问答链路" })).not.toBeInTheDocument();
    expect(screen.getByText("净收入确认口径")).toBeVisible();
    expect(screen.queryByText("Semlia 检索已发布知识块和语义资产，委托 Cube 执行查询，并保留答案、口径与来源之间的完整链路。")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /连接数据/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: "回答执行与证据" })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "向 Semlia 提问" })).toBeVisible();
  });

  it("opens evidence in the owning asset instead of a pipeline-stage page", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: /净收入确认口径.*KB-2048/ }));
    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
    expect(screen.getByRole("tab", { name: "可信度" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Cube schema 编译通过")).toBeVisible();
    expect(within(screen.getByRole("complementary", { name: "治理上下文" })).queryByRole("button", { name: /知识块与证据/ })).not.toBeInTheDocument();
  });

  it("turns an incorrect answer into a field-level knowledge revision", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "指出问题" }));
    const issuePanel = screen.getByRole("region", { name: "指出回答中的知识问题" });
    await user.click(within(issuePanel).getByRole("radio", { name: /计算规则或实现不准确/ }));
    await user.click(within(issuePanel).getByRole("button", { name: "修订相关知识" }));

    const workbench = screen.getByRole("region", { name: "净收入 知识修订工作台" });
    expect(within(workbench).getByText("已发布版本受保护")).toBeVisible();
    expect(within(workbench).getByRole("complementary", { name: "证据与检查" })).toHaveTextContent("来源 revision");
    const expression = within(workbench).getByRole("textbox", { name: "计算表达式候选值" });
    await user.clear(expression);
    await user.type(expression, "SUM(paid_amount - confirmed_refund_amount - discount_amount - tax_amount)");
    expect(within(workbench).getByRole<HTMLTextAreaElement>("textbox", { name: "知识修订原因" }).value).toContain("来自问答");

    await user.click(within(workbench).getByRole("button", { name: "运行检查" }));
    expect(within(workbench).getByText("3/3 完成")).toBeVisible();
    await user.click(within(workbench).getByRole("button", { name: "提交审核" }));

    expect(screen.getByRole("region", { name: "净收入 @13 候选资产版本详情" })).toBeVisible();
    expect(screen.getByText("spec.expression")).toBeVisible();
    expect(screen.getByText(/confirmed_refund_amount/)).toBeVisible();
    await user.click(screen.getByRole("tab", { name: /变更来源.*1/ }));
    expect(screen.getByText("人工知识修订")).toBeVisible();
  });

  it("filters the semantic asset catalog and opens an explainable asset contract", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    expect(screen.getByRole("region", { name: "知识目录" })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "知识目录" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "保存视图" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "语义资产详情" })).not.toBeInTheDocument();
    await user.type(screen.getByRole("searchbox", { name: "搜索知识目录" }), "净收入");

    const catalog = screen.getByRole("region", { name: "知识目录" });
    expect(within(catalog).getByText("净收入")).toBeVisible();
    expect(within(catalog).queryByText("活跃客户")).not.toBeInTheDocument();

    await user.click(within(catalog).getByRole("button", { name: "打开语义资产 净收入" }));
    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
    const detail = screen.getByRole("region", { name: "语义资产详情" });
    const backToCatalog = screen.getByRole("button", { name: "返回知识目录" });
    expect(backToCatalog).toBeVisible();
    expect(backToCatalog.closest(".topbar")).toBeTruthy();
    expect(detail.querySelector(".asset-detail-titlebar")).toBeNull();
    expect(detail.querySelector(".asset-detail-commandbar")).not.toBeInTheDocument();
    expect(within(detail).getByRole("tab", { name: "概览" })).toHaveAttribute("aria-selected", "true");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("当前 revision 可用于生产");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("没有阻断门禁");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("质量阻断0");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("类型门禁0");
    expect(within(detail).getByText("健康", { selector: ".asset-health-state" })).toBeVisible();
    expect(within(detail).getByRole("region", { name: "指标摘要" })).toHaveTextContent("paid_amount");
    expect(detail.querySelector(".asset-overview-paths")).not.toBeInTheDocument();
    expect(within(detail).getByText("@12", { selector: ".asset-header-state strong" })).toBeVisible();
    expect(detail).toHaveTextContent("release-2026.08.3");
    expect(detail).not.toHaveTextContent("/ 100");

    const revisionLauncher = within(detail).getByRole("button", { name: "修订知识" });
    expect(revisionLauncher).toBeVisible();
    await user.click(revisionLauncher);
    const revisionPicker = within(detail).getByRole("dialog", { name: "选择知识修订对象" });
    expect(within(revisionPicker).getByText("来源资料与已发布版本保持只读")).toBeVisible();
    await user.click(within(revisionPicker).getByRole("button", { name: /计算表达式/ }));
    expect(within(detail).getByRole("region", { name: "净收入 知识修订工作台" })).toBeVisible();
    expect(within(detail).getByRole("textbox", { name: "计算表达式候选值" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "取消修订" }));
    expect(within(detail).getByRole("button", { name: "修订知识" })).toBeVisible();

    expect(within(detail).getAllByRole("tab")).toHaveLength(6);
    await user.click(within(detail).getByRole("tab", { name: "定义" }));
    expect(within(detail).getByRole("region", { name: "指标内容模板" })).toHaveTextContent("业务衡量与比较契约");
    expect(within(detail).getByRole("region", { name: "指标内容模板" })).toHaveTextContent("指标公式");
    expect(within(detail).getByRole("heading", { name: "权威语义页" })).toBeVisible();
    const wiki = within(detail).getByRole("region", { name: "权威语义页" });
    expect(within(wiki).getByText("AI 检索词")).toBeVisible();
    expect(within(wiki).getByText("commerce.net_revenue", { exact: false })).toBeVisible();
    expect(within(wiki).getByText("跨语义域出现同名含义", { exact: false })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "提出修订" }));
    expect(within(detail).getByRole("region", { name: "净收入 知识修订工作台" })).toBeVisible();
    expect(within(detail).getByText("当前发布值 · @12")).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "取消修订" }));

    await user.click(within(detail).getByRole("tab", { name: "本体关系" }));
    expect(within(detail).getByRole("tab", { name: "本体关系" })).toHaveAttribute("aria-selected", "true");
    expect(within(detail).getByRole("heading", { name: "电商经营本体" })).toBeVisible();
    expect(within(detail).getByRole("button", { name: "语义关系" })).toHaveAttribute("aria-pressed", "true");
    expect(within(detail).getByRole("img", { name: "净收入 的语义关系图" })).toBeVisible();
    expect(within(detail).getByText("一致性通过")).toBeVisible();
    expect(within(detail).getByRole("table", { name: "本体关系类型约束" })).toHaveTextContent("端点类型、方向与基数");
    expect(within(detail).getByText("commerce.orders_model")).toBeVisible();
    expect(within(detail).getAllByText("证据推导", { exact: false }).length).toBeGreaterThan(0);
    await user.click(within(detail).getByRole("button", { name: "概念层级" }));
    expect(within(detail).getByRole("img", { name: "净收入 的本体层级图" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "依赖与影响" }));
    expect(within(detail).getByRole("img", { name: "净收入 的依赖与影响图" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "提出关系修订" }));
    expect(within(detail).getByRole("region", { name: "净收入 知识修订工作台" })).toBeVisible();
    expect(within(detail).getByRole("textbox", { name: "本体关系候选值" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "取消修订" }));

    await user.click(within(detail).getByRole("tab", { name: "实现" }));
    expect(within(detail).getByRole("heading", { name: "PhysicalBinding" })).toBeVisible();
    expect(within(detail).getByText("BIND-NET-REV-12")).toBeVisible();
    expect(within(detail).getByRole("heading", { name: "JoinContract" })).toBeVisible();

    await user.click(within(detail).getByRole("tab", { name: "可信度" }));
    expect(within(detail).getByRole("region", { name: "可信度摘要" })).toHaveTextContent("可信度健康");
    expect(within(detail).getByRole("table", { name: "指标验证规则" })).toHaveTextContent("METRIC-001");
    expect(within(detail).getByRole("heading", { name: "主张与证据" })).toBeVisible();
    expect(within(detail).getByRole("table", { name: "字段主张与证据" })).toHaveTextContent("spec.expression");
    expect(within(detail).getByText("EVD-2041", { exact: false })).toBeVisible();
    expect(within(detail).getByRole("table", { name: "最近验证运行" })).toHaveTextContent("VALRUN-");

    await user.click(within(detail).getByRole("tab", { name: "交付与影响" }));
    expect(within(detail).getByRole("heading", { name: "交付地址与消费状态" })).toBeVisible();
    expect(within(detail).getByRole("heading", { name: "当前生产指针" })).toBeVisible();
    expect(within(detail).getByRole("heading", { name: "消费者绑定" })).toBeVisible();
    expect(within(detail).getByRole("table", { name: "消费者绑定" })).toHaveTextContent("兼容");

    await user.click(backToCatalog);
    expect(screen.getByRole("region", { name: "知识目录" })).toBeVisible();
    expect(screen.getByRole("searchbox", { name: "搜索知识目录" })).toHaveValue("净收入");
    expect(screen.queryByRole("region", { name: "语义资产详情" })).not.toBeInTheDocument();
  });

  it("specializes the stable detail shell for a non-executable business concept", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    await user.type(screen.getByRole("searchbox", { name: "搜索知识目录" }), "有效支付订单");
    const catalog = screen.getByRole("region", { name: "知识目录" });
    await user.click(within(catalog).getByRole("button", { name: "打开语义资产 有效支付订单" }));

    const detail = screen.getByRole("region", { name: "语义资产详情" });
    expect(within(detail).getAllByRole("tab")).toHaveLength(5);
    expect(within(detail).queryByRole("tab", { name: "实现" })).not.toBeInTheDocument();

    await user.click(within(detail).getByRole("tab", { name: "定义" }));
    const template = within(detail).getByRole("region", { name: "业务概念内容模板" });
    expect(template).toHaveTextContent("术语与业务边界");
    expect(template).toHaveTextContent("别名与消歧");

    await user.click(within(detail).getByRole("tab", { name: "可信度" }));
    const rules = within(detail).getByRole("table", { name: "业务概念验证规则" });
    expect(rules).toHaveTextContent("CONCEPT-001");
    expect(within(detail).getByText("4 / 4 通过")).toBeVisible();

    await user.click(within(detail).getByRole("tab", { name: "交付与影响" }));
    expect(within(detail).getByText("尚无已注册使用方")).toBeVisible();
    expect(within(detail).getByRole("button", { name: "注册使用方" })).toBeVisible();
  });

  it("distinguishes an optional empty implementation from a missing contract", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    await user.type(screen.getByRole("searchbox", { name: "搜索知识目录" }), "支付订单数");
    const catalog = screen.getByRole("region", { name: "知识目录" });
    await user.click(within(catalog).getByRole("button", { name: "打开语义资产 支付订单数" }));
    const detail = screen.getByRole("region", { name: "语义资产详情" });

    await user.click(within(detail).getByRole("tab", { name: "实现" }));
    expect(within(detail).getByText("度量在单一模型内计算")).toBeVisible();
    expect(within(detail).queryByRole("button", { name: "创建 JoinContract" })).not.toBeInTheDocument();

    await user.click(within(detail).getByRole("tab", { name: "交付与影响" }));
    expect(within(detail).getByText("尚无直接消费者")).toBeVisible();
  });

  it("defines deterministic templates and validation profiles for all seven asset types", () => {
    expect(Object.keys(assetTypeProfiles)).toEqual(["业务概念", "业务实体", "语义模型", "维度", "度量", "指标", "分群"]);
    Object.values(assetTypeProfiles).forEach((profile) => {
      expect(profile.requiredFields).toHaveLength(5);
      expect(profile.validationRules).toHaveLength(4);
      expect(Object.keys(profile.emptyStates)).toEqual(["relations", "bindings", "joins", "evidence", "consumers"]);
    });

    const concept = assets.find((asset) => asset.type === "业务概念")!;
    expect(assetTypeProfiles.业务概念.implementationMode).toBe("not_applicable");
    expect(evaluateAssetTypeRules(concept).every((rule) => rule.state === "passed")).toBe(true);

    const dimension = assets.find((asset) => asset.name === "业务区域")!;
    expect(evaluateAssetTypeRules(dimension)).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: "DIMENSION-001", state: "failed" }),
      expect.objectContaining({ id: "DIMENSION-002", state: "failed" }),
    ]));
  });

  it("reviews included changes from the candidate version detail", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    expect(within(screen.getByRole("complementary", { name: "治理上下文" })).getByRole("button", { name: /资产版本/ })).toBeVisible();
    expect(screen.queryByRole("tab", { name: /变更事项|资产版本/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "查看候选资产版本 客单价 @9" }));
    expect(screen.getByRole("tab", { name: /版本差异.*2/ })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: "收起发布门禁" }));
    expect(screen.getByRole("button", { name: "展开发布门禁" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "展开发布门禁" }));
    await user.click(screen.getByRole("tab", { name: /变更来源.*1/ }));
    expect(screen.getByRole("heading", { name: "包含的变更事项" })).toBeVisible();
    expect(screen.getByText("PROP-128")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "审核候选版本 客单价 @9" }));

    const dialog = screen.getByRole("dialog", { name: "审核 客单价 · @9" });
    expect(within(dialog).getByText(/不会写入或发布真实数据/)).toBeVisible();
    expect(within(dialog).getByText("Cube 编译")).toBeVisible();

    await user.click(within(dialog).getByRole("button", { name: "模拟批准版本" }));
    expect(screen.getByRole("status")).toHaveTextContent("客单价 候选版本已在本次原型会话中标记为批准");
    expect(screen.getByRole("button", { name: "模拟发布 @9" })).toBeVisible();
  });

  it("shows immutable release bindings", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    const versionRegistry = screen.getByRole("region", { name: "语义资产版本列表" });
    expect(versionRegistry.firstElementChild).toHaveClass("asset-version-toolbar");
    expect(within(versionRegistry).queryByRole("heading", { name: "资产版本" })).not.toBeInTheDocument();
    expect(within(versionRegistry).queryByText("3 个候选 · 6 条已发布")).not.toBeInTheDocument();
    const currentRelease = screen.getByRole("button", { name: "查看语义资产版本 净收入 @12" });
    expect(currentRelease).toHaveTextContent("当前版本");
    await user.click(currentRelease);
    expect(screen.getByText("Fluxale Production")).toBeVisible();
    expect(screen.getAllByText("锁定 release-2026.08.3")).toHaveLength(2);
  });

  it("keeps every secondary menu inside its primary module", async () => {
    const user = userEvent.setup();
    render(<App />);
    const context = screen.getByRole("complementary", { name: "治理上下文" });

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    expect(within(context).getByRole("button", { name: /资产版本/ })).toBeVisible();
    expect(within(context).queryByRole("button", { name: /版本发布/ })).not.toBeInTheDocument();
    expect(within(context).queryByRole("button", { name: /^变更事项/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /变更事项|资产版本/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "变更与发布" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("region", { name: "语义资产版本列表" })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "资产版本" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "消费与反馈" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "数据接入" }));
    expect(within(context).getByText("数据接入", { exact: true })).toBeVisible();
    expect(within(context).queryByRole("button", { name: "搜索资产、变更与功能" })).not.toBeInTheDocument();
    await user.click(within(context).getByRole("button", { name: /接入自动化/ }));
    expect(screen.getByRole("button", { name: "数据接入" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("region", { name: "接入自动化列表" })).toBeVisible();
    expect(within(context).getByRole("button", { name: /接入运行/ })).toBeVisible();
    expect(screen.queryByRole("tab", { name: /运行活动/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "接入运行" })).not.toBeInTheDocument();

    await user.click(within(context).getByRole("button", { name: /接入运行/ }));
    expect(screen.getByRole("region", { name: "接入运行" })).toBeVisible();
    expect(screen.queryByRole("region", { name: "接入自动化列表" })).not.toBeInTheDocument();
  });

  it("opens searchable commands and jumps directly to an asset", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    await user.keyboard("{Control>}k{/Control}");
    const palette = screen.getByRole("dialog", { name: "搜索与命令" });
    await user.type(within(palette).getByRole("textbox", { name: "搜索资产、变更或功能" }), "净收入");
    await user.click(within(palette).getAllByRole("option")[0]);

    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
  });

  it("keeps recent assets and renders governed objects in one catalog", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(within(context).getByText("知识目录", { exact: true })).toBeVisible();
    expect(within(context).queryByText("最近打开")).not.toBeInTheDocument();
    await user.click(within(context).getByRole("button", { name: "搜索知识目录" }));
    expect(screen.getByRole("searchbox", { name: "搜索知识目录" })).toHaveFocus();
    expect(within(context).getAllByRole("button").filter((button) => button.classList.contains("context-item")).map((button) => button.querySelector("strong")?.textContent)).toEqual(["净收入", "订单经营模型", "高价值客户"]);
    expect(within(context).queryByRole("button", { name: /关系与映射/ })).not.toBeInTheDocument();
    expect(within(context).queryByRole("button", { name: /知识块与证据/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /清单|语义关系|物理映射/ })).not.toBeInTheDocument();
    const catalog = screen.getByRole("region", { name: "知识目录" });
    expect(within(catalog).queryByRole("button", { name: /净收入依赖支付订单数，语义关系/ })).not.toBeInTheDocument();
    await user.click(within(catalog).getByRole("button", { name: "展开净收入的关系与实现" }));
    expect(within(catalog).getByRole("button", { name: /净收入依赖支付订单数，语义关系/ })).toBeVisible();
    expect(within(catalog).getByRole("button", { name: /analytics.orders 映射至 净收入，物理绑定/ })).toBeVisible();
    expect(within(catalog).getByRole("button", { name: /净收入 关联 共享维度 \/ 业务区域，JoinContract/ })).toBeVisible();

    await user.click(within(catalog).getByRole("button", { name: /analytics.orders 映射至 净收入，物理绑定/ }));
    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
    expect(screen.getByRole("tab", { name: "实现" })).toHaveAttribute("aria-selected", "true");
    expect(within(screen.getByRole("region", { name: "语义资产详情" })).getAllByText("BIND-NET-REV-12").some((element) => element.closest("article")?.getAttribute("aria-current") === "true")).toBe(true);

    await user.click(within(context).getByRole("button", { name: "打开最近资产 高价值客户" }));
    expect(screen.getByRole("heading", { name: "高价值客户" })).toBeVisible();
    expect(within(context).getByRole("button", { name: "打开最近资产 高价值客户" })).toHaveAttribute("aria-current", "page");
  });

  it("keeps recent asset positions stable until the user re-enters the module", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    const recentNames = () => within(context).getAllByRole("button")
      .filter((button) => button.classList.contains("context-item"))
      .map((button) => button.querySelector("strong")?.textContent);
    const initialOrder = ["净收入", "订单经营模型", "高价值客户"];

    expect(recentNames()).toEqual(initialOrder);
    await user.click(within(context).getByRole("button", { name: "打开最近资产 高价值客户" }));
    expect(recentNames()).toEqual(initialOrder);
    expect(within(context).getByRole("button", { name: "打开最近资产 高价值客户" })).toHaveAttribute("aria-current", "page");

    await user.click(screen.getByRole("button", { name: "工作台" }));
    await user.click(screen.getByRole("button", { name: "知识资产" }));
    expect(recentNames()).toEqual(["高价值客户", "净收入", "订单经营模型"]);
  });

  it("opens system settings as a focused member directory", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(within(context).getByRole("button", { name: /成员.*24 名成员/ })).toBeVisible();
    expect(within(context).getByRole("button", { name: /访问控制/ })).toBeVisible();
    expect(within(context).queryByRole("button", { name: "搜索资产、变更与功能" })).not.toBeInTheDocument();
    expect(within(document.querySelector(".topbar") as HTMLElement).getByText("成员", { exact: true })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "工作区与成员" })).not.toBeInTheDocument();
    expect(screen.queryByText("身份提供方")).not.toBeInTheDocument();
    expect(screen.queryByText("默认角色")).not.toBeInTheDocument();

    const directory = screen.getByRole("region", { name: "成员目录" });
    const memberList = within(directory).getByRole("region", { name: "成员列表" });
    expect(within(memberList).getByText("EMP-10001")).toBeVisible();
    expect(within(memberList).getByText("yue.lin@semlia.example")).toBeVisible();
    expect(within(memberList).getAllByText("收入分析", { exact: true }).length).toBeGreaterThan(0);
    await user.type(within(directory).getByRole("searchbox", { name: "搜索成员" }), "许言");
    expect(within(memberList).getByText("EMP-10005")).toBeVisible();
    expect(within(memberList).queryByText("EMP-10001")).not.toBeInTheDocument();
  });

  it("inspects roles and grouped permissions from access control", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /访问控制/ }));

    const accessControl = screen.getByRole("region", { name: "访问控制" });
    expect(within(accessControl).getByRole("tab", { name: "角色与权限" })).toHaveAttribute("aria-selected", "true");
    expect(within(accessControl).getByRole("tab", { name: "角色分配" })).toBeVisible();
    expect(within(accessControl).getByRole("tab", { name: "有效权限检查" })).toBeVisible();
    expect(within(accessControl).getByRole("button", { name: "查看角色 Workspace Admin" })).toBeVisible();
    expect(within(accessControl).getByRole("button", { name: "查看角色 Reviewer" })).toBeVisible();

    await user.click(within(accessControl).getByRole("button", { name: "查看角色 Reviewer" }));
    const roleDialog = screen.getByRole("dialog", { name: "Reviewer 角色详情" });
    expect(within(roleDialog).getByText("proposal.review")).toBeVisible();
    expect(within(roleDialog).getByText("审核语义提案并记录评审结论。", { exact: true })).toBeVisible();
    expect(within(roleDialog).queryByText("release.publish")).not.toBeInTheDocument();
    expect(within(roleDialog).queryByRole("button", { name: "编辑角色" })).not.toBeInTheDocument();
    await user.click(within(roleDialog).getByRole("button", { name: "基于此角色创建" }));

    const editor = screen.getByRole("dialog", { name: "基于 Reviewer 创建角色" });
    expect(within(editor).getByText("发布通过治理门禁的候选版本。", { exact: true })).toBeVisible();
    const roleName = within(editor).getByLabelText("角色名称");
    await user.clear(roleName);
    await user.type(roleName, "收入域高级评审");
    await user.click(within(editor).getByRole("checkbox", { name: "配置权限 asset.edit" }));
    await user.click(within(editor).getByRole("button", { name: "预览角色变更" }));
    expect(within(editor).getByRole("region", { name: "角色变更预览" })).toHaveTextContent("新增 1 项权限");
    await user.click(within(editor).getByRole("button", { name: "创建自定义角色" }));

    expect(within(accessControl).getByText("收入域高级评审")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("收入域高级评审已创建");
    await user.click(within(accessControl).getByRole("button", { name: "查看角色 收入域高级评审" }));
    const customRoleDialog = screen.getByRole("dialog", { name: "收入域高级评审 角色详情" });
    await user.click(within(customRoleDialog).getByRole("button", { name: "编辑角色" }));
    const customRoleEditor = screen.getByRole("dialog", { name: "编辑角色 收入域高级评审" });
    expect(within(customRoleEditor).getByRole("checkbox", { name: "配置权限 asset.edit" })).toBeChecked();
    await user.click(within(customRoleEditor).getByRole("checkbox", { name: "配置权限 asset.edit" }));
    await user.click(within(customRoleEditor).getByRole("button", { name: "预览角色变更" }));
    expect(within(customRoleEditor).getByRole("region", { name: "角色变更预览" })).toHaveTextContent("移除 1 项权限");
    await user.click(within(customRoleEditor).getByRole("button", { name: "保存角色变更" }));
    expect(screen.getByRole("status")).toHaveTextContent("收入域高级评审已更新");
  });

  it("assigns scoped roles with a review step and session version update", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /访问控制/ }));
    const accessControl = screen.getByRole("region", { name: "访问控制" });
    await user.click(within(accessControl).getByRole("tab", { name: "角色分配" }));
    await user.click(within(accessControl).getByRole("button", { name: "分配角色" }));

    const dialog = screen.getByRole("dialog", { name: "分配角色" });
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "授权主体" }), "AGENT-GOVERNANCE");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "角色" }), "ROLE-AUDITOR");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "资源范围" }), "workspace:WS-SEMLIA");
    await user.click(within(dialog).getByRole("button", { name: "预览授权" }));
    expect(within(dialog).getByRole("region", { name: "授权变更预览" })).toHaveTextContent("新增 10 项操作权限");
    await user.click(within(dialog).getByRole("button", { name: "确认分配" }));

    expect(within(accessControl).getByText("治理建议 Agent")).toBeVisible();
    expect(within(accessControl).getAllByText("Auditor").length).toBeGreaterThan(1);
    expect(within(accessControl).getByText("authzv-2026.09.01-002")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("角色分配已创建");
  });

  it("blocks conflicting roles in protected scopes", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /访问控制/ }));
    const accessControl = screen.getByRole("region", { name: "访问控制" });
    await user.click(within(accessControl).getByRole("tab", { name: "角色分配" }));
    await user.click(within(accessControl).getByRole("button", { name: "分配角色" }));
    const dialog = screen.getByRole("dialog", { name: "分配角色" });
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "授权主体" }), "USR-REVIEWER");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "角色" }), "ROLE-PUBLISHER");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "资源范围" }), "asset:METRIC-NET-REVENUE");
    await user.click(within(dialog).getByRole("button", { name: "预览授权" }));

    expect(within(dialog).getByRole("alert")).toHaveTextContent("评审者与发布者必须相互独立");
    expect(within(dialog).getByRole("button", { name: "确认分配" })).toBeDisabled();
  });

  it("explains effective access and keeps business titles separate from roles", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    const directory = screen.getByRole("region", { name: "成员目录" });
    expect(within(directory).getByRole("row", { name: /EMP-10001.*语义产品经理.*Workspace Admin/ })).toBeVisible();
    expect(within(directory).getByText("已停用", { exact: true })).toBeVisible();

    await user.click(within(context).getByRole("button", { name: /访问控制/ }));
    const accessControl = screen.getByRole("region", { name: "访问控制" });
    await user.click(within(accessControl).getByRole("tab", { name: "有效权限检查" }));
    await user.click(within(accessControl).getByRole("button", { name: "检查有效权限" }));
    let result = within(accessControl).getByRole("region", { name: "有效权限结果" });
    expect(result).toHaveTextContent("拒绝");
    expect(result).toHaveTextContent("NO_MATCHING_GRANT");
    expect(result).toHaveTextContent("authzv-2026.09.01-001");

    await user.selectOptions(within(accessControl).getByRole("combobox", { name: "检查主体" }), "EMP-10001");
    await user.click(within(accessControl).getByRole("button", { name: "检查有效权限" }));
    result = within(accessControl).getByRole("region", { name: "有效权限结果" });
    expect(result).toHaveTextContent("允许");
    expect(result).toHaveTextContent("Workspace Admin");
    expect(result).toHaveTextContent("Semlia 工作区");
  });

  it("configures LLM and Embedding models as separate system settings", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /模型配置/ }));

    const configuration = screen.getByRole("region", { name: "模型配置" });
    expect(within(configuration).queryByRole("heading", { name: "模型配置" })).not.toBeInTheDocument();
    expect(configuration.querySelector(".model-config-stats")).not.toBeInTheDocument();
    expect(within(configuration).getByRole("tab", { name: "LLM 模型" })).toHaveAttribute("aria-selected", "true");
    expect(within(configuration).getByRole("combobox", { name: "默认 LLM 模型" })).toHaveValue("llm-gpt-41");
    expect(within(configuration).getByText("gpt-4.1")).toBeVisible();
    expect(within(configuration).getByText("claude-sonnet-4-20250514")).toBeVisible();

    await user.click(within(configuration).getByRole("tab", { name: "Embedding 模型" }));
    expect(within(configuration).getByRole("tab", { name: "Embedding 模型" })).toHaveAttribute("aria-selected", "true");
    expect(within(configuration).getByText("text-embedding-3-large")).toBeVisible();
    expect(within(configuration).getByText("bge-m3")).toBeVisible();
    expect(within(configuration).getByText("3,072 dimensions")).toBeVisible();
    expect(configuration.querySelector(".embedding-boundary")).not.toBeInTheDocument();
    expect(within(configuration).getByRole("button", { name: "重建向量索引" })).toBeVisible();

    await user.selectOptions(within(configuration).getByRole("combobox", { name: "默认 Embedding 模型" }), "embedding-bge-m3");
    expect(screen.getByRole("status")).toHaveTextContent("Embedding 默认模型已切换为 bge-m3");
    await user.click(within(configuration).getByRole("button", { name: "重建向量索引" }));
    const rebuildDialog = screen.getByRole("dialog", { name: "重建向量索引" });
    expect(within(rebuildDialog).getByText("全部知识目录")).toBeVisible();
    expect(within(rebuildDialog).getByText("bge-m3")).toBeVisible();
    await user.click(within(rebuildDialog).getByRole("button", { name: "开始重建" }));
    expect(screen.getByRole("status")).toHaveTextContent("向量索引重建任务已创建");
    const rebuildState = within(configuration).getByRole("region", { name: "最近向量索引重建状态" });
    expect(rebuildState).toHaveTextContent("生成向量 · 42%");
    expect(rebuildState).toHaveTextContent("1,436 / 3,420 个知识块");

    await user.click(within(configuration).getByRole("button", { name: "添加供应商" }));
    const dialog = screen.getByRole("dialog", { name: "添加模型供应商" });
    await user.type(within(dialog).getByLabelText("配置名称"), "企业向量网关");
    await user.selectOptions(within(dialog).getByLabelText("供应商"), "OpenAI 兼容");
    await user.type(within(dialog).getByLabelText("Base URL"), "https://models.example.com/v1");
    await user.type(within(dialog).getByLabelText("API Key"), "sk-prototype");
    await user.click(within(dialog).getByRole("button", { name: "添加供应商" }));
    expect(within(configuration).getByText("企业向量网关")).toBeVisible();
    expect(within(configuration).getByText("尚未配置模型")).toBeVisible();

    await user.click(within(rebuildState).getByRole("button", { name: "查看进度与日志" }));
    const rebuildProgressDialog = screen.getByRole("dialog", { name: "重建进度与日志" });
    expect(within(rebuildProgressDialog).getByRole("region", { name: "向量索引重建进度" })).toHaveTextContent("生成向量");
    expect(within(rebuildProgressDialog).getByRole("log", { name: "向量索引重建执行日志" })).toHaveTextContent("扫描知识块");
    expect(within(rebuildProgressDialog).getByText("1,436 / 3,420 个知识块")).toBeVisible();
    expect(screen.queryByRole("region", { name: "接入运行详情" })).not.toBeInTheDocument();
    await user.click(within(rebuildProgressDialog).getByRole("button", { name: "在全局运行记录中打开" }));
    const auditRuntime = screen.getByRole("region", { name: "审计与运行" });
    expect(within(auditRuntime).getByRole("tab", { name: "运行记录" })).toHaveAttribute("aria-selected", "true");
    const rebuildRun = screen.getByRole("dialog", { name: "知识目录向量索引重建" });
    const globalRebuildProgress = within(rebuildRun).getByRole("region", { name: "运行进度" });
    expect(globalRebuildProgress).toHaveTextContent("生成向量");
    expect(globalRebuildProgress).toHaveTextContent("1,436 / 3,420 个知识块");
    expect(within(rebuildRun).getByRole("log", { name: "全局运行执行日志" })).toHaveTextContent("扫描知识块");
  });

  it("uses audit and runtime as a global operations center", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /审计与运行/ }));
    const runtime = screen.getByRole("region", { name: "审计与运行" });
    expect(within(runtime).queryByRole("heading", { name: "审计与运行" })).not.toBeInTheDocument();
    expect(within(runtime).getByRole("region", { name: "运行状态概览" })).toHaveTextContent("24 小时失败");
    const runTable = within(runtime).getByRole("region", { name: "全局运行记录" });
    expect(within(runTable).getByText("客户增长知识增量构建")).toBeVisible();
    await user.click(within(runTable).getByRole("button", { name: "查看运行 客户增长知识增量构建" }));
    expect(screen.getByRole("dialog", { name: "客户增长知识增量构建" })).toHaveTextContent("生成候选版本");
    await user.click(within(screen.getByRole("dialog", { name: "客户增长知识增量构建" })).getByRole("button", { name: "关闭" }));

    await user.click(within(runtime).getByRole("tab", { name: "审计日志" }));
    const auditTable = within(runtime).getByRole("region", { name: "审计事件" });
    expect(within(auditTable).getByText("Codex MCP Workspace")).toBeVisible();
    await user.selectOptions(within(runtime).getByRole("combobox", { name: "筛选调用渠道" }), "MCP");
    expect(within(auditTable).getByText("调用语义检索")).toBeVisible();
    expect(within(auditTable).queryByText("同步元数据")).not.toBeInTheDocument();
    await user.click(within(auditTable).getByRole("button", { name: "查看审计事件 调用语义检索" }));
    expect(screen.getByRole("dialog", { name: "调用语义检索" })).toHaveTextContent("tr_d219a4");
    await user.click(within(screen.getByRole("dialog", { name: "调用语义检索" })).getByRole("button", { name: "关闭" }));

    await user.click(within(runtime).getByRole("tab", { name: "运行设置" }));
    await user.clear(within(runtime).getByRole("spinbutton", { name: "最大并发任务" }));
    await user.type(within(runtime).getByRole("spinbutton", { name: "最大并发任务" }), "6");
    await user.click(within(runtime).getByRole("button", { name: "保存更改" }));
    expect(screen.getByRole("status")).toHaveTextContent("审计与运行设置已保存");
  });

  it("keeps interfaces, bindings and runtime feedback in their owning modules", async () => {
    const user = userEvent.setup();
    render(<App />);

    expect(screen.queryByRole("button", { name: "语义交付" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /接口与集成/ }));
    const integrations = screen.getByRole("region", { name: "接口与集成" });
    expect(within(integrations).getAllByText("REST API", { exact: true }).length).toBeGreaterThan(0);
    expect(within(integrations).getByText("MCP", { exact: true })).toBeVisible();
    expect(within(integrations).getAllByText("CLI", { exact: true }).length).toBeGreaterThan(0);
    expect(within(integrations).getByText("SDK", { exact: true })).toBeVisible();
    expect(within(integrations).getByText("Fluxale Production")).toBeVisible();
    expect(within(integrations).getByText("Codex MCP Workspace")).toBeVisible();
    expect(within(integrations).queryByText("https://api.semlia.example/v1")).not.toBeInTheDocument();
    await user.click(within(integrations).getAllByRole("button", { name: "使用说明" })[0]);
    const guideDialog = screen.getByRole("dialog", { name: "REST API 使用说明" });
    expect(within(guideDialog).getByText("https://api.semlia.example/v1")).toBeVisible();
    expect(within(guideDialog).getByRole("heading", { name: "快速开始" })).toBeVisible();
    expect(within(guideDialog).getByText(/knowledge\/search/)).toBeVisible();
    await user.click(within(guideDialog).getByRole("button", { name: "完成" }));
    await user.click(within(integrations).getByRole("button", { name: "创建客户端" }));
    const clientDialog = screen.getByRole("dialog", { name: "创建客户端" });
    await user.type(within(clientDialog).getByLabelText("客户端名称"), "经营分析 Agent");
    await user.selectOptions(within(clientDialog).getByLabelText("调用方式"), "MCP");
    await user.selectOptions(within(clientDialog).getByLabelText("环境"), "开发");
    await user.click(within(clientDialog).getByRole("button", { name: "创建客户端" }));
    expect(screen.getByRole("dialog", { name: "保存客户端凭据" })).toHaveTextContent("只显示一次");
    await user.click(screen.getByRole("button", { name: "完成" }));
    expect(within(integrations).getByText("经营分析 Agent")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    await user.click(screen.getByRole("button", { name: "查看语义资产版本 净收入 @12" }));
    expect(screen.getByRole("heading", { name: "使用这个资产版本的应用" })).toBeVisible();
    expect(screen.getByText("Fluxale Production")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "工作台" }));
    expect(screen.getByRole("region", { name: "工作台待办队列" })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "待办" })).not.toBeInTheDocument();
    expect(within(context).queryByRole("button", { name: /动态/ })).not.toBeInTheDocument();
  });

  it("scopes machine clients with expiry and explicit revocation", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /接口与集成/ }));
    const integrations = screen.getByRole("region", { name: "接口与集成" });

    expect(within(integrations).getByText("已过期", { exact: true })).toBeVisible();
    await user.click(within(integrations).getByRole("button", { name: "撤销客户端 Fluxale Production" }));
    expect(within(integrations).getByText("已撤销", { exact: true })).toBeVisible();

    await user.click(within(integrations).getByRole("button", { name: "创建客户端" }));
    const dialog = screen.getByRole("dialog", { name: "创建客户端" });
    expect(within(dialog).queryByText("工作区全量权限")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("checkbox", { name: "semantic.resolve" })).toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: "semantic.execute" })).toBeChecked();
    await user.type(within(dialog).getByLabelText("客户端名称"), "受限查询 Agent");
    await user.click(within(dialog).getByRole("checkbox", { name: "semantic.execute" }));
    await user.clear(within(dialog).getByLabelText("到期时间"));
    await user.type(within(dialog).getByLabelText("到期时间"), "2027-01-31");
    await user.click(within(dialog).getByRole("button", { name: "创建客户端" }));
    await user.click(screen.getByRole("button", { name: "完成" }));
    expect(within(integrations).getByText("受限查询 Agent")).toBeVisible();
    expect(within(integrations).getByText("1 项权限")).toBeVisible();
    expect(within(integrations).getByText("2027-01-31")).toBeVisible();
  });

  it("blocks protected publish conflicts while preserving an independent publisher path", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    await user.click(screen.getByRole("button", { name: "查看候选资产版本 客单价 @9" }));
    await user.click(screen.getByRole("button", { name: "审核候选版本 客单价 @9" }));
    const reviewDialog = screen.getByRole("dialog", { name: "审核 客单价 · @9" });
    expect(within(reviewDialog).getByText("陈默 · Reviewer")).toBeVisible();
    expect(reviewDialog).toHaveTextContent("发布者必须是独立主体");
    await user.click(within(reviewDialog).getByRole("button", { name: "模拟批准版本" }));
    await user.click(screen.getByRole("button", { name: "模拟发布 @9" }));
    const publishDialog = screen.getByRole("dialog", { name: "发布客单价 @9" });
    expect(within(publishDialog).getByRole("combobox", { name: "发布身份" })).toHaveValue("USR-PUBLISHER");
    expect(within(publishDialog).getByText("陈默评审 · 周岚发布")).toBeVisible();
    await user.selectOptions(within(publishDialog).getByRole("combobox", { name: "发布身份" }), "USR-REVIEWER");
    expect(within(publishDialog).getByRole("alert")).toHaveTextContent("评审者与发布者必须相互独立");
    expect(within(publishDialog).getByRole("button", { name: "确认模拟发布并生效" })).toBeDisabled();
  });

  it("uses the workbench as an actionable queue instead of a duplicate status dashboard", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "工作台" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(within(context).queryByRole("button", { name: /^待办/ })).not.toBeInTheDocument();
    expect(within(context).getAllByRole("button", { name: /打开待办详情/ })).toHaveLength(3);
    await user.click(within(context).getByRole("button", { name: "搜索待办" }));
    expect(screen.getByRole("searchbox", { name: "搜索待办" })).toHaveFocus();
    expect(within(context).queryByRole("button", { name: /动态/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "待办" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "工作台待办队列" })).toBeVisible();
    expect(screen.queryByRole("region", { name: "我的待办概览" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "可信状态" })).not.toBeInTheDocument();

    await user.click(within(context).getByRole("button", { name: "打开待办详情 检查客户增长域运行异常" }));
    expect(screen.getByRole("region", { name: "接入运行详情" })).toBeVisible();
    expect(within(context).getByRole("button", { name: "打开待办详情 检查客户增长域运行异常" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "工作台" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "变更与发布" })).not.toHaveAttribute("aria-current", "page");
    const backToTasks = screen.getByRole("button", { name: "返回待办" });
    expect(backToTasks.closest(".topbar")).not.toBeNull();
    await user.click(backToTasks);
    expect(screen.getByRole("region", { name: "工作台待办队列" })).toBeVisible();

    await user.selectOptions(screen.getByRole("combobox", { name: "筛选待办风险" }), "高风险");
    const queue = screen.getByRole("region", { name: "工作台待办队列" });
    expect(within(queue).getByRole("button", { name: /旧区域别名无法安全废弃/ })).toBeVisible();
    expect(within(queue).queryByRole("button", { name: /确认客单价退款订单口径/ })).not.toBeInTheDocument();

    await user.selectOptions(screen.getByRole("combobox", { name: "筛选待办风险" }), "全部风险");
    await user.click(screen.getByRole("button", { name: /我发起 1/ }));
    expect(within(queue).getByRole("button", { name: /补充净收入财务口径证据/ })).toBeVisible();
    expect(within(queue).queryByRole("button", { name: /旧区域别名无法安全废弃/ })).not.toBeInTheDocument();
  });

  it("scopes version comparison to one semantic asset", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    expect(screen.queryByRole("button", { name: "比较版本" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "查看语义资产版本 净收入 @12" }));
    await user.click(screen.getByRole("button", { name: "与 @11 比较" }));

    const dialog = screen.getByRole("dialog", { name: "比较净收入版本" });
    expect(dialog).toBeVisible();
    expect(within(dialog).getByText("净收入 · 上一版本")).toBeVisible();
    expect(within(dialog).getByText("净收入 · 当前选择")).toBeVisible();
  });

  it("provides distinct, functional pages for data ingestion", async () => {
    const user = userEvent.setup();
    render(<App />);
    const context = screen.getByRole("complementary", { name: "治理上下文" });

    await user.click(screen.getByRole("button", { name: "数据接入" }));
    const connectionSearch = screen.getByRole("searchbox", { name: "搜索数据来源" });
    await user.type(connectionSearch, "PostgreSQL");
    const sourceRegistry = screen.getByRole("tabpanel", { name: /数据库连接/ });
    expect(within(sourceRegistry).getByText("PostgreSQL Analytics")).toBeVisible();
    expect(within(sourceRegistry).getAllByText("元数据快照差异 · CDC 未配置").length).toBeGreaterThan(0);
    expect(within(sourceRegistry).queryByText("Cube Commerce")).not.toBeInTheDocument();
    await user.clear(connectionSearch);
    await user.click(screen.getByRole("button", { name: "连接数据库" }));
    const dialog = screen.getByRole("dialog", { name: "连接数据库" });
    expect(within(dialog).getByRole("button", { name: "保存并建立基线" })).toBeDisabled();
    const incrementalCapabilities = within(dialog).getByLabelText("数据库增量能力");
    expect(incrementalCapabilities).toHaveTextContent("只读 Catalog");
    expect(incrementalCapabilities).toHaveTextContent("SourceRevision 差异");
    expect(incrementalCapabilities).toHaveTextContent("CDC 未配置");
    await user.type(within(dialog).getByLabelText("连接名称"), "Finance Warehouse");
    await user.type(within(dialog).getByLabelText("主机地址"), "finance.internal");
    await user.type(within(dialog).getByLabelText("数据库名称"), "finance");
    await user.type(within(dialog).getByLabelText("用户名"), "semlia_reader");
    const passwordInput = within(dialog).getByLabelText("密码");
    await user.type(passwordInput, "read-only-secret");
    expect(passwordInput).toHaveAttribute("type", "password");
    await user.click(within(dialog).getByRole("button", { name: "显示密码" }));
    expect(passwordInput).toHaveAttribute("type", "text");
    await user.click(within(dialog).getByRole("button", { name: "隐藏密码" }));
    expect(passwordInput).toHaveAttribute("type", "password");
    await user.click(within(dialog).getByRole("button", { name: "测试连接" }));
    expect(await within(dialog).findByText("连接测试通过")).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: "保存并建立基线" }));
    expect(screen.getByText("Finance Warehouse")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "编辑 Finance Warehouse" }));
    const editDialog = screen.getByRole("dialog", { name: "编辑数据库连接" });
    const editName = within(editDialog).getByLabelText("连接名称");
    await user.clear(editName);
    await user.type(editName, "Finance Warehouse Updated");
    await user.click(within(editDialog).getByRole("button", { name: "测试连接" }));
    expect(await within(editDialog).findByText("连接测试通过")).toBeVisible();
    await user.click(within(editDialog).getByRole("button", { name: "保存修改" }));
    expect(screen.getByText("Finance Warehouse Updated")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "删除 Finance Warehouse Updated" }));
    const deleteDialog = screen.getByRole("alertdialog", { name: "删除数据库连接" });
    expect(within(deleteDialog).getByText("Finance Warehouse Updated")).toBeVisible();
    await user.click(within(deleteDialog).getByRole("button", { name: "确认删除" }));
    expect(screen.queryByText("Finance Warehouse Updated")).not.toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /文件导入/ }));
    await user.click(screen.getByRole("button", { name: "重新导入 FY2026 营收目标" }));
    const reimportDialog = screen.getByRole("dialog", { name: "导入文件" });
    expect(within(reimportDialog).getByLabelText("导入方式")).toHaveValue("snapshot");
    expect(within(reimportDialog).getByLabelText("目标来源")).toHaveValue("FY2026 营收目标");
    await user.click(within(reimportDialog).getByRole("button", { name: "取消" }));

    await user.click(screen.getByRole("button", { name: "导入文件" }));
    const fileDialog = screen.getByRole("dialog", { name: "导入文件" });
    expect(within(fileDialog).getByRole("button", { name: "导入并开始构建" })).toBeDisabled();
    await user.selectOptions(within(fileDialog).getByLabelText("导入方式"), "snapshot");
    expect(within(fileDialog).getByLabelText("目标来源")).toHaveValue("FY2026 营收目标");
    expect(within(fileDialog).queryByLabelText("来源名称")).not.toBeInTheDocument();
    await user.selectOptions(within(fileDialog).getByLabelText("导入方式"), "create");
    expect(within(fileDialog).getByLabelText("来源名称")).toBeVisible();
    await user.upload(within(fileDialog).getByLabelText("选择文件"), new File(["region,target\n华东,120"], "regional_targets.csv", { type: "text/csv" }));
    await user.click(within(fileDialog).getByRole("button", { name: "检查文件" }));
    expect(await within(fileDialog).findByText("文件检查通过")).toBeVisible();
    await user.click(within(fileDialog).getByRole("button", { name: "导入并开始构建" }));
    expect(screen.getByRole("tabpanel", { name: /文件导入/ })).toHaveTextContent("regional_targets");

    await user.click(screen.getByRole("button", { name: "编辑 regional_targets" }));
    const editFileDialog = screen.getByRole("dialog", { name: "编辑文件来源" });
    expect(within(editFileDialog).getByLabelText("源文件")).toHaveValue("regional_targets.csv · 1 KB");
    const editDatasetName = within(editFileDialog).getByLabelText("来源名称");
    await user.clear(editDatasetName);
    await user.type(editDatasetName, "区域经营目标");
    await user.click(within(editFileDialog).getByRole("button", { name: "保存修改" }));
    expect(screen.getByText("区域经营目标")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "删除 区域经营目标" }));
    const deleteFileDialog = screen.getByRole("alertdialog", { name: "删除文件来源" });
    expect(within(deleteFileDialog).getByText("区域经营目标")).toBeVisible();
    await user.click(within(deleteFileDialog).getByRole("button", { name: "确认删除" }));
    expect(screen.queryByText("区域经营目标")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "导入文件" }));
    const markdownDialog = screen.getByRole("dialog", { name: "导入文件" });
    await user.upload(within(markdownDialog).getByLabelText("选择文件"), new File(["# 净收入\n\n净收入扣除全额退款。\n\n```sql\nselect 1\n```"], "metric_dictionary.md", { type: "text/markdown" }));
    expect(within(markdownDialog).getByLabelText("分块方式")).toHaveValue("按标题层级");
    expect(within(markdownDialog).getByLabelText("保留代码块")).toBeChecked();
    expect(within(markdownDialog).queryByLabelText("字段分隔符")).not.toBeInTheDocument();
    await user.click(within(markdownDialog).getByRole("button", { name: "检查文件" }));
    expect(await within(markdownDialog).findByText("文件检查通过")).toBeVisible();
    await user.click(within(markdownDialog).getByRole("button", { name: "导入并开始构建" }));
    expect(screen.getByRole("tabpanel", { name: /文件导入/ })).toHaveTextContent("metric_dictionary");

    await user.click(screen.getByRole("button", { name: "编辑 metric_dictionary" }));
    const editMarkdownDialog = screen.getByRole("dialog", { name: "编辑文件来源" });
    const markdownName = within(editMarkdownDialog).getByLabelText("来源名称");
    await user.clear(markdownName);
    await user.type(markdownName, "指标口径手册");
    await user.selectOptions(within(editMarkdownDialog).getByLabelText("分块方式"), "按固定长度");
    await user.click(within(editMarkdownDialog).getByRole("button", { name: "保存修改" }));
    expect(await screen.findByText("指标口径手册")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "重新导入 指标口径手册" }));
    const markdownVersionDialog = screen.getByRole("dialog", { name: "导入文件" });
    expect(within(markdownVersionDialog).getByLabelText("目标来源")).toHaveValue("指标口径手册");
    await user.upload(within(markdownVersionDialog).getByLabelText("选择文件"), new File(["# 净收入\n\n更新后的口径。"], "metric_dictionary.md", { type: "text/markdown" }));
    await user.click(within(markdownVersionDialog).getByRole("button", { name: "检查文件" }));
    expect(await within(markdownVersionDialog).findByText("文件检查通过", {}, { timeout: 2_000 })).toBeVisible();
    await user.click(within(markdownVersionDialog).getByRole("button", { name: "导入新版本" }));
    expect(screen.getByRole("tabpanel", { name: /文件导入/ })).toHaveTextContent("新版本");

    await user.click(within(context).getByRole("button", { name: /接入自动化/ }));
    expect(screen.getByRole("region", { name: "接入自动化列表" })).toBeVisible();
    expect(within(context).queryByRole("button", { name: /构建运行/ })).not.toBeInTheDocument();
    await user.click(within(context).getByRole("button", { name: /接入运行/ }));
    const runList = screen.getByRole("region", { name: "接入运行" });
    expect(runList).toBeVisible();
    expect(runList).toHaveTextContent("经营数据元数据同步");
    expect(runList).toHaveTextContent("2,846 对象 · 12 项变化");
    expect(runList).toHaveTextContent("数据库同步");
    expect(runList).not.toHaveTextContent("候选版本");
    expect(runList).not.toHaveTextContent("知识块");
    const firstRunRow = within(runList).getByRole("button", { name: /查看运行 RUN-240824-1432/ });
    expect(firstRunRow.querySelector(".build-run-list-id")).toHaveTextContent("RUN-240824-1432");
    expect(firstRunRow.querySelector(".build-run-list-task")).toHaveTextContent("经营数据元数据同步");
    expect(firstRunRow.querySelector(".build-run-list-time")).toHaveTextContent("2026-08-24 14:32:18");
    expect(firstRunRow.querySelector(".build-run-list-mode")).toHaveTextContent("数据库同步");
    expect(firstRunRow.querySelector(".build-run-list-duration")).toHaveTextContent("2 分 18 秒");
    expect(screen.getByRole("searchbox", { name: "搜索接入运行" })).toHaveAttribute("placeholder", "搜索 Run ID、任务或来源");
    expect(screen.getByText("这里只记录数据进入系统的过程")).toBeVisible();
    await user.click(screen.getByRole("button", { name: /查看运行 RUN-240824-1432/ }));
    const runDetail = screen.getByRole("region", { name: "接入运行详情" });
    expect(within(runDetail).getByLabelText("元数据版本差异")).toBeVisible();
    expect(within(runDetail).getByLabelText("元数据版本差异")).toHaveTextContent("3新增8修改1删除");
    expect(runDetail).toHaveTextContent("schema:9f2e8a");
    expect(within(runDetail).getByLabelText("接入阶段")).toHaveTextContent("触发下游");
    expect(within(runDetail).queryByRole("group", { name: "运行结果关系图" })).not.toBeInTheDocument();
    expect(within(runDetail).queryByRole("button", { name: /关联候选版本/ })).not.toBeInTheDocument();
    await user.click(within(runDetail).getByRole("button", { name: "查看执行日志" }));
    const runLogDialog = screen.getByRole("dialog", { name: "执行日志" });
    expect(within(runLogDialog).getByRole("log", { name: "运行执行日志" })).toHaveTextContent("连接来源");
    expect(within(runLogDialog).getByRole("log", { name: "运行执行日志" })).toHaveTextContent("RUN-240901-1208");
    await user.click(within(runLogDialog).getByRole("button", { name: "关闭执行日志" }));
    const backToRuns = screen.getByRole("button", { name: "返回接入运行" });
    expect(backToRuns.closest(".topbar")).not.toBeNull();
    await user.click(backToRuns);
    expect(screen.getByRole("region", { name: "接入运行" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: /查看运行 RUN-240824-1432/ }));
    await user.click(within(screen.getByRole("region", { name: "接入运行详情" })).getByRole("button", { name: "打开下游运行" }));
    expect(screen.getByRole("region", { name: "全局运行记录" })).toBeVisible();
    const downstreamDialog = screen.getByRole("dialog", { name: "客户增长知识增量构建" });
    expect(downstreamDialog).toHaveTextContent("上游接入 RUN-240824-1432");
  }, 15_000);

  it("creates, edits, filters, pauses and deletes build tasks", async () => {
    const user = userEvent.setup();
    render(<App />);
    const context = screen.getByRole("complementary", { name: "治理上下文" });

    await user.click(screen.getByRole("button", { name: "数据接入" }));
    await user.click(within(context).getByRole("button", { name: /接入自动化/ }));
    const taskTable = screen.getByRole("region", { name: "接入自动化列表" });
    expect(within(taskTable).getByText("经营数据元数据同步")).toBeVisible();
    expect(within(taskTable).getAllByText("全量校准").length).toBeGreaterThan(0);
    expect(within(taskTable).queryByText("src-ecommerce@9f2e8a")).not.toBeInTheDocument();
    expect(within(taskTable).queryByRole("button", { name: /^(停用|启用) / })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "异常" }));
    expect(within(taskTable).getByText("客户增长源增量扫描")).toBeVisible();
    expect(within(taskTable).queryByText("经营数据元数据同步")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^全部/ }));

    await user.click(screen.getByRole("button", { name: "新建构建任务" }));
    const createDialog = screen.getByRole("dialog", { name: "新建构建任务" });
    expect(createDialog.querySelector(".build-task-section-heading")).not.toBeInTheDocument();
    expect(within(createDialog).queryByText("基本信息", { exact: true })).not.toBeInTheDocument();
    expect(within(createDialog).queryByText("输入范围", { exact: true })).not.toBeInTheDocument();
    expect(within(createDialog).queryByText("运行策略", { exact: true })).not.toBeInTheDocument();
    const enabledToggle = within(createDialog).getByLabelText("启用任务");
    expect(enabledToggle).toBeChecked();
    expect(enabledToggle.closest("header")).toHaveClass("build-task-dialog-header");
    expect(within(createDialog).getByRole("radio", { name: /元数据增量/ })).toBeChecked();
    expect(within(createDialog).getByRole("button", { name: "查看元数据增量说明" })).toBeVisible();
    const modeHelp = within(createDialog).getByRole("tooltip");
    expect(modeHelp).toHaveTextContent("元数据快照，不是业务数据快照");
    expect(modeHelp).toHaveTextContent("失败运行不会推进 checkpoint");
    expect(within(createDialog).queryByText("保存任务配置，不会立即发起构建。")).not.toBeInTheDocument();
    expect(within(createDialog).queryByText("识别这项持续构建工作")).not.toBeInTheDocument();
    expect(within(createDialog).queryByText("所选来源内有权限读取的对象均参与构建")).not.toBeInTheDocument();
    expect(within(createDialog).queryByText("定义每次构建的处理方式与触发时机")).not.toBeInTheDocument();
    expect(within(createDialog).queryByRole("combobox", { name: "对象范围" })).not.toBeInTheDocument();
    expect(within(createDialog).getByRole("button", { name: "创建任务" })).toBeDisabled();
    await user.type(within(createDialog).getByLabelText("任务名称"), "区域目标知识构建");
    expect(within(createDialog).getByRole("button", { name: "创建任务" })).toBeDisabled();
    await user.click(within(createDialog).getByRole("button", { name: /选择数据来源/ }));
    expect(within(createDialog).getByRole("searchbox", { name: "搜索可用数据来源" })).toBeVisible();
    await user.click(within(createDialog).getByRole("checkbox", { name: "选择数据来源 PostgreSQL Analytics" }));
    await user.click(within(createDialog).getByRole("button", { name: "完成选择" }));
    await user.click(within(createDialog).getByRole("radio", { name: /全量校准/ }));
    await user.click(within(createDialog).getByRole("radio", { name: "定时调度" }));
    await user.selectOptions(within(createDialog).getByLabelText("计划模板"), "0 9 * * 1-5");
    expect(within(createDialog).getByLabelText("Cron 表达式")).toHaveValue("0 9 * * 1-5");
    expect(createDialog.querySelector(".build-schedule-preview")).not.toBeInTheDocument();
    expect(createDialog.querySelector(".build-task-toggle")).not.toBeInTheDocument();
    await user.clear(within(createDialog).getByLabelText("Cron 表达式"));
    expect(within(createDialog).getByRole("button", { name: "创建任务" })).toBeDisabled();
    await user.type(within(createDialog).getByLabelText("Cron 表达式"), "30 3 * * 1-5");
    await user.selectOptions(within(createDialog).getByLabelText("调度时区"), "UTC");
    await user.click(within(createDialog).getByRole("button", { name: "创建任务" }));
    expect(within(taskTable).getByText("区域目标知识构建")).toBeVisible();

    await user.click(within(taskTable).getByRole("button", { name: "编辑 区域目标知识构建" }));
    const editDialog = screen.getByRole("dialog", { name: "编辑构建任务" });
    expect(within(editDialog).getByRole("radio", { name: "定时调度" })).toBeChecked();
    expect(within(editDialog).getByLabelText("Cron 表达式")).toHaveValue("30 3 * * 1-5");
    expect(within(editDialog).getByLabelText("调度时区")).toHaveValue("UTC");
    const taskName = within(editDialog).getByLabelText("任务名称");
    await user.clear(taskName);
    await user.type(taskName, "区域目标增量构建");
    await user.click(within(editDialog).getByLabelText("启用任务"));
    await user.click(within(editDialog).getByRole("button", { name: "保存修改" }));
    expect(within(taskTable).getByText("区域目标增量构建")).toBeVisible();
    const editedTaskRow = within(taskTable).getByText("区域目标增量构建").closest(".build-task-row");
    expect(editedTaskRow).not.toBeNull();
    expect(within(editedTaskRow as HTMLElement).getByText("停用")).toBeVisible();
    expect(within(editedTaskRow as HTMLElement).getByRole("button", { name: "运行 区域目标增量构建" })).toBeDisabled();

    await user.click(within(taskTable).getByRole("button", { name: "删除 区域目标增量构建" }));
    const deleteDialog = screen.getByRole("alertdialog", { name: "删除构建任务" });
    await user.click(within(deleteDialog).getByRole("button", { name: "确认删除" }));
    expect(within(taskTable).queryByText("区域目标增量构建")).not.toBeInTheDocument();
  }, 15_000);

  it("completes the governed lifecycle from discovery to feedback", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "数据接入" }));
    expect(screen.getByRole("searchbox", { name: "搜索数据来源" })).toBeVisible();
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /接入自动化/ }));
    const taskTable = screen.getByRole("region", { name: "接入自动化列表" });
    expect(taskTable).toBeVisible();
    await user.click(within(taskTable).getByRole("button", { name: "运行 经营数据元数据同步" }));
    expect(screen.getAllByText("运行中").length).toBeGreaterThan(1);
    expect(await within(taskTable).findByText("已完成")).toBeVisible();
    await user.click(within(context).getByRole("button", { name: /接入运行/ }));
    const ingressRuns = screen.getByRole("region", { name: "接入运行" });
    expect(ingressRuns).not.toHaveTextContent("RUN-SESSION");
    expect(ingressRuns).not.toHaveTextContent("知识增量构建");
    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    expect(within(screen.getByRole("complementary", { name: "治理上下文" })).queryByRole("button", { name: "搜索资产、变更与功能" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "查看候选资产版本 客单价 @9" }));
    const backToVersions = screen.getByRole("button", { name: "返回资产版本列表" });
    expect(backToVersions.closest(".topbar")).not.toBeNull();
    expect(document.querySelector(".governance-detail-commandbar")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "客单价 @9 候选资产版本详情" })).toHaveTextContent("版本范围");
    await user.click(screen.getByRole("button", { name: "审核候选版本 客单价 @9" }));
    await user.click(within(screen.getByRole("dialog", { name: "审核 客单价 · @9" })).getByRole("button", { name: "模拟批准版本" }));
    await user.click(screen.getByRole("button", { name: "模拟发布 @9" }));
    await user.click(within(screen.getByRole("dialog", { name: "发布客单价 @9" })).getByRole("button", { name: "确认模拟发布并生效" }));
    expect(screen.getByText(/release-2026\.08\.4-session/)).toBeVisible();

    await user.click(backToVersions);
    await user.click(screen.getByRole("button", { name: "查看语义资产版本 净收入 @12" }));
    expect(screen.getByRole("heading", { name: "使用这个资产版本的应用" })).toBeVisible();
    expect(screen.getByText("Fluxale Production")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "工作台" }));
    await user.click(within(screen.getByRole("complementary", { name: "治理上下文" })).getByRole("button", { name: "打开待办详情 旧区域别名无法安全废弃" }));
    expect(screen.getByRole("region", { name: "业务区域 @4 候选资产版本详情" })).toBeVisible();
    expect(screen.getByRole("button", { name: "工作台" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "变更与发布" })).not.toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "返回待办" }).closest(".topbar")).not.toBeNull();
    await user.click(screen.getByRole("tab", { name: /变更来源.*1/ }));
    expect(screen.getByRole("heading", { name: "包含的变更事项" })).toBeVisible();
  }, 15_000);
});
