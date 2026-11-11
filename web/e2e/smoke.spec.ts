import { expect, test } from '@playwright/test'

const apiURL = process.env.NORIX_E2E_API_URL ?? 'http://127.0.0.1:8080'
const email = process.env.NORIX_E2E_EMAIL ?? `owner-${Date.now()}@example.com`
const password = process.env.NORIX_E2E_PASSWORD ?? 'playwright-owner-password'

test.describe('Norix smoke', () => {
  test.beforeAll(() => {
    test.skip(!process.env.NORIX_E2E, 'Set NORIX_E2E=1 with a running control plane to run smoke tests')
  })

  test('healthz is up', async ({ request }) => {
    const response = await request.get(`${apiURL}/healthz`)
    expect(response.ok()).toBeTruthy()
    expect(await response.text()).toContain('ok')
  })

  test('bootstrap, login shell and create project', async ({ page }) => {
    await page.goto('/')
    const configured = await page.getByRole('heading', { name: /Welcome back|Create your administrator/ }).textContent()

    if (configured?.includes('Create your administrator')) {
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
    await page.getByRole('button', { name: 'Projects', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({ timeout: 15000 })
    const projectName = `demo-${Date.now()}`
    await page.getByLabel('New project').fill(projectName)
    await page.getByRole('button', { name: 'Create project' }).click()
    await expect(page.getByRole('button', { name: projectName })).toBeVisible()
  })
})
