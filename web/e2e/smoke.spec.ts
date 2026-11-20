import { expect, test } from '@playwright/test'
import { apiURL, requireE2E, uiLogin } from './helpers'

test.describe('Norix smoke', () => {
  test.beforeAll(() => {
    const skip = requireE2E()
    test.skip(!!skip, skip ?? undefined)
  })

  test('healthz is up', async ({ request }) => {
    const response = await request.get(`${apiURL}/healthz`)
    expect(response.ok()).toBeTruthy()
    expect(await response.text()).toContain('ok')
  })

  test('bootstrap, login shell and create project', async ({ page }) => {
    await uiLogin(page)

    await page.getByRole('link', { name: 'Projects', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({ timeout: 15000 })

    const projectName = `demo-${Date.now()}`
    await page.getByRole('button', { name: 'New project' }).click()
    await expect(page.getByRole('dialog', { name: 'New project' })).toBeVisible()
    await page.getByLabel('Name').fill(projectName)
    await page.getByRole('button', { name: 'Create project' }).click()
    await expect(page.getByRole('heading', { name: projectName })).toBeVisible({ timeout: 15000 })
  })
})
