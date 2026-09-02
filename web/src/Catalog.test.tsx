import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import { App } from "./App";

const { workspace, asset } = vi.hoisted(() => ({
  workspace: {
    id: "wsp_01arz3ndektsv4rrffq69g5fav",
    slug: "semantic-core",
    displayName: "Semantic Core",
    createdAt: "2026-09-02T04:00:00Z",
    updatedAt: "2026-09-02T04:00:00Z",
  },
  asset: {
    id: "ast_01arz3ndektsv4rrffq69g5fav",
    address: "commerce.net_revenue",
    assetType: "metric" as const,
    lifecycleState: "active" as const,
    currentRevisionId: "rev_01arz3ndektsv4rrffq69g5fav",
    title: "Net revenue",
    summary: "Revenue after refunds and adjustments.",
    updatedAt: "2026-09-02T04:00:00Z",
  },
}));

vi.mock("./catalog", () => ({
  listWorkspaces: vi.fn().mockResolvedValue([workspace]),
  listAssets: vi.fn().mockResolvedValue({ items: [asset], page: { limit: 100 } }),
  getAsset: vi.fn().mockResolvedValue({
    ...asset,
    createdAt: asset.updatedAt,
    relationCount: 2,
    currentRevision: {
      id: asset.currentRevisionId,
      assetId: asset.id,
      sequence: 3,
      schemaVersion: "1.0.0",
      contentDigest: "a".repeat(64),
      content: { title: asset.title, summary: asset.summary },
      createdBy: "catalog-web",
      createdAt: asset.updatedAt,
      evidence: [],
    },
  }),
  listAssetRelations: vi.fn().mockResolvedValue({ items: [], maxDepth: 1 }),
  createWorkspace: vi.fn(),
  createAsset: vi.fn(),
}));

describe("production catalog", () => {
  it("loads a real registry view and opens asset detail", async () => {
    const user = userEvent.setup();
    render(<App />);

    expect(await screen.findByRole("button", { name: "知识资产" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "知识资产" }));
    expect((await screen.findAllByText("Net revenue")).length).toBeGreaterThan(0);
    expect(screen.getByText("net_revenue")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "打开语义资产 Net revenue" }));
    expect(await screen.findByRole("heading", { name: "Net revenue" })).toBeVisible();
    expect(screen.getByText("@3")).toBeVisible();
  });
});
