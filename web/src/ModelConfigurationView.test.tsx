import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CapabilityProvider } from "./authorization";
import { GovernanceRuntimeProvider } from "./governanceRuntime";
import { ModelConfigurationView } from "./ModelConfigurationView";
import { createGovernanceFixtureApi } from "./testing/governanceFixtureApi";
import { authorizationSession } from "./testing/data";

describe("embedding model dimensions", () => {
  it.each([[2, true], [4096, true], [4097, false]] as const)("validates dimension %s against the index contract", async (dimension, valid) => {
    const user = userEvent.setup();
    render(<CapabilityProvider session={authorizationSession}><GovernanceRuntimeProvider workspaceId="wsp_fixture" api={createGovernanceFixtureApi()}><ModelConfigurationView onNotify={vi.fn()} /></GovernanceRuntimeProvider></CapabilityProvider>);
    await user.click(screen.getByRole("tab", { name: "Embedding 模型" }));
    await user.click(await screen.findByRole("button", { name: "编辑模型 text-embedding-3-large" }));
    const dialog = screen.getByRole("dialog", { name: "编辑模型" });
    const input = within(dialog).getByLabelText("向量维度") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, String(dimension));
    expect(input.checkValidity()).toBe(valid);
    expect(input.closest("form")!.checkValidity()).toBe(valid);
    await user.click(within(dialog).getByRole("button", { name: "保存模型" }));
    if (valid) await waitFor(() => expect(screen.queryByRole("dialog", { name: "编辑模型" })).not.toBeInTheDocument());
    else expect(dialog).toBeVisible();
  });
});

describe("model provider credential env name", () => {
  it("rejects a lowercase env name before the request is sent and explains the format", async () => {
    const api = createGovernanceFixtureApi();
    const createModelProvider = vi.spyOn(api, "createModelProvider");
    const user = userEvent.setup();
    render(<CapabilityProvider session={authorizationSession}><GovernanceRuntimeProvider workspaceId="wsp_fixture" api={api}><ModelConfigurationView onNotify={vi.fn()} /></GovernanceRuntimeProvider></CapabilityProvider>);
    await user.click(await screen.findByRole("button", { name: "添加供应商" }));
    const dialog = screen.getByRole("dialog", { name: "添加模型供应商" });
    await user.type(within(dialog).getByLabelText("配置名称"), "深度Deepseek");
    await user.selectOptions(within(dialog).getByLabelText("供应商"), "openai_compatible");
    await user.type(within(dialog).getByLabelText("Base URL"), "https://api.deepseek.com");
    await user.type(within(dialog).getByLabelText("凭据环境变量"), "deepseek_token");
    await user.type(within(dialog).getByLabelText("API Key"), "sk-test");
    expect(within(dialog).getByRole("note")).toHaveTextContent("需以大写字母开头，只能含大写字母、数字和下划线");
    expect(within(dialog).getByRole("button", { name: "添加供应商" })).toBeDisabled();
    expect(createModelProvider).not.toHaveBeenCalled();

    await user.clear(within(dialog).getByLabelText("凭据环境变量"));
    await user.type(within(dialog).getByLabelText("凭据环境变量"), "SEMLIA_DEEPSEEK_API_KEY");
    expect(within(dialog).queryByRole("note")).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "添加供应商" })).toBeEnabled();
  });
});
