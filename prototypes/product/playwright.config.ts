import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:4176",
    colorScheme: "light",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "desktop", use: { viewport: { width: 1440, height: 900 } } },
    { name: "compact-desktop", use: { viewport: { width: 1024, height: 768 } } },
  ],
  webServer: {
    command: "pnpm dev --port 4176",
    url: "http://127.0.0.1:4176",
    reuseExistingServer: true,
    timeout: 30_000,
  },
});
