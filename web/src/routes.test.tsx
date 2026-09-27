import { describe, expect, it } from "vitest";
import { assetRoute, workOperationRoute, type WorkScope } from "./routes";

describe("exact knowledge and production routes", () => {
  it("retains immutable answer provenance in an asset route", () => {
    const url = new URL(assetRoute("asset/id", { revisionId: "rev_old", releaseId: "release_old" }), "https://local.test");
    expect(url.pathname).toBe("/assets/asset%2Fid");
    expect(url.searchParams.get("revision")).toBe("rev_old");
    expect(url.searchParams.get("release")).toBe("release_old");
  });

  it.each<WorkScope>(["pending", "initiated", "done"])("preserves %s scope alongside release and origin", (scope) => {
    const route = workOperationRoute("operation/id", scope, { productionRelease: "release_old", from: "drafts" });
    const url = new URL(route, "https://local.test");
    expect(url.pathname).toBe("/work/operations/operation%2Fid");
    expect(url.searchParams.get("scope")).toBe(scope === "pending" ? null : scope);
    expect(url.searchParams.get("productionRelease")).toBe("release_old");
    expect(url.searchParams.get("from")).toBe("drafts");
    expect(route.match(/\?/g)).toHaveLength(1);
  });
});
