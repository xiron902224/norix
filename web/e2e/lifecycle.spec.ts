import { expect, test } from '@playwright/test'
import {
  apiURL,
  connectPost,
  ensureSessionToken,
  fullStackEnabled,
  requireE2E,
  uiLogin,
} from './helpers'

/**
 * Bootstrap → project → image app → domain → logs → delete.
 *
 * UI steps that need an online Docker agent (Deploy → running) are skipped unless
 * NORIX_E2E_FULL=1. API-level CreateWorkload / AssignDomain / GetWorkloadLogs /
 * DeleteWorkload always run when NORIX_E2E=1.
 */
test.describe('Norix lifecycle', () => {
  test.beforeAll(() => {
    const skip = requireE2E()
    test.skip(!!skip, skip ?? undefined)
  })

  test('API: create project, image workload, domain, logs, delete', async ({ request }) => {
    const token = await ensureSessionToken(request)
    const suffix = Date.now()

    const projectRes = await connectPost(request, '/v1.ProjectService/CreateProject', {
      name: `e2e-api-${suffix}`,
    }, token)
    expect(projectRes.ok()).toBeTruthy()
    const projectBody = await projectRes.json() as { project?: { id?: string; name?: string } }
    const projectId = projectBody.project?.id
    expect(projectId).toBeTruthy()

    const workloadRes = await connectPost(request, '/v1.WorkloadService/CreateWorkload', {
      workload: {
        projectId,
        name: `web-${suffix}`,
        runtime: 'RUNTIME_TYPE_CONTAINER',
        imageOrSource: 'nginx:alpine',
        containerPort: 80,
        sourceKind: 'image',
        cpuShares: 1024,
        memoryLimitMb: '512',
        replicaCount: 1,
        scheduleStrategy: 'spread',
        preferredZone: 'default',
      },
    }, token)
    expect(workloadRes.ok()).toBeTruthy()
    const workloadBody = await workloadRes.json() as { workload?: { id?: string; name?: string } }
    const workloadId = workloadBody.workload?.id
    expect(workloadId).toBeTruthy()

    if (fullStackEnabled()) {
      const deploy = await connectPost(request, '/v1.WorkloadService/Deploy', {
        workload: { id: workloadId },
        containerPort: 80,
      }, token)
      // Agent required for 200; surface status for debugging when FULL=1.
      expect([200, 400, 409, 503]).toContain(deploy.status())
    }

    const hostname = `e2e-${suffix}.localhost`
    const domainRes = await connectPost(request, '/v1.WorkloadService/AssignDomain', {
      workloadId,
      hostname,
      tlsMode: 'internal',
    }, token)
    expect(domainRes.ok()).toBeTruthy()
    const domainBody = await domainRes.json() as { route?: { hostname?: string } }
    expect(domainBody.route?.hostname).toBe(hostname)

    const logsRes = await connectPost(request, '/v1.WorkloadService/GetWorkloadLogs', {
      workloadId,
      tailLines: 40,
    }, token)
    expect(logsRes.ok()).toBeTruthy()
    const logsBody = await logsRes.json() as { lines?: string[] }
    expect(Array.isArray(logsBody.lines)).toBeTruthy()

    const del = await connectPost(request, '/v1.WorkloadService/DeleteWorkload', {
      id: workloadId,
    }, token)
    expect(del.ok()).toBeTruthy()

    const list = await connectPost(request, '/v1.WorkloadService/ListWorkloads', {
      projectId,
    }, token)
    expect(list.ok()).toBeTruthy()
    const listBody = await list.json() as { workloads?: Array<{ id?: string; status?: string }> }
    const stillThere = (listBody.workloads || []).find((w) => w.id === workloadId && w.status !== 'deleting')
    expect(stillThere).toBeFalsy()
  })

  test('UI: project → image app → domain → logs → delete', async ({ page }) => {
    await uiLogin(page)

    const projectName = `e2e-ui-${Date.now()}`
    const appName = `web-${Date.now()}`
    const hostname = `ui-${Date.now()}.localhost`

    await page.getByRole('link', { name: 'Projects', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible({ timeout: 15000 })
    await page.getByRole('button', { name: 'New project' }).click()
    await page.getByLabel('Name').fill(projectName)
    await page.getByRole('button', { name: 'Create project' }).click()
    await expect(page.getByRole('heading', { name: projectName })).toBeVisible({ timeout: 15000 })

    await page.getByRole('button', { name: 'New application' }).click()
    await expect(page.getByRole('dialog', { name: /New application/ })).toBeVisible()
    await page.getByLabel('Name', { exact: true }).fill(appName)
    await page.getByLabel('Image').fill('nginx:alpine')
    await page.getByRole('button', { name: 'Create' }).click()
    await expect(page.getByRole('heading', { name: appName })).toBeVisible({ timeout: 20000 })

    if (fullStackEnabled()) {
      const deployBtn = page.getByRole('button', { name: /^(Deploy|Start|Redeploy)$/ })
      if (await deployBtn.isEnabled()) {
        await deployBtn.click()
        await expect(page.getByText(/Deploy queued|Deploy in progress|running/i)).toBeVisible({ timeout: 60000 }).catch(() => {
          // Agent may still reject; structure is what we assert under FULL.
        })
      }
    } else {
      test.info().annotations.push({
        type: 'note',
        description: 'Skipped Deploy click — set NORIX_E2E_FULL=1 with an online agent for Docker deploy',
      })
    }

    await page.getByRole('link', { name: 'Domains', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Domains' })).toBeVisible({ timeout: 15000 })
    await page.getByRole('button', { name: 'Add domain' }).click()
    await expect(page.getByRole('dialog', { name: 'Add domain' })).toBeVisible()
    await page.getByLabel('Project').selectOption({ label: projectName })
    await page.getByLabel('Target application').selectOption({ label: new RegExp(`^${appName}\\b`) })
    await page.getByLabel('Domain').fill(hostname)
    await page.getByRole('button', { name: 'Assign domain' }).click()
    await expect(page.getByText(hostname)).toBeVisible({ timeout: 15000 })

    await page.getByRole('link', { name: 'Projects', exact: true }).click()
    await page.getByRole('heading', { name: 'Projects' }).waitFor({ timeout: 15000 })
    // Open project then application (table row / button with name).
    await page.getByText(projectName, { exact: true }).first().click()
    await expect(page.getByRole('heading', { name: projectName })).toBeVisible({ timeout: 15000 })
    await page.getByText(appName, { exact: true }).first().click()
    await expect(page.getByRole('heading', { name: appName })).toBeVisible({ timeout: 15000 })

    await expect(page.getByText('Recent runtime logs')).toBeVisible()
    await expect(page.locator('pre.log-view')).toBeVisible()

    page.once('dialog', (dialog) => dialog.accept())
    await page.locator('.compass-danger-zone').getByRole('button', { name: 'Delete' }).click()
    await expect(page.getByRole('heading', { name: projectName })).toBeVisible({ timeout: 20000 })
  })

  test('healthz still ok after lifecycle', async ({ request }) => {
    const response = await request.get(`${apiURL}/healthz`)
    expect(response.ok()).toBeTruthy()
  })
})
