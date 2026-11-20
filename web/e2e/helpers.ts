import { expect, type APIRequestContext, type Page } from '@playwright/test'

export const apiURL = process.env.NORIX_E2E_API_URL ?? 'http://127.0.0.1:8080'
export const email = process.env.NORIX_E2E_EMAIL ?? `owner-${Date.now()}@example.com`
export const password = process.env.NORIX_E2E_PASSWORD ?? 'playwright-owner-password'

type ConnectBody = Record<string, unknown>

export async function connectPost(
  request: APIRequestContext,
  path: string,
  body: ConnectBody = {},
  token?: string,
) {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'Connect-Protocol-Version': '1',
  }
  if (token) headers.Authorization = `Bearer ${token}`
  return request.post(`${apiURL}/api${path}`, {
    headers,
    data: body,
  })
}

export async function ensureSessionToken(request: APIRequestContext): Promise<string> {
  if (process.env.NORIX_E2E_TOKEN) return process.env.NORIX_E2E_TOKEN

  const status = await connectPost(request, '/v1.AuthService/GetBootstrapStatus', {})
  expect(status.ok()).toBeTruthy()
  const statusBody = await status.json() as { configured?: boolean }

  if (!statusBody.configured) {
    const boot = await connectPost(request, '/v1.AuthService/CreateInitialAdmin', {
      organizationName: 'Norix E2E',
      adminName: 'Owner',
      adminEmail: email,
      adminPassword: password,
    })
    expect(boot.ok()).toBeTruthy()
    const bootBody = await boot.json() as { token?: string }
    expect(bootBody.token).toBeTruthy()
    return bootBody.token as string
  }

  const loginEmail = process.env.NORIX_E2E_EXISTING_EMAIL ?? email
  const loginPassword = process.env.NORIX_E2E_EXISTING_PASSWORD ?? password
  const login = await connectPost(request, '/v1.AuthService/Login', {
    email: loginEmail,
    password: loginPassword,
  })
  expect(login.ok()).toBeTruthy()
  const loginBody = await login.json() as { token?: string }
  expect(loginBody.token).toBeTruthy()
  return loginBody.token as string
}

/** Bootstrap or sign-in through the UI; lands on Command center. */
export async function uiLogin(page: Page) {
  await page.goto('/')
  const heading = page.getByRole('heading', { name: /Welcome back|Create your administrator/ })
  await expect(heading).toBeVisible({ timeout: 15000 })
  const title = await heading.textContent()

  if (title?.includes('Create your administrator')) {
    await page.getByLabel('Organization').fill('Norix E2E')
    await page.getByLabel('Administrator name').fill('Owner')
    await page.getByLabel('Email').fill(email)
    await page.getByLabel('Password', { exact: true }).fill(password)
    await page.getByLabel('Confirm password').fill(password)
    await page.getByRole('button', { name: 'Create administrator' }).click()
  } else {
    await page.getByLabel('Email').fill(process.env.NORIX_E2E_EXISTING_EMAIL ?? email)
    await page.getByLabel('Password').fill(process.env.NORIX_E2E_EXISTING_PASSWORD ?? password)
    await page.getByRole('button', { name: 'Sign in' }).click()
  }

  await expect(page.getByRole('heading', { name: 'Command center' })).toBeVisible({ timeout: 15000 })
}

export function requireE2E() {
  if (!process.env.NORIX_E2E) {
    return 'Set NORIX_E2E=1 with a running control plane to run E2E tests'
  }
  return null
}

export function fullStackEnabled() {
  return process.env.NORIX_E2E_FULL === '1'
}
