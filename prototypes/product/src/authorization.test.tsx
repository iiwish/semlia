import { render, screen } from "@testing-library/react";

import {
  CapabilityProvider,
  RequireCapability,
  evaluateAuthorization,
  findSeparationOfDutyConflicts,
  useCan,
  useResourceCan,
} from "./authorization";
import { authorizationBindings, authorizationPrincipals, authorizationRoles, authorizationSession } from "./data";
import type { CapabilitySession } from "./types";

function CapabilityProbe() {
  const canReadRoles = useCan("role.read");
  const canPublishProtectedAsset = useResourceCan("release.publish", {
    type: "asset",
    id: "METRIC-NET-REVENUE",
    domainId: "commerce",
    environment: "production",
    protected: true,
  });

  return (
    <>
      <output>{canReadRoles ? "可查看角色" : "不可查看角色"}</output>
      <output>{canPublishProtectedAsset.allowed ? "可发布" : canPublishProtectedAsset.explanation}</output>
    </>
  );
}

describe("authorization projection", () => {
  it("exposes action and resource decisions without checking role display names", () => {
    render(
      <CapabilityProvider>
        <CapabilityProbe />
        <RequireCapability action="audit.read" fallback={<p>无审计权限</p>}>
          <p>审计记录</p>
        </RequireCapability>
      </CapabilityProvider>,
    );

    expect(screen.getByText("可查看角色")).toBeVisible();
    expect(screen.getByText("可发布")).toBeVisible();
    expect(screen.getByText("审计记录")).toBeVisible();
  });

  it("updates consumers when the capability session changes", () => {
    const restrictedSession: CapabilitySession = {
      ...authorizationSession,
      version: "authzv-test-restricted",
      capabilities: ["workspace.read"],
    };
    const { rerender } = render(
      <CapabilityProvider session={restrictedSession}>
        <CapabilityProbe />
      </CapabilityProvider>,
    );

    expect(screen.getByText("不可查看角色")).toBeVisible();
    rerender(
      <CapabilityProvider session={authorizationSession}>
        <CapabilityProbe />
      </CapabilityProvider>,
    );
    expect(screen.getByText("可查看角色")).toBeVisible();
  });

  it("returns explainable allow and deny decisions", () => {
    const allowed = evaluateAuthorization({
      principalId: authorizationSession.principalId,
      action: "role.assign",
      resource: { type: "workspace", id: "WS-SEMLIA" },
    });
    const denied = evaluateAuthorization({
      principalId: "USR-AUDITOR",
      action: "release.publish",
      resource: { type: "asset", id: "METRIC-NET-REVENUE", domainId: "commerce", protected: true },
    });

    expect(allowed).toMatchObject({ allowed: true, reasonCode: "ROLE_GRANT", authorizationVersion: authorizationSession.version });
    expect(allowed.explanation).toContain("Workspace Admin");
    expect(denied).toMatchObject({ allowed: false, reasonCode: "NO_MATCHING_GRANT" });
    expect(denied.explanation).toContain("release.publish");
  });

  it("detects protected-scope reviewer and publisher conflicts", () => {
    const conflicts = findSeparationOfDutyConflicts({
      principalId: "USR-REVIEWER",
      roleId: "ROLE-PUBLISHER",
      scope: { type: "asset", id: "METRIC-NET-REVENUE", label: "净收入", protected: true },
      roles: authorizationRoles,
      bindings: authorizationBindings,
    });

    expect(conflicts).toHaveLength(1);
    expect(conflicts[0]).toMatchObject({ code: "PROTECTED_REVIEW_PUBLISH_CONFLICT", blocking: true });
    expect(conflicts[0].message).toContain("评审者与发布者必须相互独立");
    expect(authorizationPrincipals.find((principal) => principal.id === "USR-REVIEWER")?.kind).toBe("user");
  });
});
