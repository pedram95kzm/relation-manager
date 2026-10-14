import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 120_000,
  expect: { timeout: 10_000 },
  reporter: "list",
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:5173",
    viewport: { width: 1440, height: 1000 },
    actionTimeout: 15_000,
    screenshot: "only-on-failure",
    trace: "off",
    launchOptions: {
      ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE
        ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE }
        : {}),
      args: [
        "--no-proxy-server",
        ...(process.env.PLAYWRIGHT_LOCALHOST_IPV6 === "true"
          ? ["--host-resolver-rules=MAP localhost [::1]"]
          : []),
      ],
    },
  },
});
