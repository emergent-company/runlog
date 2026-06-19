import { test, expect } from './fixtures';

test.describe('Run Test launch flow', () => {
  test('clicking Run Test from tests table redirects to run detail', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });

    await test.step('click Run Test for TestFail', async () => {
      const testRow = page.locator('tr[id^="test-"]').filter({ hasText: 'TestFail' }).first();
      const runTestBtn = testRow.locator('[data-run-test]');
      await expect(runTestBtn).toBeAttached();
      await runTestBtn.dispatchEvent('click');
    });

    await test.step('verify redirect to run detail page with TestFail name', async () => {
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 15000 });
      await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('TestFail');
    });
  });

  test('clicking Run Test on test detail page redirects to run detail page', async ({ page }) => {
    await test.step('navigate to TestPass detail page', async () => {
      await page.goto('tests/TestPass');
      await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible();
    });

    await test.step('click Run Test button on detail page', async () => {
      const runTestBtn = page.locator('[data-testid="run-test-btn"]');
      await expect(runTestBtn).toBeVisible();
      await runTestBtn.dispatchEvent('click');
    });

    await test.step('verify redirect to run detail page with TestPass name', async () => {
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 15000 });
      await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('TestPass');
    });
  });
});
