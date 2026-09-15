import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": process.env.SEMLIA_VITE_API_TARGET ?? "http://127.0.0.1:18080",
      "/health": process.env.SEMLIA_VITE_API_TARGET ?? "http://127.0.0.1:18080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    globals: true,
    css: true,
    include: ["src/**/*.test.tsx"],
    testTimeout: 15_000,
  },
});
