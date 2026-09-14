import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { SessionEntryState, SessionRuntimeProvider } from "./sessionRuntime";

afterEach(() => vi.unstubAllGlobals());

it("submits passwords only in the login body and reports generic credential failures", async () => {
  const requests: Request[] = [];
  vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
    requests.push(request);
    if (request.url.endsWith("/auth/methods")) return Response.json({ password: true, oidc: false });
    return Response.json({ code: "UNAUTHENTICATED", message: "authentication is required" }, { status: 401 });
  }));
  render(<SessionRuntimeProvider><SessionEntryState /></SessionRuntimeProvider>);
  const user = userEvent.setup();
  await user.type(await screen.findByLabelText("登录账号"), "local@example.com");
  await user.type(screen.getByLabelText("密码"), "Synthetic test password 123");
  await user.click(screen.getByRole("button", { name: "登录" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("账号或密码不正确");
  expect(screen.getByLabelText("密码")).toHaveValue("");
  const request = requests.find((r) => r.method === "POST");
  expect(request?.url).not.toContain("Synthetic");
  expect(await request?.json()).toEqual({ username: "local@example.com", password: "Synthetic test password 123" });
  expect(screen.queryByRole("link", { name: "使用组织账户登录" })).not.toBeInTheDocument();
});
