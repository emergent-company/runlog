import { test, expect } from './fixtures';

test.describe('Test launch', () => {
  test('run test button on detail page redirects to run detail', async ({ page }) => {
    await test.step('navigate to TestPass detail page', async () => {
      await page.goto('tests/TestPass');
      await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible();
    });
    await test.step('click Run Test button', async () => {
      const btn = page.locator('[data-testid="run-test-btn"]');
      await expect(btn).toBeVisible();
      await btn.click();
    });
    await test.step('verify redirect to run detail page', async () => {
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 15000 });
      await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('TestPass');
    });
  });
});
