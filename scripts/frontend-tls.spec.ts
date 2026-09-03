import { test, expect } from "@playwright/test"

// All requests below originate in Chromium, including cross-site Fetch metadata.
test("production Secure cookies, reverse proxy and browser cross-site boundaries", async ({ page, context, browser }) => {
  const port = Number(process.env.E2E_PORT ?? "4187")
  const origin = `https://app.example.test:${port + 1}`
  const plain = `http://app.example.test:${port + 2}`
  const credentials = { email: "browser-admin@example.test", password: "browser test initial password" }

  // Use a .test hostname: browsers can treat localhost HTTP as a secure exception.
  await page.goto(`${plain}/admin/login`)
  const plainLogin = await page.evaluate(async (data) => (await fetch("/auth/login", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data),
  })).status, credentials)
  expect(plainLogin).toBe(200)
  expect((await context.cookies()).some(cookie => cookie.name === "__Host-session")).toBe(false)
  expect(await page.evaluate(async () => (await fetch("/auth/me")).status)).toBe(401)

  const home = await page.goto(`${origin}/admin/login`)
  expect(home?.headers()["strict-transport-security"]).toBe("max-age=31536000")
  await page.getByLabel("Email", { exact: true }).fill(credentials.email)
  await page.getByLabel("Password", { exact: true }).fill(credentials.password)
  const loginResponse = page.waitForResponse(response => response.url() === `${origin}/auth/login`)
  await page.getByRole("button", { name: "Sign in", exact: true }).click()
  const login = await loginResponse
  expect(login.status()).toBe(200)
  expect((await login.request().allHeaders())["sec-fetch-site"]).toBe("same-origin")
  const setCookie = await login.headerValue("set-cookie")
  expect(setCookie).toContain("__Host-session=")
  expect(setCookie).not.toMatch(/;\s*Domain=/i)
  await expect(page.getByRole("button", { name: "Sign out", exact: true })).toBeVisible()
  const cookie = (await context.cookies()).find(item => item.name === "__Host-session")
  expect(cookie).toMatchObject({ domain: "app.example.test", path: "/", secure: true, httpOnly: true, sameSite: "Lax" })
  expect(await page.evaluate(() => document.cookie.includes("__Host-session"))).toBe(false)

  expect(await page.evaluate(async () => (await fetch("/api/v1/products", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: "TLS browser product", status: "enabled", owner_id: 1, price: "1.23", order: "tls" }),
  })).status)).toBe(201)

  const attacker = await context.newPage()
  try {
    for (const path of ["/auth/login", "/auth/logout", "/api/v1/products"]) {
      await attacker.goto(`https://attacker.invalid.test:${port + 1}/`)
      const target = origin + path
      const blockedResponse = attacker.waitForResponse(response => response.url() === target && response.request().method() === "POST")
      // Native form navigation exposes the full response (unlike opaque no-cors
      // Fetch events). Global CSRF must reject before any JSON body decoding.
      await attacker.evaluate(target => {
        const form = document.createElement("form")
        form.method = "POST"; form.action = target
        document.body.append(form); form.submit()
      }, target)
      const blocked = await blockedResponse
      expect(blocked.status()).toBe(403)
      // Inspect the browser-generated metadata as received by the proxy.
      expect(await blocked.headerValue("x-e2e-fetch-site")).toBe("cross-site")
      expect(await blocked.headerValue("x-e2e-cookie-present")).toBe("false")
      await attacker.waitForURL(target, { waitUntil: "load" })
    }
    expect((await context.cookies()).find(item => item.name === "__Host-session")?.value === cookie?.value).toBe(true)

    // Lax does permit cookies on a cross-site, top-level safe navigation.
    await attacker.goto(`https://attacker.invalid.test:${port + 1}/`)
    await attacker.evaluate(url => {
      const link = document.createElement("a")
      link.href = url; link.textContent = "Open account"; document.body.append(link)
    }, `${origin}/auth/me`)
    const accountResponse = attacker.waitForResponse(`${origin}/auth/me`)
    await attacker.getByRole("link", { name: "Open account" }).click()
    const account = await accountResponse
    expect(account.status()).toBe(200)
    expect((await account.json()).data.email).toBe(credentials.email)
    expect(await account.headerValue("x-e2e-fetch-site")).toBe("cross-site")
    expect(await account.headerValue("x-e2e-cookie-present")).toBe("true")
  } finally { await attacker.close() }

  // A second same-origin login rotates the cookie; same-origin logout clears it.
  expect(await page.evaluate(async data => (await fetch("/auth/login", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data),
  })).status, credentials)).toBe(200)
  const rotated = (await context.cookies()).find(item => item.name === "__Host-session")
  expect(Boolean(rotated && rotated.value !== cookie?.value)).toBe(true)
  expect(await page.evaluate(async () => (await fetch("/auth/logout", { method: "POST" })).status)).toBe(204)
  expect((await context.cookies()).some(item => item.name === "__Host-session")).toBe(false)
  expect(await page.evaluate(async () => (await fetch("/auth/me")).status)).toBe(401)

  // Replaying real, previously accepted cookies must fail at the database, not
  // merely because the browser forgot them after rotation/logout.
  const replay = await browser.newContext({ ignoreHTTPSErrors: true })
  try {
    const stale = await replay.newPage()
    for (const prior of [cookie!, rotated!]) {
      await replay.addCookies([prior])
      const response = await stale.goto(`${origin}/auth/me`)
      expect(await response?.headerValue("x-e2e-cookie-present")).toBe("true")
      expect(response?.status()).toBe(401)
    }
  } finally { await replay.close() }
})
