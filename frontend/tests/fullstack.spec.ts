import { test, expect, type Page } from "@playwright/test";
import type {
  Analysis,
  Definition,
  Module,
  Session,
  Usage,
} from "../src/types";

// Run against a disposable database and an actual API. No HTTP routes are mocked.
const email = `browser-${Date.now()}-${Math.random().toString(16).slice(2)}@example.invalid`;
const password = "Disposable-browser-password-2026!";
const accountName = "Browser reflection user";

test.beforeEach(async ({ page }) => {
  page.on("requestfailed", (request) => {
    console.log(
      `Browser request failed: ${request.method()} ${new URL(request.url()).pathname} ${request.failure()?.errorText}`,
    );
  });
  await page.addInitScript(() => {
    if (!localStorage.getItem("relations-locale"))
      localStorage.setItem("relations-locale", "en");
    if (!localStorage.getItem("relations-theme"))
      localStorage.setItem("relations-theme", "light");
  });
});

async function loginSession(
  page: Page,
  accountEmail: string,
  accountPassword: string,
): Promise<Session> {
  const response = await page.request.post("/api/v1/auth/login", {
    headers: { Origin: new URL(page.url()).origin },
    data: { email: accountEmail, password: accountPassword },
  });
  expect(response.status()).toBe(200);
  return (await response.json()).data as Session;
}

async function submitWizard(page: Page, module: Module, localExample = false) {
  await page.goto(`/reflect/${module}`);
  const response = await page.request.get(
    `/api/v1/questionnaires/${module}?locale=en`,
  );
  expect(response.status()).toBe(200);
  const bank = (await response.json()).data as Definition;
  const respondents = module === "compatibility" ? 2 : 1;
  const questionSteps = Math.ceil(bank.questions.length / 6) * respondents;
  for (let step = 0; step < questionSteps; step++) {
    const cards = page.locator("fieldset.question-card");
    await expect(cards.first()).toBeVisible();
    const count = await cards.count();
    for (let i = 0; i < count; i++) {
      if (module === "self" && step === 0 && i === 0) {
        await cards.nth(i).locator(".unknown-choice").click();
      } else {
        await cards.nth(i).locator(".likert label").nth(2).click();
      }
    }
    const lastWithoutContext =
      step + 1 === questionSteps && bank.context_fields.length === 0;
    await page
      .getByRole("button", {
        name: lastWithoutContext ? "Review & submit" : "Continue",
        exact: true,
      })
      .click();
    await expect(page.locator(".step-label")).toContainText(
      `Page ${step + 2} /`,
    );
  }
  if (bank.context_fields.length) {
    await expect(
      page.getByRole("heading", { name: "A little context", exact: true }),
    ).toBeVisible();
    for (const field of bank.context_fields) {
      const control = page.getByLabel(field.text.en, { exact: false });
      if (field.type === "boolean") {
        await control.check();
      } else if (field.type === "enum") {
        const choice =
          field.id === "partner_response_origin"
            ? "independent"
            : field.id === "relationship_type"
              ? "partner"
              : field.id === "safety"
                ? "yes"
                : field.options![0].value;
        await control.selectOption(choice);
      } else {
        await control.fill(
          field.id === "topic"
            ? "The disagreement concerns how we share household responsibilities."
            : "An example supplied voluntarily for this disposable browser test.",
        );
      }
    }
    await page
      .getByRole("button", { name: "Review & submit", exact: true })
      .click();
  }
  await expect(
    page.getByRole("heading", { name: "Review & submit", exact: true }),
  ).toBeVisible();
  const submit = page.getByRole("button", {
    name: "See my results",
    exact: true,
  });
  await expect(submit).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: "I understand the educational limitations and consent to saving and processing these responses.",
      exact: true,
    })
    .check();
  if (localExample) {
    await page
      .getByRole("checkbox", {
        name: "Generate a local example report without sending my responses externally.",
        exact: true,
      })
      .check();
  }
  const submitted = page.waitForResponse(
    (r) =>
      r.request().method() === "POST" &&
      /\/analyses\/[^/]+\/submit$/.test(new URL(r.url()).pathname),
  );
  await submit.click();
  const saved = await submitted;
  expect(saved.status()).toBe(200);
  const analysis = (await saved.json()).data as Analysis & {
    ai_consent_at: string | null;
  };
  expect(analysis.module).toBe(module);
  expect(analysis.status).toBe("completed");
  expect(analysis.report?.kind).toBe(
    localExample ? "example" : "deterministic",
  );
  expect(analysis.report?.limitations.length).toBeGreaterThan(0);
  await expect(page).toHaveURL(new RegExp(`/analyses/${analysis.id}$`));
  await expect(
    page.getByRole("heading", { name: "Your responses", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("table")).toHaveCount(respondents);
  if (module === "self")
    expect(analysis.scores.respondents.self.unknown).toBe(1);
  if (module === "compatibility") {
    expect(analysis.scores.compatibility?.index).toBe(100);
    expect(analysis.scores.compatibility?.included_dimensions.length).toBe(15);
    await expect(
      page.getByRole("heading", { name: "Answer similarity", exact: true }),
    ).toBeVisible();
  }
  for (const respondent of Object.values(analysis.scores.respondents)) {
    expect(respondent.dimensions.every((d) => d.score === 50)).toBe(true);
  }
  return analysis;
}

test("an account completes all reflection modules, resumes a draft and retains preferences and authentication", async ({
  page,
}) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.goto("/register");
  await page.getByLabel("Your name", { exact: true }).fill(accountName);
  await page.getByLabel("Email address", { exact: true }).fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "A little clarity, one reflection at a time.",
      exact: true,
    }),
  ).toBeVisible();
  await page
    .getByRole("link", {
      name: "Complete your profile to begin a reflection.",
      exact: true,
    })
    .click();
  await page.getByLabel("Age", { exact: true }).fill("30");
  await page
    .getByRole("combobox", { name: "Gender", exact: true })
    .selectOption("prefer_not_to_say");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Saved" }),
  ).toBeVisible();

  const results: Analysis[] = [];
  for (const module of ["self", "compatibility", "conflict"] as Module[]) {
    results.push(await submitWizard(page, module));
  }
  await page.goto("/history");
  await expect(page.locator(".analysis-row")).toHaveCount(3);
  await page
    .getByRole("combobox", { name: "Filter", exact: true })
    .selectOption("compatibility");
  await expect(page.locator(".analysis-row")).toHaveCount(1);
  await expect(page.locator(".analysis-row")).toContainText(
    "Explore your connection",
  );

  await page.goto("/reflect/self");
  await page
    .locator("fieldset.question-card")
    .first()
    .locator(".likert label")
    .nth(2)
    .click();
  const draftCreated = page.waitForResponse(
    (r) =>
      r.request().method() === "POST" &&
      new URL(r.url()).pathname === "/api/v1/analyses",
  );
  await page.getByRole("button", { name: "Save & exit", exact: true }).click();
  const draft = (await (await draftCreated).json()).data as Analysis;
  await expect(page).toHaveURL(/\/history$/);
  await page
    .locator(`.analysis-link[href="/analyses/${draft.id}/edit"]`)
    .click();
  await expect(
    page.locator("fieldset.question-card").first().locator('input[value="3"]'),
  ).toBeChecked();
  await page.getByRole("button", { name: "Save & exit", exact: true }).click();
  const draftRow = page
    .locator(".analysis-row")
    .filter({ has: page.locator(`a[href="/analyses/${draft.id}/edit"]`) });
  page.once("dialog", (dialog) => dialog.accept());
  await draftRow
    .getByRole("button", { name: "Delete: Understand yourself", exact: true })
    .click();
  await expect(
    page.locator(`a[href="/analyses/${draft.id}/edit"]`),
  ).toHaveCount(0);

  await page
    .getByRole("button", { name: "Use dark theme", exact: true })
    .click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page
    .getByRole("button", { name: "Switch to Persian", exact: true })
    .click();
  await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "fa");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page
    .getByRole("button", { name: "Switch to English", exact: true })
    .click();
  await expect(page.locator("html")).toHaveAttribute("dir", "ltr");
  await expect(page.locator(".account-chip")).toContainText(accountName);
  expect(
    await page.evaluate(() =>
      Object.keys(localStorage).some((k) => /token|password|secret/i.test(k)),
    ),
  ).toBe(false);

  await page.goto("/admin");
  await expect(page).toHaveURL(/\/$/);
  await expect(
    page.getByRole("link", { name: "Administration", exact: true }),
  ).toHaveCount(0);
  const session = await loginSession(page, email, password);
  const forbidden = await page.request.get("/api/v1/admin/analytics", {
    headers: { Authorization: `Bearer ${session.access_token}` },
  });
  expect(forbidden.status()).toBe(403);

  await page.goto(`/analyses/${results[0].id}`);
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page).toHaveURL(/\/history$/);
  await expect(
    page.locator(`.analysis-link[href="/analyses/${results[0].id}"]`),
  ).toHaveCount(0);

  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await expect(
    page.getByRole("heading", {
      name: "A little clarity, one reflection at a time.",
      exact: true,
    }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1,
    ),
  ).toBe(true);
  await page.screenshot({
    path: "test-results/mobile-dashboard.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
  expect(pageErrors).toEqual([]);
});

test("administrator manages actual users, provider settings and audited limits", async ({
  page,
  request,
}) => {
  test.skip(
    !process.env.E2E_ADMIN_EMAIL || !process.env.E2E_ADMIN_PASSWORD,
    "Seed a disposable administrator and set E2E_ADMIN_EMAIL/E2E_ADMIN_PASSWORD.",
  );
  const managedName = `Managed browser user ${Date.now()}`;
  const managedEmail = `managed-${Date.now()}@example.invalid`;
  const fixture = await request.post("/api/v1/auth/register", {
    headers: {
      Origin: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:5173",
    },
    data: { name: managedName, email: managedEmail, password },
  });
  expect(fixture.status()).toBe(201);
  await page.goto("/login");
  await page
    .getByLabel("Email address", { exact: true })
    .fill(process.env.E2E_ADMIN_EMAIL!);
  await page
    .getByLabel("Password", { exact: true })
    .fill(process.env.E2E_ADMIN_PASSWORD!);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByRole("link", { name: "Administration", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Administration", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".stats-grid .stat-card")).toHaveCount(4);
  await page.getByRole("tab", { name: "Users", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Search by name or email", exact: true })
    .fill(managedName);
  const searchResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" &&
      new URL(response.url()).pathname === "/api/v1/admin/users" &&
      new URL(response.url()).searchParams.get("q") === managedName,
  );
  await page.getByRole("button", { name: "Filter", exact: true }).click();
  expect((await searchResponse).status()).toBe(200);
  await expect(page.locator(".loading")).toHaveCount(0);
  const row = page.getByRole("row").filter({ hasText: managedName });
  await expect(row).toContainText(managedEmail);
  await expect(row.getByRole("spinbutton")).toBeEnabled();
  await row.getByRole("spinbutton").fill("7");
  const limitResponse = page.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" &&
      /\/api\/v1\/admin\/users\/[^/]+$/.test(new URL(response.url()).pathname),
  );
  await row.getByRole("button", { name: "Save changes", exact: true }).click();
  const limitSaved = await limitResponse;
  expect(limitSaved.request().postDataJSON()).toEqual({ daily_limit: 7 });
  expect(limitSaved.status()).toBe(200);
  expect((await limitSaved.json()).data.daily_limit).toBe(7);
  await expect(row.getByRole("spinbutton")).toHaveValue("7");
  await row.getByRole("button", { name: "Disable", exact: true }).click();
  await expect(
    row.getByRole("button", { name: "Enable", exact: true }),
  ).toBeVisible();
  await row.getByRole("button", { name: "Enable", exact: true }).click();
  await expect(
    row.getByRole("button", { name: "Disable", exact: true }),
  ).toBeVisible();

  await page.getByRole("tab", { name: "AI provider", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "AI provider", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Model ID", { exact: true }).fill("gpt-4.1-mini");
  await page
    .getByRole("checkbox", { name: "Enable AI generation", exact: true })
    .uncheck();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Saved" }),
  ).toBeVisible();
  await expect(page.getByLabel("New API key", { exact: true })).toHaveValue("");
  await expect(
    page.getByRole("button", { name: "Test connection", exact: true }),
  ).toBeDisabled();

  await page.getByRole("tab", { name: "Daily limits", exact: true }).click();
  await page.getByLabel("Global daily limit", { exact: true }).fill("950");
  await page.getByLabel("Default user daily limit", { exact: true }).fill("8");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Saved" }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Audit log", exact: true }).click();
  await expect(
    page.getByRole("cell", { name: "limits.updated", exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("cell", { name: "user.updated", exact: true }).first(),
  ).toBeVisible();
});

test("startup demonstration accounts have eligible profiles and their intended roles", async ({
  page,
}) => {
  test.skip(
    process.env.E2E_DEFAULT_ACCOUNTS !== "true",
    "Enable development/test startup defaults in a fresh disposable database.",
  );
  for (const account of [
    {
      email: "admin@relations.local",
      password: "RelationsAdmin2026!",
      role: "admin",
    },
    {
      email: "user@relations.local",
      password: "RelationsUser2026!",
      role: "user",
    },
  ]) {
    await page.goto("/login");
    await page.getByLabel("Email address", { exact: true }).fill(account.email);
    await page.getByLabel("Password", { exact: true }).fill(account.password);
    const loggedIn = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname === "/api/v1/auth/login",
    );
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    const response = await loggedIn;
    expect(response.status()).toBe(200);
    const user = (await response.json()).data.user;
    expect(user.role).toBe(account.role);
    expect(user.age).toBe(Math.max(30, Number(process.env.MIN_AGE || 18)));
    expect(user.gender).toBe("prefer_not_to_say");
    await expect(
      page.getByRole("heading", {
        name: "A little clarity, one reflection at a time.",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", {
        name: "Complete your profile to begin a reflection.",
        exact: true,
      }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("link", { name: "Administration", exact: true }),
    ).toHaveCount(account.role === "admin" ? 1 : 0);
    const refresh = (await page.context().cookies()).find(
      (cookie) =>
        cookie.name === "rm_refresh" || cookie.name === "__Secure-rm_refresh",
    );
    expect(refresh?.httpOnly).toBe(true);
    expect(refresh?.sameSite).toBe("Lax");
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page).toHaveURL(/\/login$/);
  }
});

test("local preview and selected example reports use actual scores without external sharing or quota", async ({
  page,
}) => {
  test.skip(
    process.env.E2E_DEFAULT_ACCOUNTS !== "true",
    "Enable startup defaults in a fresh disposable database.",
  );
  await page.goto("/login");
  await page
    .getByLabel("Email address", { exact: true })
    .fill("admin@relations.local");
  await page
    .getByLabel("Password", { exact: true })
    .fill("RelationsAdmin2026!");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByRole("link", { name: "Administration", exact: true }).click();
  await page.getByRole("tab", { name: "AI provider", exact: true }).click();
  await page
    .getByRole("combobox", { name: "AI provider", exact: true })
    .selectOption("openai");
  await page.getByLabel("Model ID", { exact: true }).fill("gpt-4.1-mini");
  await page
    .getByRole("checkbox", { name: "Enable AI generation", exact: true })
    .uncheck();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "Saved" }),
  ).toBeVisible();
  await expect(page.getByLabel("New API key", { exact: true })).toHaveValue("");
  await expect(
    page.getByRole("button", { name: "Test connection", exact: true }),
  ).toBeDisabled();

  for (const module of ["self", "compatibility", "conflict"] as Module[]) {
    await page
      .getByRole("combobox", { name: "Questions", exact: true })
      .selectOption(module);
    const previewResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname === "/api/v1/admin/provider/preview",
    );
    await page
      .getByRole("button", { name: "Preview example response", exact: true })
      .click();
    const response = await previewResponse;
    expect(response.status()).toBe(200);
    const preview = (await response.json()).data;
    expect(preview.external_request).toBe(false);
    expect(preview.is_example).toBe(true);
    expect(preview.provider).toBe("mock");
    expect(preview.report.kind).toBe("example");
    expect(preview.report.module).toBe(module);
    expect(preview.used_sample_answers).toBe(true);
    for (const respondent of Object.values(preview.scores.respondents) as {
      dimensions: { score: number | null }[];
    }[]) {
      expect(
        respondent.dimensions.every((dimension) => dimension.score === 50),
      ).toBe(true);
    }
    await expect(page.locator(".preview-result")).toContainText(
      "Local example report",
    );
  }
  await page
    .getByRole("combobox", { name: "AI provider", exact: true })
    .selectOption("mock");
  await expect(page.getByLabel("New API key", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("Model ID", { exact: true })).toHaveValue(
    "local-example-v1",
  );
  await page
    .getByRole("checkbox", { name: "Enable AI generation", exact: true })
    .check();
  const providerSaved = page.waitForResponse(
    (response) =>
      response.request().method() === "PUT" &&
      new URL(response.url()).pathname === "/api/v1/admin/provider",
  );
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  const providerResponse = await providerSaved;
  expect(providerResponse.status()).toBe(200);
  const provider = (await providerResponse.json()).data;
  expect(provider.provider).toBe("mock");
  expect(provider.active).toBe(true);
  expect(provider.has_key).toBe(false);
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await page
    .getByLabel("Email address", { exact: true })
    .fill("user@relations.local");
  await page.getByLabel("Password", { exact: true }).fill("RelationsUser2026!");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: "A little clarity, one reflection at a time.",
      exact: true,
    }),
  ).toBeVisible();
  const session = await loginSession(
    page,
    "user@relations.local",
    "RelationsUser2026!",
  );
  const authorization = { Authorization: `Bearer ${session.access_token}` };
  const before = (
    await (
      await page.request.get("/api/v1/analyses/usage", {
        headers: authorization,
      })
    ).json()
  ).data as Usage;
  expect(before.is_example).toBe(true);
  const analysis = await submitWizard(page, "self", true);
  expect(analysis.provider).toBe("mock");
  expect(analysis.model).toBe("local-example-v1");
  expect(analysis.ai_consent_at).toBeNull();
  await expect(
    page.getByRole("heading", { name: "Local example report", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".result-summary")).toContainText(
    "No prompt or personal data was sent to an external provider",
  );
  const after = (
    await (
      await page.request.get("/api/v1/analyses/usage", {
        headers: authorization,
      })
    ).json()
  ).data as Usage;
  expect(after.user_used).toBe(before.user_used);
  expect(after.global_used).toBe(before.global_used);
});
