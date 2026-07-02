import { test, expect } from './fixtures';

test.describe('Sidebar icons', () => {
  test('every nav item', async ({ page }) => {
    test.info().annotations.push({ type: 'description', description: 'each sidebar nav link has an iconify span' });
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

  test('all-runs → list', async ({ page }) => {
    test.info().annotations.push({ type: 'description', description: 'all-runs nav item icon is lucide--list' });
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

  test('dashboard → layout-dashboard', async ({ page }) => {
    test.info().annotations.push({ type: 'description', description: 'dashboard nav item icon is lucide--layout-dashboard' });
    await test.step('load dashboard page', async () => {
      await page.goto('');
    });
    await test.step('verify dashboard icon class', async () => {
      const link = page.locator('#layout-sidebar a.menu-item[href="/ui/"]');
      await expect(link.locator('span.iconify.lucide--layout-dashboard')).toBeAttached();
    });
  });

  test('tests → flask-conical', async ({ page }) => {
    test.info().annotations.push({ type: 'description', description: 'tests nav item icon is lucide--flask-conical' });
    await test.step('load dashboard page', async () => {
      await page.goto('');
    });
    await test.step('verify tests icon class', async () => {
      const link = page.locator('#layout-sidebar a.menu-item[href="/ui/tests"]');
      await expect(link.locator('span.iconify.lucide--flask-conical')).toBeAttached();
    });
  });

  test('events → file-text', async ({ page }) => {
    test.info().annotations.push({ type: 'description', description: 'events nav item icon is lucide--file-text' });
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
