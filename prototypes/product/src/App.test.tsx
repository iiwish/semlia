import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { App } from "./App";

describe("Semlia product prototype", () => {
  it("uses a three-layer semantic operations workspace", () => {
    render(<App />);

    expect(screen.getByLabelText("Semlia 主功能")).toBeInTheDocument();
    expect(screen.getByRole("complementary", { name: "治理上下文" })).toBeInTheDocument();
    expect(screen.getByRole("main")).toHaveClass("workspace-canvas");
  });

  it("keeps the mock boundary visible on the operational first view", () => {
    render(<App />);

    expect(screen.getByText("Prototype · Mock data")).toBeVisible();
    expect(screen.getByRole("heading", { name: "语义治理概览" })).toBeVisible();
    expect(screen.getByRole("img", { name: "语义覆盖关系图" })).toBeVisible();
  });

  it("filters the asset catalog and opens a trusted asset detail", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "语义资产" }));
    await user.type(screen.getByRole("searchbox", { name: "搜索语义资产" }), "净收入");

    const catalog = screen.getByRole("region", { name: "语义资产目录" });
    expect(within(catalog).getByText("净收入")).toBeVisible();
    expect(within(catalog).queryByText("活跃客户")).not.toBeInTheDocument();

    await user.click(within(catalog).getByRole("button", { name: /净收入/ }));
    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
    expect(within(screen.getByRole("region", { name: "语义资产详情" })).getAllByText("release-2026.08.3")).toHaveLength(2);
  });

  it("reviews a proposal without implying a durable write", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "变更提案" }));
    await user.click(screen.getByRole("button", { name: /审核 PROP-128/ }));

    const dialog = screen.getByRole("dialog", { name: "审核提案 PROP-128" });
    expect(within(dialog).getByText("仅更新本次原型会话，不会写入或发布真实数据。")).toBeVisible();
    expect(within(dialog).getByText("Cube compile")).toBeVisible();

    await user.click(within(dialog).getByRole("button", { name: "模拟批准" }));
    expect(screen.getByRole("status")).toHaveTextContent("已在本次原型会话中标记为批准");
  });

  it("shows immutable release bindings", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "发布版本" }));
    expect(screen.getByRole("heading", { name: "不可变发布版本" })).toBeVisible();
    expect(screen.getByText("Fluxale Production")).toBeVisible();
    expect(screen.getAllByText("锁定 release-2026.08.3")).toHaveLength(2);
  });

  it("completes the governed lifecycle from discovery to feedback", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "接入发现" }));
    expect(screen.getByRole("heading", { name: "接入与语义发现" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "运行增量发现" }));
    expect(screen.getByText("发现完成：6 个提案")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "查看 6 个提案" }));

    await user.click(screen.getByRole("button", { name: /审核 PROP-128/ }));
    await user.click(within(screen.getByRole("dialog", { name: "审核提案 PROP-128" })).getByRole("button", { name: "模拟批准" }));

    await user.click(screen.getByRole("button", { name: "发布版本" }));
    expect(screen.getByRole("heading", { name: "Release candidate" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "模拟发布 candidate" }));
    await user.click(within(screen.getByRole("dialog", { name: "发布 release candidate" })).getByRole("button", { name: "确认模拟发布" }));
    expect(screen.getByText("release-2026.08.4-session")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "消费监控" }));
    expect(screen.getByRole("heading", { name: "消费与反馈" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "创建绑定" }));
    const bindingDialog = screen.getByRole("dialog", { name: "创建消费绑定" });
    await user.type(within(bindingDialog).getByLabelText("消费者名称"), "Revenue Copilot");
    await user.click(within(bindingDialog).getByRole("button", { name: "创建模拟绑定" }));
    expect(screen.getByText("Revenue Copilot")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "生成修复提案" }));
    expect(screen.getByRole("heading", { name: "变更提案" })).toBeVisible();
  });
});
