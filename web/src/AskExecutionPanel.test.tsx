import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { AskExecutionPanel } from "./AskExecutionPanel";
import { cancelExecution, executePlan, getExecution, type ExecutionPlan, type ExecutionResult } from "./execution";

vi.mock("./execution", () => ({ executePlan: vi.fn(), getExecution: vi.fn(), cancelExecution: vi.fn() }));
const plan: ExecutionPlan = { id: "rsp_plan_a", queryId: "smq_query", releaseId: "rls_release", resolverVersion: "alpha-1", intent: "aggregate", assets: [], objects: [], executionStatus: "requires_execution_validation", planDigest: `sha256:${"a".repeat(64)}`, createdAt: "2026-09-05T10:00:00Z" };
function result(): ExecutionResult { return { availability: "ephemeral", replay: false, columns: ["amount"], rows: [["sensitive-exact-value"]], run: { id: "run_record", workspaceId: "wsp_a", queryId: plan.queryId, planId: plan.id, planDigest: plan.planDigest, principalId: "prn_actor", channel: "web", sourceId: "src_source", sourceRevisionId: "srv_revision", adapterVersion: "postgres-aggregate/v1", state: "succeeded", rowCount: 1, byteCount: 64, startedAt: plan.createdAt, traceId: "a".repeat(32), policyVersion: 1, timeoutMs: 1000, maxRows: 10, maxBytes: 1024, cancelRequested: false } }; }

beforeEach(() => { vi.resetAllMocks(); localStorage.clear(); });

it("keeps result rows ephemeral and reloads metadata without automatic execution", async () => {
  vi.mocked(executePlan).mockResolvedValue(result());
  const user = userEvent.setup();
  const view = render(<AskExecutionPanel workspaceId="wsp_a" plan={plan}/>);
  expect(executePlan).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "执行只读查询" }));
  expect(await screen.findByText("sensitive-exact-value")).toBeVisible();
  expect(localStorage.getItem("semlia.execution.wsp_a")).toBe("run_record");
  expect(JSON.stringify(localStorage)).not.toContain("sensitive-exact-value");
  view.unmount();
  const metadata = result(); metadata.availability = "metadata_only"; metadata.replay = true; delete metadata.rows; delete metadata.columns;
  vi.mocked(getExecution).mockResolvedValue(metadata);
  render(<AskExecutionPanel workspaceId="wsp_a"/>);
  expect(await screen.findByText(/原始结果行未存储/)).toBeVisible();
  expect(screen.queryByText("sensitive-exact-value")).not.toBeInTheDocument();
  expect(executePlan).toHaveBeenCalledTimes(1);
});

it("fences a slow old workspace response and does not reuse its key for a new plan", async () => {
  let finish!: (value: ExecutionResult) => void;
  vi.mocked(executePlan).mockReturnValueOnce(new Promise(resolve => { finish = resolve; })).mockResolvedValue(result());
  const user = userEvent.setup();
  const view = render(<AskExecutionPanel workspaceId="wsp_a" plan={plan}/>);
  await user.click(screen.getByRole("button", { name: "执行只读查询" }));
  const oldKey = vi.mocked(executePlan).mock.calls[0][2];
  view.rerender(<AskExecutionPanel workspaceId="wsp_b" plan={{ ...plan, id: "rsp_plan_b" }}/>);
  await act(async () => finish(result()));
  expect(screen.queryByText("sensitive-exact-value")).not.toBeInTheDocument();
  expect(localStorage.getItem("semlia.execution.wsp_a")).toBeNull();
  await user.click(screen.getByRole("button", { name: "执行只读查询" }));
  expect(vi.mocked(executePlan).mock.calls[1][2]).not.toBe(oldKey);
});

it("can inspect and durably cancel a restored running execution without a plan", async () => {
  localStorage.setItem("semlia.execution.wsp_a", "run_record");
  const running = result(); running.run.state = "running"; running.availability = "metadata_only"; delete running.rows;
  vi.mocked(getExecution).mockResolvedValue(running);
  vi.mocked(cancelExecution).mockResolvedValue({ ...running, run: { ...running.run, cancelRequested: true } });
  render(<AskExecutionPanel workspaceId="wsp_a"/>);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "请求取消执行" }));
  await waitFor(() => expect(cancelExecution).toHaveBeenCalledWith("wsp_a", "run_record"));
  expect(screen.getByRole("button", { name: "请求取消执行" })).toBeDisabled();
  expect(executePlan).not.toHaveBeenCalled();
});
