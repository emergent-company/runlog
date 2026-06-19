import { test, expect } from './fixtures';
import { execSync } from 'child_process';

const DB_PATH = '/tmp/runlog-e2e.db';

test.describe('Test timeout', () => {
  test('run is marked as timed out after reap when test never finishes', async ({ page }) => {
    let runId: string | undefined;

    await test.step('launch TestFail from tests table to create a new run', async () => {
      await page.goto('tests');
      await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();

      const testRow = page.locator('tr[id^="test-"]').filter({ hasText: 'TestFail' }).first();
      const runTestBtn = testRow.locator('[data-run-test]');
      await expect(runTestBtn).toBeAttached();
      await runTestBtn.dispatchEvent('click');

      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 15000 });
    });

    await test.step('extract run ID from URL', async () => {
      const m = page.url().match(/\/ui\/runs\/(\d+)/);
      expect(m).not.toBeNull();
      runId = m![1];
      expect(parseInt(runId!)).toBeGreaterThan(0);
    });

    await test.step('backdate started_at so run appears stale (past the default 30min timeout)', async () => {
      execSync(
        `sqlite3 "${DB_PATH}" "UPDATE test_runs SET started_at = datetime('now', '-1 hour') WHERE id = ${runId}"`,
        { timeout: 5000, stdio: 'pipe' },
      );
    });

    await test.step('trigger /reap endpoint on the daemon', async () => {
      const resp = await page.request.post('http://localhost:17430/reap');
      expect(resp.ok()).toBeTruthy();
    });

    await test.step('navigate to run detail page and verify timed out status', async () => {
      await page.goto(`runs/${runId}`);
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
      await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('Fail');
      await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('timed out');
    });
  });
});
