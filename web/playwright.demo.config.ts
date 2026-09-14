import { defineConfig } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

process.umask(0o077);
const root = resolve(import.meta.dirname, "../.semlia/resume-demo/retail-v1");
const state = JSON.parse(readFileSync(`${root}/state.json`, "utf8"));
if (!/^semlia_demo_[0-9a-f]{16}$/.test(state.owner) || !Number.isInteger(state.apiPort)) throw new Error("Owned normal demo runtime required");

export default defineConfig({
  testDir: "./e2e-demo", workers: 1, fullyParallel: false, retries: 0,
  reporter: "list", outputDir: `${root}/browser`, timeout: 90_000,
  expect: { timeout: 15_000 },
  use: { baseURL: `http://127.0.0.1:${state.apiPort}`, colorScheme: "light", trace: "off", screenshot: "only-on-failure", actionTimeout: 15_000 },
  projects: [
    { name: "desktop", use: { viewport: { width: 1440, height: 900 } } },
    { name: "compact-desktop", use: { viewport: { width: 1024, height: 768 } } },
  ],
});
