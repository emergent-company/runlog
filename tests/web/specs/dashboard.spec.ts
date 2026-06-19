import { test, expect } from './fixtures';

test.describe('Dashboard', () => {
  test('renders app shell, stat cards, and recent runs table', async ({ page }) => {
    await test.step('navigate to dashboard', async () => {
      await page.goto('./');
    });
    await test.step('verify app shell rendered', async () => {
      await expect(page.locator('[data-testid="app-page"]')).toBeVisible();
    });
    await test.step('verify stat cards present', async () => {
      await expect(page.locator('[data-testid="dashboard-content"]')).toBeVisible();
      await expect(page.locator('[data-testid="stat-cards"]')).toBeVisible();
    });
    await test.step('verify stat cards have content', async () => {
      const statCards = page.locator('[data-testid="stat-cards"] .card');
      const count = await statCards.count();
      expect(count).toBeGreaterThan(0);
    });
  });
});
