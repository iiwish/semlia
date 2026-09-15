import { render, screen } from "@testing-library/react";
import { afterEach, vi } from "vitest";

import { clearSessionCSRFToken } from "./apiClient";
import { CapabilityProvider, useCan } from "./authorization";
import { SessionEntryState, SessionRuntimeProvider, useSessionRuntime } from "./sessionRuntime";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const principalId = "prn_01arz3ndektsv4rrffq69g5fav";

afterEach(() => {
  vi.unstubAllGlobals();
  clearSessionCSRFToken();
  window.sessionStorage.clear();
});

describe("server-projected authorization session", () => {
  it("uses effective capabilities for custom roles instead of a static role-name map", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      account: { id: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Custom Role User" },
      workspaces: [{
        id: workspaceId,
        slug: "alpha",
        displayName: "Alpha",
        principalId,
        roleIds: ["custom_catalog_reader"],
        capabilities: ["workspace.read", "asset.read"],
        authorizationVersion: 12,
      }],
      expiresAt: "2026-09-04T18:00:00Z",
      traceId: "trace-session-custom",
    }), { status: 200, headers: { "Content-Type": "application/json", "X-Semlia-CSRF": "csrf-session-custom-role-tests-123456" } })));

    render(<SessionRuntimeProvider><SessionCapabilityProbe /></SessionRuntimeProvider>);

    expect(await screen.findByText("可读取资产")).toBeVisible();
    expect(screen.getByText("不可管理角色")).toBeVisible();
    expect(screen.getByTestId("authorization-version")).toHaveTextContent("12");
  });
});

function SessionCapabilityProbe() {
  const runtime = useSessionRuntime();
  if (!runtime.capabilitySession) return <SessionEntryState />;
  return <CapabilityProvider session={runtime.capabilitySession}><CapabilityValues version={runtime.capabilitySession.version} /></CapabilityProvider>;
}

function CapabilityValues({ version }: { version: string }) {
  const canReadAssets = useCan("asset.read");
  const canManageRoles = useCan("role.manage");
  return <><span>{canReadAssets ? "可读取资产" : "不可读取资产"}</span><span>{canManageRoles ? "可管理角色" : "不可管理角色"}</span><output data-testid="authorization-version">{version}</output></>;
}
