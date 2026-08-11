import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import { App } from "./App";
import type { SystemStatus } from "./status";

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
