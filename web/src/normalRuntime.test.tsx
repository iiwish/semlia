import { render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { App } from "./App";

afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); });

it("requires a real session even when legacy environment flags are supplied", async () => {
  vi.stubEnv("VITE_LOCAL_UAT_IDENTITIES", "1");
  vi.stubEnv("VITE_CATALOG_FIXTURE", "1");
  const requests: Request[] = [];
  vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
    requests.push(request);
    if (request.url.endsWith("/auth/methods")) return Response.json({ password: true, oidc: false });
    return Response.json({ code: "UNAUTHENTICATED" }, { status: 401 });
  }));
  render(<App />);
  expect(await screen.findByLabelText("登录账号")).toBeVisible();
  expect(screen.queryByLabelText("本机验收身份")).not.toBeInTheDocument();
  expect(requests.every((request) => !request.headers.has("X-Semlia-Principal"))).toBe(true);
});
