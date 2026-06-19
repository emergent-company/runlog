import { test, expect } from './fixtures';

test.describe('Run detail', () => {
  test('navigates from test detail to run detail via click and shows test name', async ({ page }) => {
    await test.step('navigate to TestPass detail page', async () => {
      await page.goto('tests/TestPass');
    });
    await test.step('click the first run row', async () => {
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
    });
    await test.step('verify run detail page shows correct test name', async () => {
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
      await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('TestPass');
    });
  });

  test('run detail shows events table for a run with event data', async ({ page }) => {
    await test.step('navigate to TestMetrics detail page', async () => {
      await page.goto('tests/TestMetrics');
    });
    await test.step('click the first run row', async () => {
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
    });
    await test.step('verify events section appears', async () => {
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
      await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('Events');
    });
  });
});
