import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e-knowledge",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: "list",
  outputDir: "test-results/knowledge",
  use: { baseURL: "http://127.0.0.1:4186", colorScheme: "light", reducedMotion: "reduce", trace: "retain-on-failure" },
  projects: [
    { name: "desktop", use: { viewport: { width: 1440, height: 900 } } },
    { name: "compact-desktop", use: { viewport: { width: 1024, height: 768 } } },
  ],
  webServer: { command: "pnpm dev --port 4186 --strictPort", url: "http://127.0.0.1:4186", reuseExistingServer: true, timeout: 30000 },
});
