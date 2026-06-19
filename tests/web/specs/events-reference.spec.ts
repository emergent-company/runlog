import { test, expect } from './fixtures';

test.describe('Events Reference', () => {
  test('loads and shows page title', async ({ page }) => {
    await test.step('navigate to events reference page', async () => {
      await page.goto('./events');
      await expect(page.locator('[data-testid="events-reference-page"]')).toBeVisible();
    });
    await test.step('verify page title', async () => {
      await expect(page.locator('[data-testid="events-reference-page"]')).toContainText('Events Reference');
    });
  });

  test('lists all event kinds with description, usage, and meta columns', async ({ page }) => {
    await test.step('navigate to events reference page', async () => {
      await page.goto('./events');
      await expect(page.locator('[data-testid="events-reference-page"]')).toBeVisible();
    });
    await test.step('verify table columns', async () => {
      const pageContent = page.locator('[data-testid="events-reference-page"]');
      await expect(pageContent).toContainText('Kind');
      await expect(pageContent).toContainText('Description');
      await expect(pageContent).toContainText('Usage');
      await expect(pageContent).toContainText('Meta');
    });
    await test.step('verify core event kinds are present', async () => {
      const pageContent = page.locator('[data-testid="events-reference-page"]');
      await expect(pageContent).toContainText('section');
      await expect(pageContent).toContainText('log');
      await expect(pageContent).toContainText('failure');
      await expect(pageContent).toContainText('state_change');
      await expect(pageContent).toContainText('token_usage');
      await expect(pageContent).toContainText('tag');
    });
    await test.step('verify meta badge and row count', async () => {
      const pageContent = page.locator('[data-testid="events-reference-page"]');
      await expect(pageContent).toContainText('yes');
      const bodyRows = pageContent.locator('table tbody tr');
      const count = await bodyRows.count();
      expect(count).toBeGreaterThanOrEqual(16);
    });
  });

  test('footer explains meta event behavior', async ({ page }) => {
    await test.step('navigate to events reference page', async () => {
      await page.goto('./events');
    });
    await test.step('verify footer text', async () => {
      await expect(page.locator('[data-testid="events-reference-page"]')).toContainText(
        'Meta events are hidden from the run timeline by default'
      );
    });
  });
});
