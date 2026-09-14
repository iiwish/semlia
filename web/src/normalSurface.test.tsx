import { expect, it } from "vitest";
import product from "./ProductApp.tsx?raw";
import catalog from "./catalogRuntime.tsx?raw";
import authorization from "./authorizationAdminRuntime.tsx?raw";
import access from "./AccessControlView.tsx?raw";
import capabilities from "./authorization.tsx?raw";
import session from "./sessionRuntime.tsx?raw";

it("keeps prototype modes and identities outside production surfaces", () => {
  for (const source of [product, catalog, authorization, access, capabilities, session]) {
    expect(source).not.toMatch(/fixtureGovernance|fixtureAssets|fixtureDefaultAccess|runtime\.fixture|createAuthorizationFixtureApi|EMP-10001/);
    expect(source).not.toMatch(/from ["']\.\/testing\//);
  }
});
