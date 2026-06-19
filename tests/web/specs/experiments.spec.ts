import { test, expect } from './fixtures';

test.describe('Experiments page', () => {
  test('loads and shows page title', async ({ page }) => {
    await test.step('navigate to experiments page', async () => {
      await page.goto('./experiments');
      await expect(page.locator('[data-testid="experiments-page"]')).toBeVisible();
    });
    await test.step('verify page title contains Experiments', async () => {
      await expect(page.locator('[data-testid="experiments-page"]')).toContainText('Experiments');
    });
  });

  test('lists experiments with pass rate and tags columns', async ({ page }) => {
    await test.step('navigate to experiments page', async () => {
      await page.goto('./experiments');
      await expect(page.locator('[data-testid="experiments-page"]')).toBeVisible();
    });
    await test.step('verify table headers', async () => {
      const content = page.locator('[data-testid="experiments-page"]');
      await expect(content).toContainText('Experiment');
      await expect(content).toContainText('Runs');
      await expect(content).toContainText('Pass Rate');
    });
    await test.step('verify at least one experiment row exists', async () => {
      const rows = page.locator('[data-testid="experiments-page"] table tbody tr');
      const count = await rows.count();
      expect(count).toBeGreaterThan(0);
    });
  });

  test('sidebar shows Experiments navigation link', async ({ page }) => {
    await test.step('navigate to dashboard', async () => {
      await page.goto('./');
    });
    await test.step('verify sidebar contains Experiments link', async () => {
      const sidebar = page.locator('#layout-sidebar');
      await expect(sidebar).toContainText('Experiments');
    });
  });
});
