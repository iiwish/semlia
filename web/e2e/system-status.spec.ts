import { expect, test, type Page, type Route } from "@playwright/test";

const traceId = "4bf92f3577b34da6a3ce929d0e0e4736";

test("renders the real ready contract without overflow", async ({ page }, testInfo) => {
  await routeReady(page);
  await page.goto("/");

  await expect(page.getByText("Control plane ready")).toBeVisible();
  await expect(page.getByText("test-build")).toBeVisible();
  await expect(page.getByText(traceId).first()).toBeVisible();
  await expectNoOverflow(page);
  await page.screenshot({ path: testInfo.outputPath("ready.png"), fullPage: true });
});

test("shows dependency error code and trace", async ({ page }, testInfo) => {
  await routeDependencyUnavailable(page);
  await page.goto("/");

  await expect(page.getByRole("alert")).toContainText("Dependency unavailable");
  await expect(page.getByText("DEPENDENCY_UNAVAILABLE")).toBeVisible();
  await expect(page.getByText(traceId).first()).toBeVisible();
  await expectNoOverflow(page);
  await page.screenshot({ path: testInfo.outputPath("dependency-unavailable.png"), fullPage: true });
});

test("shows a safe configuration error when the API is unreachable", async ({ page }, testInfo) => {
  await page.route("**/health/live", (route) => route.abort("connectionrefused"));
  await page.goto("/");

  await expect(page.getByRole("alert")).toContainText("Control API unreachable");
  await expect(page.getByText("CONFIGURATION_ERROR")).toBeVisible();
  await expect(page.getByText("Not available")).toBeVisible();
  await expectNoOverflow(page);
  await page.screenshot({ path: testInfo.outputPath("configuration-error.png"), fullPage: true });
});

test("supports keyboard refresh and reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  let livenessRequests = 0;
  let holdRefresh = false;
  let releaseRefresh!: () => void;
  const refreshBarrier = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  await page.route("**/health/live", async (route) => {
    livenessRequests += 1;
    if (holdRefresh) {
      await refreshBarrier;
    }
    await json(route, 200, { status: "live", traceId });
  });
  await routeReadyRemainder(page);
  await page.goto("/");
  await expect(page.getByText("Control plane ready")).toBeVisible();
  const initialRequests = livenessRequests;

  const refresh = page.getByRole("button", { name: "Refresh status" });
  await refresh.focus();
  await expect(refresh).toBeFocused();
  const outline = await refresh.evaluate((element) => getComputedStyle(element).outlineStyle);
  expect(outline).not.toBe("none");
  holdRefresh = true;
  await page.keyboard.press("Enter");
  await expect(page.getByText("Checking control plane")).toBeVisible();
  await expect(page.locator(".is-spinning")).toHaveCSS("animation-name", "none");
  releaseRefresh();
  await expect(page.getByText("Control plane ready")).toBeVisible();
  expect(livenessRequests).toBe(initialRequests + 1);
});

async function routeReady(page: Page) {
  await page.route("**/health/live", (route) => json(route, 200, { status: "live", traceId }));
  await routeReadyRemainder(page);
}

async function routeReadyRemainder(page: Page) {
  await page.route("**/health/ready", (route) => json(route, 200, { status: "ready", traceId }));
  await page.route("**/api/v1/system/info", (route) =>
    json(route, 200, {
      service: "semlia",
      apiVersion: "v1",
      schemaVersion: "0.1.0",
      buildVersion: "test-build",
      traceId,
    }),
  );
}

async function routeDependencyUnavailable(page: Page) {
  await page.route("**/health/live", (route) => json(route, 200, { status: "live", traceId }));
  await page.route("**/health/ready", (route) =>
    json(route, 503, {
      code: "DEPENDENCY_UNAVAILABLE",
      message: "A required dependency is unavailable.",
      traceId,
      details: {},
      retryable: true,
    }),
  );
  await page.route("**/api/v1/system/info", (route) =>
    json(route, 200, {
      service: "semlia",
      apiVersion: "v1",
      schemaVersion: "0.1.0",
      buildVersion: "test-build",
      traceId,
    }),
  );
}

async function json(route: Route, status: number, body: object) {
  await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}

async function expectNoOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    content: document.documentElement.scrollWidth,
  }));
  expect(dimensions.content).toBeLessThanOrEqual(dimensions.viewport);
}
