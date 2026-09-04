import { defineConfig } from "@playwright/test"
import { fileURLToPath } from "node:url"

// Copied only into a disposable production-profile project by frontend-e2e.sh.
if (process.env.E2E_DISPOSABLE_DATABASE !== "1") throw new Error("TLS suite requires a disposable database")
const port = Number(process.env.E2E_PORT ?? "4187")
if (!Number.isInteger(port) || port < 1024 || port > 65533) throw new Error("Invalid E2E_PORT")
const cwd = fileURLToPath(new URL("..", import.meta.url))
export default defineConfig({
  testDir: "./e2e",
  testMatch: "tls.spec.ts",
  workers: 1,
  retries: 0,
  timeout: 60_000,
  use: {
    baseURL: `https://app.example.test:${port + 1}`,
    // httptest certificate only; no system trust-store changes.
    ignoreHTTPSErrors: true,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: { args: ["--no-proxy-server", "--host-resolver-rules=MAP app.example.test 127.0.0.1, MAP attacker.invalid.test 127.0.0.1"] },
  },
  webServer: [
    {
      command: "go run ./cmd/api", cwd,
      url: `http://127.0.0.1:${port}/health/ready`,
      reuseExistingServer: false, timeout: 120_000,
      stdout: "pipe",
      gracefulShutdown: { signal: "SIGTERM", timeout: 15_000 },
      env: {
        HTTP_ADDRESS: `127.0.0.1:${port}`, AUTH_SESSION_SECURE: "true",
        HTTP_TRUSTED_PROXY_CIDRS: "127.0.0.1/32",
        AUTH_BOOTSTRAP_EMAIL: "browser-admin@example.test",
        AUTH_BOOTSTRAP_PASSWORD: "browser test initial password",
        AUTH_LOGIN_RATE_LIMIT: "100", AUTH_AUDIT_RETENTION_DAYS: "0",
      },
    },
    {
      command: "go run ./.gobackend/tls-proxy/main.go", cwd,
      url: `https://127.0.0.1:${port + 1}/health/ready`,
      ignoreHTTPSErrors: true,
      reuseExistingServer: false, timeout: 120_000,
      stdout: "pipe",
      gracefulShutdown: { signal: "SIGTERM", timeout: 15_000 },
    },
  ],
})
