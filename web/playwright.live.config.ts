import { defineConfig } from "@playwright/test";

const baseURL = process.env.SEMLIA_LIVE_BASE_URL ?? "http://127.0.0.1:18081";

export default defineConfig({
  testDir: "./e2e-live",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  outputDir: "test-results/live",
  timeout: 90_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL,
    colorScheme: "light",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "desktop", use: { viewport: { width: 1440, height: 900 } } },
    { name: "compact-desktop", use: { viewport: { width: 1024, height: 768 } } },
  ],
});
