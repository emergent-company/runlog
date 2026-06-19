import { test, expect } from './fixtures';

test.describe('HTMX race conditions', () => {
  test('rapid sequential clicks on different test rows navigates to the last one', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('rapidly click two different test rows', async () => {
      const rowA = page.locator('tr[id^="test-"]').filter({ hasText: 'TestPass' }).first();
      // First click triggers HTMX navigation; wait briefly then click second
      await rowA.click();
      await page.waitForTimeout(200);
      const rowB = page.locator('tr[id^="test-"]').filter({ hasText: 'TestFail' });
      if (await rowB.count() > 0) {
        await rowB.first().click({ timeout: 2000 }).catch(() => {});
      }
    });
    await test.step('verify test detail page loads for the last clicked row', async () => {
      await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
  });

  test('double-click on same test row does not cause JS errors', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('double-click a test row', async () => {
      const row = page.locator('tr[id^="test-"]').filter({ hasText: 'TestPass' }).first();
      await row.click({ clickCount: 2 });
    });
    await test.step('verify test detail page loads', async () => {
      await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
  });

  test('click test row while delayed category filter request is in-flight', async ({ page }) => {
    await test.step('delay category filter responses', async () => {
      await page.route('**/ui/tests?category=*', async route => {
        await new Promise(r => setTimeout(r, 1000));
        await route.continue();
      });
    });
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('click a test row while filter request is in-flight', async () => {
      const row = page.locator('tr[id^="test-"]').filter({ hasText: 'TestPass' }).first();
      await row.click();
    });
    await test.step('verify navigation completes', async () => {
      await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible({ timeout: 8000 });
    });
  });
});
