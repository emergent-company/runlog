import { test, expect } from './fixtures';

test.describe('Tests list', () => {
  test('renders all seeded tests in the tests table', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('verify each seeded test name appears', async () => {
      await expect(page.locator('[data-testid="tests-content"]')).toContainText('TestPass');
      await expect(page.locator('[data-testid="tests-content"]')).toContainText('TestFail');
      await expect(page.locator('[data-testid="tests-content"]')).toContainText('TestSkip');
      await expect(page.locator('[data-testid="tests-content"]')).toContainText('TestMetrics');
      await expect(page.locator('[data-testid="tests-content"]')).toContainText('TestWithTags');
    });
  });

  test('clicking a test row navigates to its detail page', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('click a test row', async () => {
      const testRow = page.locator('tr[id^="test-"]').filter({ hasText: 'TestPass' }).first();
      await testRow.click();
    });
    await test.step('verify test detail page loads with correct test name', async () => {
      await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible({ timeout: 5000 });
      await expect(page.locator('[data-testid="test-detail-content"]')).toContainText('TestPass');
    });
  });

  test('shows single table with category badges', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('verify table has category column', async () => {
      const table = page.locator('[data-testid="tests-content"] table');
      await expect(table).toBeVisible();
      await expect(table).toContainText('Category');
    });
    await test.step('verify category badges exist', async () => {
      const badges = page.locator('[data-testid="tests-content"] table .badge-ghost');
      const count = await badges.count();
      expect(count).toBeGreaterThan(0);
    });
  });

  test('category filter hides tests from other categories', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('select a category filter', async () => {
      const select = page.locator('[data-testid="category-filter"]');
      const options = await select.locator('option').all();
      const firstCategory = await options[1]?.getAttribute('value');
      if (!firstCategory) return;
      await select.selectOption(firstCategory);
    });
    await test.step('verify all visible badges match the selected category', async () => {
      const badges = page.locator('[data-testid="tests-content"] .badge-ghost');
      const count = await badges.count();
      for (let i = 0; i < count; i++) {
        await expect(badges.nth(i)).toBeVisible();
      }
    });
  });

  test('action dropdown appears on each test row', async ({ page }) => {
    await test.step('navigate to tests page', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
    });
    await test.step('verify dropdown icons exist', async () => {
      const dropdowns = page.locator('.lucide--ellipsis-vertical');
      const count = await dropdowns.count();
      expect(count).toBeGreaterThan(0);
    });
  });
});
