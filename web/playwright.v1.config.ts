import { defineConfig } from "@playwright/test";

const root = process.env.SEMLIA_V1_BROWSER_ROOT;
const baseURL = process.env.SEMLIA_V1_BROWSER_BASE_URL;
const runId = process.env.SEMLIA_V1_BROWSER_RUN_ID;
if (!root || !baseURL || !/^http:\/\/127\.0\.0\.1:\d+$/.test(baseURL) || !/^\d{13}$/.test(runId ?? "")) throw new Error("Use the owned V1 browser acceptance launcher");

export default defineConfig({
  testDir: "./e2e-v1",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  outputDir: `${root}/browser/${runId}/playwright`,
  timeout: 300_000,
  expect: { timeout: 20_000 },
  use: { baseURL, colorScheme: "light", trace: "retain-on-failure", actionTimeout: 20_000, navigationTimeout: 30_000, screenshot: "only-on-failure" },
  projects: [
    { name: "desktop", use: { viewport: { width: 1440, height: 900 } } },
    { name: "compact-desktop", use: { viewport: { width: 1024, height: 768 } } },
  ],
});
