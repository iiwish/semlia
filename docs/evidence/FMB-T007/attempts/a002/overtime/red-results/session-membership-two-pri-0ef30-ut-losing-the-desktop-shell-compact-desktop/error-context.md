# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: session-membership.spec.ts >> two principals receive their current member capabilities without losing the desktop shell
- Location: e2e-auth/session-membership.spec.ts:25:1

# Error details

```
Error: expect(received).toEqual(expected) // deep equality

- Expected  -   1
+ Received  + 268

- Array []
+ Array [
+   Object {
+     "description": "Ensure the contrast between foreground and background colors meets WCAG 2 AA minimum contrast ratio thresholds",
+     "help": "Elements must meet minimum color contrast ratio thresholds",
+     "helpUrl": "https://dequeuniversity.com/rules/axe/4.13/color-contrast?application=playwright",
+     "id": "color-contrast",
+     "impact": "serious",
+     "nodes": Array [
+       Object {
+         "all": Array [],
+         "any": Array [
+           Object {
+             "data": Object {
+               "bgColor": "#f2f4f7",
+               "contrastRatio": 4.22,
+               "expectedContrastRatio": "4.5:1",
+               "fgColor": "#69758a",
+               "fontSize": "6.8pt (9px)",
+               "fontWeight": "normal",
+               "messageKey": null,
+             },
+             "id": "color-contrast",
+             "impact": "serious",
+             "message": "Element has insufficient color contrast of 4.22 (foreground color: #69758a, background color: #f2f4f7, font size: 6.8pt (9px), font weight: normal). Expected contrast ratio of 4.5:1",
+             "relatedNodes": Array [
+               Object {
+                 "html": "<div class=\"segment-control\" aria-label=\"成员管理视图\"><button type=\"button\" aria-pressed=\"true\">成员 <span>1</span></button><button type=\"button\" aria-pressed=\"false\">邀请 <span>0</span></button></div>",
+                 "target": Array [
+                   ".segment-control",
+                 ],
+               },
+             ],
+           },
+         ],
+         "failureSummary": "Fix any of the following:
+   Element has insufficient color contrast of 4.22 (foreground color: #69758a, background color: #f2f4f7, font size: 6.8pt (9px), font weight: normal). Expected contrast ratio of 4.5:1",
+         "html": "<button type=\"button\" aria-pressed=\"false\">邀请 <span>0</span></button>",
+         "impact": "serious",
+         "none": Array [],
+         "target": Array [
+           ".segment-control > button:nth-child(2)",
+         ],
+       },
+       Object {
+         "all": Array [],
+         "any": Array [
+           Object {
+             "data": Object {
+               "bgColor": "#f8f9fb",
+               "contrastRatio": 4.41,
+               "expectedContrastRatio": "4.5:1",
+               "fgColor": "#69758a",
+               "fontSize": "5.3pt (7px)",
+               "fontWeight": "bold",
+               "messageKey": null,
+             },
+             "id": "color-contrast",
+             "impact": "serious",
+             "message": "Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+             "relatedNodes": Array [
+               Object {
+                 "html": "<div class=\"member-admin-head\"><span>成员</span><span>授权角色</span><span>加入时间</span><span>状态</span><span>操作</span></div>",
+                 "target": Array [
+                   ".member-admin-head",
+                 ],
+               },
+             ],
+           },
+         ],
+         "failureSummary": "Fix any of the following:
+   Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+         "html": "<span>成员</span>",
+         "impact": "serious",
+         "none": Array [],
+         "target": Array [
+           ".member-admin-head > span:nth-child(1)",
+         ],
+       },
+       Object {
+         "all": Array [],
+         "any": Array [
+           Object {
+             "data": Object {
+               "bgColor": "#f8f9fb",
+               "contrastRatio": 4.41,
+               "expectedContrastRatio": "4.5:1",
+               "fgColor": "#69758a",
+               "fontSize": "5.3pt (7px)",
+               "fontWeight": "bold",
+               "messageKey": null,
+             },
+             "id": "color-contrast",
+             "impact": "serious",
+             "message": "Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+             "relatedNodes": Array [
+               Object {
+                 "html": "<div class=\"member-admin-head\"><span>成员</span><span>授权角色</span><span>加入时间</span><span>状态</span><span>操作</span></div>",
+                 "target": Array [
+                   ".member-admin-head",
+                 ],
+               },
+             ],
+           },
+         ],
+         "failureSummary": "Fix any of the following:
+   Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+         "html": "<span>授权角色</span>",
+         "impact": "serious",
+         "none": Array [],
+         "target": Array [
+           ".member-admin-head > span:nth-child(2)",
+         ],
+       },
+       Object {
+         "all": Array [],
+         "any": Array [
+           Object {
+             "data": Object {
+               "bgColor": "#f8f9fb",
+               "contrastRatio": 4.41,
+               "expectedContrastRatio": "4.5:1",
+               "fgColor": "#69758a",
+               "fontSize": "5.3pt (7px)",
+               "fontWeight": "bold",
+               "messageKey": null,
+             },
+             "id": "color-contrast",
+             "impact": "serious",
+             "message": "Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+             "relatedNodes": Array [
+               Object {
+                 "html": "<div class=\"member-admin-head\"><span>成员</span><span>授权角色</span><span>加入时间</span><span>状态</span><span>操作</span></div>",
+                 "target": Array [
+                   ".member-admin-head",
+                 ],
+               },
+             ],
+           },
+         ],
+         "failureSummary": "Fix any of the following:
+   Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+         "html": "<span>状态</span>",
+         "impact": "serious",
+         "none": Array [],
+         "target": Array [
+           ".member-admin-head > span:nth-child(4)",
+         ],
+       },
+       Object {
+         "all": Array [],
+         "any": Array [
+           Object {
+             "data": Object {
+               "bgColor": "#f8f9fb",
+               "contrastRatio": 4.41,
+               "expectedContrastRatio": "4.5:1",
+               "fgColor": "#69758a",
+               "fontSize": "5.3pt (7px)",
+               "fontWeight": "bold",
+               "messageKey": null,
+             },
+             "id": "color-contrast",
+             "impact": "serious",
+             "message": "Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+             "relatedNodes": Array [
+               Object {
+                 "html": "<div class=\"member-admin-head\"><span>成员</span><span>授权角色</span><span>加入时间</span><span>状态</span><span>操作</span></div>",
+                 "target": Array [
+                   ".member-admin-head",
+                 ],
+               },
+             ],
+           },
+         ],
+         "failureSummary": "Fix any of the following:
+   Element has insufficient color contrast of 4.41 (foreground color: #69758a, background color: #f8f9fb, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+         "html": "<span>操作</span>",
+         "impact": "serious",
+         "none": Array [],
+         "target": Array [
+           ".member-admin-head > span:nth-child(5)",
+         ],
+       },
+       Object {
+         "all": Array [],
+         "any": Array [
+           Object {
+             "data": Object {
+               "bgColor": "#fefefe",
+               "contrastRatio": 4.27,
+               "expectedContrastRatio": "4.5:1",
+               "fgColor": "#4d75da",
+               "fontSize": "5.3pt (7px)",
+               "fontWeight": "normal",
+               "messageKey": null,
+             },
+             "id": "color-contrast",
+             "impact": "serious",
+             "message": "Element has insufficient color contrast of 4.27 (foreground color: #4d75da, background color: #fefefe, font size: 5.3pt (7px), font weight: normal). Expected contrast ratio of 4.5:1",
+             "relatedNodes": Array [
+               Object {
+                 "html": "<section class=\"member-admin-table\" aria-label=\"工作区成员列表\">",
+                 "target": Array [
+                   ".member-admin-table",
+                 ],
+               },
+             ],
+           },
+         ],
+         "failureSummary": "Fix any of the following:
+   Element has insufficient color contrast of 4.27 (foreground color: #4d75da, background color: #fefefe, font size: 5.3pt (7px), font weight: normal). Expected contrast ratio of 4.5:1",
+         "html": "<small>当前账户</small>",
+         "impact": "serious",
+         "none": Array [],
+         "target": Array [
+           "strong > small",
+         ],
+       },
+       Object {
+         "all": Array [],
+         "any": Array [
+           Object {
+             "data": Object {
+               "bgColor": "#e9f6f3",
+               "contrastRatio": 4.06,
+               "expectedContrastRatio": "4.5:1",
+               "fgColor": "#2a8475",
+               "fontSize": "5.3pt (7px)",
+               "fontWeight": "bold",
+               "messageKey": null,
+             },
+             "id": "color-contrast",
+             "impact": "serious",
+             "message": "Element has insufficient color contrast of 4.06 (foreground color: #2a8475, background color: #e9f6f3, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+             "relatedNodes": Array [
+               Object {
+                 "html": "<span class=\"member-state member-state-active\">有效</span>",
+                 "target": Array [
+                   ".member-state",
+                 ],
+               },
+             ],
+           },
+         ],
+         "failureSummary": "Fix any of the following:
+   Element has insufficient color contrast of 4.06 (foreground color: #2a8475, background color: #e9f6f3, font size: 5.3pt (7px), font weight: bold). Expected contrast ratio of 4.5:1",
+         "html": "<span class=\"member-state member-state-active\">有效</span>",
+         "impact": "serious",
+         "none": Array [],
+         "target": Array [
+           ".member-state",
+         ],
+       },
+     ],
+     "tags": Array [
+       "cat.color",
+       "wcag2aa",
+       "wcag143",
+       "TTv5",
+       "TT13.c",
+       "EN-301-549",
+       "EN-9.1.4.3",
+       "ACT",
+       "RGAAv4",
+       "RGAA-3.2.1",
+     ],
+   },
+ ]
```

# Page snapshot

```yaml
- generic [ref=e3]:
  - complementary [ref=e4]:
    - button "返回语义问答" [ref=e5] [cursor=pointer]
    - navigation "Semlia 主功能" [ref=e15]:
      - button "语义问答" [ref=e16] [cursor=pointer]
      - button "工作台" [ref=e20] [cursor=pointer]
      - button "知识资产" [ref=e27] [cursor=pointer]
      - button "变更与发布" [ref=e39] [cursor=pointer]
      - button "数据接入" [ref=e46] [cursor=pointer]
    - navigation "Semlia 平台管理" [ref=e52]:
      - button "系统设置" [active] [ref=e53] [cursor=pointer]
  - complementary "治理上下文" [ref=e58]:
    - generic [ref=e59]:
      - generic [ref=e60]: 平台管理
      - button "收起二级菜单" [ref=e61] [cursor=pointer]
    - generic [ref=e65]:
      - button "成员 工作区成员与邀请" [ref=e66] [cursor=pointer]:
        - generic [ref=e68]:
          - strong [ref=e69]: 成员
          - generic [ref=e70]: 工作区成员与邀请
      - button "模型配置 LLM · Embedding" [ref=e73] [cursor=pointer]:
        - generic [ref=e75]:
          - strong [ref=e76]: 模型配置
          - generic [ref=e77]: LLM · Embedding
      - button "接口与集成 4 种接口" [ref=e80] [cursor=pointer]:
        - generic [ref=e82]:
          - strong [ref=e83]: 接口与集成
          - generic [ref=e84]: 4 种接口
    - separator "调整二级菜单宽度" [ref=e87]
  - generic [ref=e88]:
    - banner [ref=e89]:
      - strong [ref=e91]: 成员
      - generic [ref=e92]:
        - generic [ref=e93]:
          - generic [ref=e94]:
            - generic [ref=e95]: 工作区
            - combobox "工作区" [ref=e96]:
              - option "Alpha Workspace" [selected]
          - button "刷新真实目录" [ref=e97] [cursor=pointer]
        - group [ref=e103]:
          - generic "账户 Alpha Admin" [ref=e104] [cursor=pointer]:
            - strong [ref=e109]: Alpha Admin
    - main [ref=e112]:
      - region "成员管理" [ref=e113]:
        - generic [ref=e114]:
          - generic "成员管理视图" [ref=e115]:
            - button "成员 1" [pressed] [ref=e116] [cursor=pointer]
            - button "邀请 0" [ref=e117] [cursor=pointer]
          - searchbox "搜索成员与邀请" [ref=e122]
          - button "刷新成员与邀请" [ref=e123] [cursor=pointer]
          - button "邀请成员" [ref=e129] [cursor=pointer]
        - region "工作区成员列表" [ref=e133]:
          - generic [ref=e134]:
            - generic [ref=e135]: 成员
            - generic [ref=e136]: 授权角色
            - generic [ref=e137]: 状态
            - generic [ref=e138]: 操作
          - generic [ref=e140]:
            - generic [ref=e141]:
              - strong [ref=e142]: Alpha Admin当前账户
              - code [ref=e143]: prn_01arz3ndektsv4rrffq69g5fav
            - generic [ref=e144]: Workspace Admin
            - generic [ref=e146]: 有效
            - generic [ref=e147]:
              - button "停用 Alpha Admin" [ref=e148] [cursor=pointer]
              - button "移除 Alpha Admin" [ref=e151] [cursor=pointer]
```

# Test source

```ts
  1   | import AxeBuilder from "@axe-core/playwright";
  2   | import { expect, test, type Page, type Route } from "@playwright/test";
  3   | import { mkdirSync } from "node:fs";
  4   | import { resolve } from "node:path";
  5   | 
  6   | const evidenceDirectory = process.env.SEMLIA_BROWSER_EVIDENCE_DIR ?? resolve(process.cwd(), "../docs/evidence/ALPHA-T002/screenshots");
  7   | const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
  8   | 
  9   | test.beforeAll(() => mkdirSync(evidenceDirectory, { recursive: true }));
  10  | 
  11  | test("unauthenticated users see a keyboard-accessible sign-in state", async ({ page }, testInfo) => {
  12  |   await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ code: "UNAUTHENTICATED", message: "session required", traceId: "trace-sign-in", details: {} }) }));
  13  |   await page.goto("/");
  14  | 
  15  |   const login = page.getByRole("link", { name: "使用组织账户登录" });
  16  |   await expect(page.getByRole("heading", { name: "登录工作区" })).toBeVisible();
  17  |   await login.focus();
  18  |   await expect(login).toBeFocused();
  19  |   await expect(page.getByLabel("Semlia 主功能")).toHaveCount(0);
  20  |   const accessibility = await new AxeBuilder({ page }).include(".session-entry-content").analyze();
  21  |   expect(accessibility.violations).toEqual([]);
  22  |   await page.screenshot({ path: resolve(evidenceDirectory, `${testInfo.project.name}-sign-in.png`), fullPage: true });
  23  | });
  24  | 
  25  | test("two principals receive their current member capabilities without losing the desktop shell", async ({ page }, testInfo) => {
  26  |   let roleId = "workspace_admin";
  27  |   await installAuthenticatedRoutes(page, () => roleId);
  28  |   await page.goto("/");
  29  | 
  30  |   await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
  31  |   await expect(page.getByRole("button", { name: "知识资产" })).toBeVisible();
  32  |   await expect(page.getByRole("button", { name: "数据接入" })).toBeVisible();
  33  |   await expect(page.getByLabel("账户 Alpha Admin")).toBeVisible();
  34  |   await page.getByRole("button", { name: "系统设置" }).click();
  35  |   await expect(page.getByRole("region", { name: "工作区成员列表" })).toBeVisible();
  36  |   await expect(page.getByRole("button", { name: "邀请成员" })).toBeVisible();
  37  |   await expect(page.getByRole("button", { name: "停用 Alpha Admin" })).toBeVisible();
  38  | 
  39  |   const clipped = await page.locator(".topbar, .member-admin-toolbar, .member-admin-table").evaluateAll((elements) => elements.filter((element) => element.scrollWidth > element.clientWidth + 1).length);
  40  |   expect(clipped).toBe(0);
  41  |   // Sample the actual entry animation rather than letting a fast fade evade Axe.
  42  |   const entryStyles = await page.locator(".member-state-active").evaluate((badge) => {
  43  |     const ancestors = [];
  44  |     for (let element: Element | null = badge; element; element = element.parentElement) {
  45  |       for (const animation of element.getAnimations()) {
  46  |         const duration = animation.effect?.getTiming().duration;
  47  |         if (typeof duration === "number" && duration > 0) {
  48  |           animation.pause();
  49  |           animation.currentTime = duration / 3;
  50  |         }
  51  |       }
  52  |       const style = getComputedStyle(element);
  53  |       ancestors.push({ className: element.className, opacity: style.opacity, color: style.color, backgroundColor: style.backgroundColor, animationName: style.animationName, animationDuration: style.animationDuration });
  54  |     }
  55  |     return ancestors;
  56  |   });
  57  |   await testInfo.attach("member-entry-computed-styles", { body: JSON.stringify(entryStyles, null, 2), contentType: "application/json" });
  58  |   const accessibility = await new AxeBuilder({ page }).include(".member-administration-view").analyze();
> 59  |   expect(accessibility.violations).toEqual([]);
      |                                    ^ Error: expect(received).toEqual(expected) // deep equality
  60  |   expect(entryStyles.every((style) => style.opacity === "1")).toBe(true);
  61  |   await page.screenshot({ path: resolve(evidenceDirectory, `${testInfo.project.name}-member-admin.png`), fullPage: true });
  62  | 
  63  |   roleId = "auditor";
  64  |   await page.emulateMedia({ reducedMotion: "reduce" });
  65  |   await page.reload();
  66  |   await page.getByRole("button", { name: "系统设置" }).click();
  67  |   await expect(page.getByRole("region", { name: "工作区成员列表" })).toBeVisible();
  68  |   await expect(page.getByRole("button", { name: "邀请成员" })).toHaveCount(0);
  69  |   await expect(page.getByRole("button", { name: "停用 Alpha Admin" })).toHaveCount(0);
  70  |   await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
  71  |   const reducedDuration = await page.locator(".member-administration-view").evaluate((element) => Number.parseFloat(getComputedStyle(element).animationDuration));
  72  |   expect(reducedDuration).toBeLessThanOrEqual(0.00001);
  73  | });
  74  | 
  75  | test("an administrator can invite a member and logout with the session CSRF verifier", async ({ page }) => {
  76  |   let invitationHeader = "";
  77  |   let logoutHeader = "";
  78  |   await installAuthenticatedRoutes(page, () => "workspace_admin", (route) => {
  79  |     invitationHeader = route.request().headers()["x-semlia-csrf"] ?? "";
  80  |   }, (route) => {
  81  |     logoutHeader = route.request().headers()["x-semlia-csrf"] ?? "";
  82  |   });
  83  |   await page.goto("/");
  84  |   await page.getByRole("button", { name: "系统设置" }).click();
  85  |   await page.getByRole("button", { name: "邀请成员" }).click();
  86  |   const dialog = page.getByRole("dialog", { name: "邀请工作区成员" });
  87  |   await dialog.getByLabel("组织邮箱").fill("reviewer@example.com");
  88  |   await dialog.getByLabel("初始角色").selectOption("reviewer");
  89  |   await dialog.getByRole("button", { name: "创建邀请" }).click();
  90  |   await expect(page.getByText("reviewer@example.com")).toBeVisible();
  91  |   expect(invitationHeader).toBe("csrf-verifier-value-for-browser-tests-123456");
  92  | 
  93  |   await page.getByLabel("账户 Alpha Admin").click();
  94  |   await page.getByRole("button", { name: "退出登录" }).click();
  95  |   await expect(page.getByRole("heading", { name: "登录工作区" })).toBeVisible();
  96  |   expect(logoutHeader).toBe("csrf-verifier-value-for-browser-tests-123456");
  97  | });
  98  | 
  99  | async function installAuthenticatedRoutes(page: Page, role: () => string, onInvite?: (route: Route) => void, onLogout?: (route: Route) => void) {
  100 |   await page.route("**/api/v1/**", async (route) => {
  101 |     const request = route.request();
  102 |     const url = new URL(request.url());
  103 |     const path = url.pathname;
  104 |     if (path === "/api/v1/session" && request.method() === "GET") {
  105 |       return route.fulfill({ status: 200, headers: { "Content-Type": "application/json", "X-Semlia-CSRF": "csrf-verifier-value-for-browser-tests-123456" }, body: JSON.stringify(session(role())) });
  106 |     }
  107 |     if (path === "/api/v1/session" && request.method() === "DELETE") {
  108 |       onLogout?.(route);
  109 |       return route.fulfill({ status: 204 });
  110 |     }
  111 |     if (path === "/api/v1/workspaces") return json(route, { items: [workspace()] });
  112 |     if (path.endsWith("/catalog/assets")) return json(route, { items: [asset()], page: { limit: 100 } });
  113 |     if (path.endsWith("/members")) return json(route, { items: [member()] });
  114 |     if (path.endsWith("/invitations") && request.method() === "GET") return json(route, { items: [] });
  115 |     if (path.endsWith("/invitations") && request.method() === "POST") {
  116 |       onInvite?.(route);
  117 |       const body = request.postDataJSON() as { email: string; roleId: string };
  118 |       return json(route, { id: "ivn_01arz3ndektsv4rrffq69g5fav", issuer: "https://issuer.example", email: body.email, roleId: body.roleId, status: "pending", expiresAt: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(), createdAt: "2026-09-04T10:00:00Z" }, 201);
  119 |     }
  120 |     if (path.includes("/governance/")) return json(route, { items: [], page: { limit: 100 } });
  121 |     return json(route, { code: "NOT_FOUND", message: "not found", traceId: "trace-not-found", details: {} }, 404);
  122 |   });
  123 | }
  124 | 
  125 | function session(roleId: string) {
  126 |   const capabilities = ["workspace.read", "asset.read", "member.read", ...(roleId === "workspace_admin" ? ["member.manage"] : [])];
  127 |   return { account: { id: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Alpha Admin" }, workspaces: [{ id: workspaceId, slug: "alpha", displayName: "Alpha Workspace", principalId: "prn_01arz3ndektsv4rrffq69g5fav", roleIds: [roleId], capabilities, authorizationVersion: roleId === "auditor" ? 4 : 3 }], expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(), traceId: "trace-session" };
  128 | }
  129 | 
  130 | function workspace() {
  131 |   return { id: workspaceId, slug: "alpha", displayName: "Alpha Workspace", createdAt: "2026-09-04T08:00:00Z", updatedAt: "2026-09-04T08:00:00Z" };
  132 | }
  133 | 
  134 | function asset() {
  135 |   return { id: "ast_01arz3ndektsv4rrffq69g5fav", address: "commerce.net_revenue", assetType: "metric", lifecycleState: "active", currentRevisionId: "rev_01arz3ndektsv4rrffq69g5fav", title: "Net revenue", summary: "Revenue after refunds and adjustments.", updatedAt: "2026-09-04T08:00:00Z" };
  136 | }
  137 | 
  138 | function member() {
  139 |   return { id: "mbr_01arz3ndektsv4rrffq69g5fav", accountId: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Alpha Admin", principalId: "prn_01arz3ndektsv4rrffq69g5fav", status: "active", roleIds: ["workspace_admin"], admittedAt: "2026-09-04T08:00:00Z" };
  140 | }
  141 | 
  142 | function json(route: Route, body: unknown, status = 200) {
  143 |   return route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
  144 | }
  145 | 
```