import { test, expect } from './fixtures';

test.describe('Sidebar icons', () => {
  test('all sidebar nav items have icon spans', async ({ page }) => {
    await test.step('load dashboard page', async () => {
      await page.goto('');
    });
    await test.step('verify every nav link has an iconify span', async () => {
      const navLinks = page.locator('#layout-sidebar a.menu-item');
      const count = await navLinks.count();
      expect(count).toBeGreaterThan(0);
      for (let i = 0; i < count; i++) {
        const link = navLinks.nth(i);
        const icon = link.locator('span.iconify');
        await expect(icon).toBeAttached();
      }
    });
  });

  test('all-runs nav item has lucide--list icon', async ({ page }) => {
    await test.step('load dashboard page', async () => {
      await page.goto('');
    });
    await test.step('verify all-runs icon class', async () => {
      const allRunsLink = page.locator('#layout-sidebar a.menu-item[href="/ui/runs"]');
      await expect(allRunsLink).toBeVisible();
      const icon = allRunsLink.locator('span.iconify.lucide--list');
      await expect(icon).toBeAttached();
    });
  });

  test('dashboard nav item has lucide--layout-dashboard icon', async ({ page }) => {
    await test.step('load dashboard page', async () => {
      await page.goto('');
    });
    await test.step('verify dashboard icon class', async () => {
      const link = page.locator('#layout-sidebar a.menu-item[href="/ui/"]');
      await expect(link.locator('span.iconify.lucide--layout-dashboard')).toBeAttached();
    });
  });

  test('tests nav item has lucide--flask-conical icon', async ({ page }) => {
    await test.step('load dashboard page', async () => {
      await page.goto('');
    });
    await test.step('verify tests icon class', async () => {
      const link = page.locator('#layout-sidebar a.menu-item[href="/ui/tests"]');
      await expect(link.locator('span.iconify.lucide--flask-conical')).toBeAttached();
    });
  });

  test('events nav item has lucide--file-text icon in sidebar', async ({ page }) => {
    await test.step('load dashboard page', async () => {
      await page.goto('');
    });
    await test.step('verify events sidebar link has icon', async () => {
      const sidebar = page.locator('#layout-sidebar');
      await expect(sidebar).toContainText('Events');
      const link = page.locator('#layout-sidebar a.menu-item[href="/ui/events"]');
      await expect(link.locator('span.iconify.lucide--file-text')).toBeAttached();
    });
  });
});
