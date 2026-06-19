import { test, expect } from './fixtures';

test.describe('Test detail', () => {
  test('displays run history table for a test with mixed outcomes', async ({ page }) => {
    await test.step('navigate to TestFail detail page', async () => {
      await page.goto('tests/TestFail');
      await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible();
    });
    await test.step('verify run rows exist in history table', async () => {
      const rows = page.locator('tr[id^="run-"]');
      await expect(rows).not.toHaveCount(0);
    });
  });

  test('shows test name in the detail header', async ({ page }) => {
    await test.step('navigate to TestFail detail page', async () => {
      await page.goto('tests/TestFail');
    });
    await test.step('verify header contains test name', async () => {
      await expect(page.locator('[data-testid="test-name"]')).toContainText('TestFail');
    });
  });

  test('test detail page loads with correct test name for TestSkip', async ({ page }) => {
    await test.step('navigate to TestSkip detail page', async () => {
      await page.goto('tests/TestSkip');
    });
    await test.step('verify content area shows correct test name', async () => {
      await expect(page.locator('[data-testid="test-detail-content"]')).toContainText('TestSkip');
    });
  });

  test('test detail page loads with correct test name for TestMetrics', async ({ page }) => {
    await test.step('navigate to TestMetrics detail page', async () => {
      await page.goto('tests/TestMetrics');
    });
    await test.step('verify content area shows correct test name', async () => {
      await expect(page.locator('[data-testid="test-detail-content"]')).toContainText('TestMetrics');
    });
  });
});
