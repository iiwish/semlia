import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import { StatusView as App } from "./StatusView";
import type { SystemStatus } from "./status";
import { App as ProductEntry } from "./App";
import type { ReactNode } from "react";

const entryRuntime = vi.hoisted(() => ({ workspaceId: "wsp_empty", assets: [], loading: false, workspaces: [{ id: "wsp_empty" }], query: "", error: "" }));
const identity = vi.hoisted(() => ({ accountId: "usr_first", principalId: "prn_first", version: "1" }));
vi.mock("./catalogRuntime", () => ({ CatalogRuntimeProvider: ({ children }: { children: ReactNode }) => <><input aria-label="Synthetic revision cache" defaultValue="" />{children}</>, useCatalogRuntime: () => entryRuntime }));
vi.mock("./CatalogControls", () => ({ CatalogEntryState: () => <div>Catalog entry</div> }));
vi.mock("./ProductApp", () => ({ ProductApp: () => <div>Authorized product shell<input aria-label="Synthetic answer" defaultValue="" /></div> }));
vi.mock("./sessionRuntime", () => ({ SessionRuntimeProvider: ({ children }: { children: ReactNode }) => children, SessionEntryState: () => <div>Session entry</div>, useSessionRuntime: () => ({ phase: "authenticated", session: { account: { id: identity.accountId } }, activeWorkspace: { principalId: identity.principalId }, capabilitySession: { principalId: identity.principalId, version: identity.version } }) }));

it("resets the entire catalog and answer subtree for a new identity but retains same-principal authorization refresh", async () => {
  const view = render(<ProductEntry />);
  await userEvent.type(screen.getByLabelText("Synthetic revision cache"), "private R1");
  await userEvent.type(screen.getByLabelText("Synthetic answer"), "private rows");
  identity.version = "2";
  view.rerender(<ProductEntry />);
  expect(screen.getByLabelText("Synthetic revision cache")).toHaveValue("private R1");
  expect(screen.getByLabelText("Synthetic answer")).toHaveValue("private rows");
  identity.principalId = "prn_second";
  view.rerender(<ProductEntry />);
  expect(screen.getByLabelText("Synthetic revision cache")).toHaveValue("");
  expect(screen.getByLabelText("Synthetic answer")).toHaveValue("");
  await userEvent.type(screen.getByLabelText("Synthetic answer"), "second account rows");
  identity.accountId = "usr_second";
  view.rerender(<ProductEntry />);
  expect(screen.getByLabelText("Synthetic answer")).toHaveValue("");
});

it("mounts the authorized shell in an empty workspace so the first source can be created", () => {
  render(<ProductEntry />);
  expect(screen.getByText("Authorized product shell")).toBeVisible();
  expect(screen.queryByText("Catalog entry")).not.toBeInTheDocument();
});

const traceId = "4bf92f3577b34da6a3ce929d0e0e4736";

const readyStatus: SystemStatus = {
  kind: "ready",
  traceId,
  info: {
    service: "semlia",
    apiVersion: "v1",
    schemaVersion: "0.1.0",
    buildVersion: "test-build",
    traceId,
  },
};

const dependencyStatus: SystemStatus = {
  kind: "dependency_unavailable",
  code: "DEPENDENCY_UNAVAILABLE",
  message: "A required dependency is unavailable.",
  traceId,
  retryable: true,
};

const configurationStatus: SystemStatus = {
  kind: "configuration_error",
  code: "CONFIGURATION_ERROR",
  message: "Unable to reach the control API.",
  traceId: null,
};

describe("system status", () => {
  it("announces the loading state without inventing a result", () => {
    const pending = new Promise<SystemStatus>(() => undefined);
    render(<App loadStatus={() => pending} />);

    expect(screen.getByRole("heading", { name: "System status" })).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("Checking control plane");
    expect(screen.getByRole("button", { name: "Refresh status" })).toBeDisabled();
  });

  it("renders ready system and contract information", async () => {
    render(<App loadStatus={() => Promise.resolve(readyStatus)} />);

    expect(await screen.findByText("Control plane ready")).toBeVisible();
    expect(screen.getByText("test-build")).toBeVisible();
    expect(screen.getByText("v1 / 0.1.0")).toBeVisible();
    expect(screen.getAllByText(traceId).length).toBeGreaterThan(0);
  });

  it("shows a stable dependency error and trace ID", async () => {
    render(<App loadStatus={() => Promise.resolve(dependencyStatus)} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Dependency unavailable");
    expect(screen.getByText("DEPENDENCY_UNAVAILABLE")).toBeVisible();
    expect(screen.getAllByText(traceId).length).toBeGreaterThan(0);
  });

  it("separates configuration failure from dependency health", async () => {
    render(<App loadStatus={() => Promise.resolve(configurationStatus)} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Control API unreachable");
    expect(screen.getByText("CONFIGURATION_ERROR")).toBeVisible();
    expect(screen.getByText("Not available")).toBeVisible();
  });

  it("refreshes through a stable loading state", async () => {
    const user = userEvent.setup();
    const loadStatus = vi
      .fn<() => Promise<SystemStatus>>()
      .mockResolvedValueOnce(readyStatus)
      .mockResolvedValueOnce(dependencyStatus);
    render(<App loadStatus={loadStatus} />);

    await screen.findByText("Control plane ready");
    await user.click(screen.getByRole("button", { name: "Refresh status" }));
    expect(await screen.findByText("Dependency unavailable")).toBeVisible();
    expect(loadStatus).toHaveBeenCalledTimes(2);
  });
});
