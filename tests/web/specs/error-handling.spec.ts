import { test, expect } from './fixtures';

test.describe('Error handling', () => {
  test('non-existent test name returns 200 with empty detail page, not JSON', async ({ page }) => {
    await test.step('navigate to non-existent test', async () => {
      const resp = await page.goto('tests/TestADKSessions_GetInvalidSession');
      expect(resp?.status()).toBe(200);
      const ct = resp?.headers()['content-type'] || '';
      expect(ct).toContain('text/html');
    });
    await test.step('verify page is HTML, not JSON', async () => {
      await expect(page.locator('body')).toBeVisible();
      const text = await page.locator('body').innerText();
      expect(text).not.toContain('"message"');
    });
  });

  test('non-existent run ID returns 404', async ({ page }) => {
    await test.step('navigate to non-existent run', async () => {
      const resp = await page.goto('runs/999999');
      expect(resp?.status()).toBe(404);
    });
  });

  test('HTMX error response shows error content on page', async ({ page }) => {
    await test.step('intercept a test navigation and return 500', async () => {
      await page.route('**/ui/tests/TestPass', route => route.fulfill({
        status: 500,
        contentType: 'text/html',
        body: '<div>Server Error Occurred</div>',
      }));
    });
    await test.step('navigate to tests page and click TestPass row', async () => {
      await page.goto('tests');
      const respPromise = page.waitForResponse(r =>
        r.url().includes('/ui/tests/TestPass') && r.status() === 500
      );
      await page.locator('tr[id^="test-"]').filter({ hasText: 'TestPass' }).first().click();
      const resp = await respPromise;
      expect(resp.status()).toBe(500);
    });
    await test.step('verify error content appears in the main content area', async () => {
      await expect(page.locator('#main-content')).toContainText('Server Error', { timeout: 5000 });
    });
  });
});
