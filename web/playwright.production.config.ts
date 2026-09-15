import { defineConfig } from "@playwright/test";

if (!process.env.SEMLIA_ACCEPTANCE_BASE_URL || !process.env.SEMLIA_ACCEPTANCE_ROOT) throw new Error("Use the isolated production-acceptance.sh launcher");

export default defineConfig({
  testDir: "./e2e-production",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  outputDir: `${process.env.SEMLIA_ACCEPTANCE_ROOT}/browser`,
  timeout: 180_000,
  expect: { timeout: 20_000 },
  use: { baseURL: process.env.SEMLIA_ACCEPTANCE_BASE_URL, colorScheme: "light", trace: "retain-on-failure", actionTimeout: 20_000 },
  projects: [
    { name: "desktop", use: { viewport: { width: 1440, height: 900 } } },
    { name: "compact-desktop", use: { viewport: { width: 1024, height: 768 } } },
  ],
});
