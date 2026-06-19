# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: timeout.spec.ts >> Test timeout >> run is marked as timed out after reap when test never finishes
- Location: specs/timeout.spec.ts:7:7

# Error details

```
Error: expect(locator).toBeVisible() failed

Locator: locator('[data-testid="run-detail-content"]')
Expected: visible
Timeout: 15000ms
Error: element(s) not found

Call log:
  - Expect "toBeVisible" with timeout 15000ms
  - waiting for locator('[data-testid="run-detail-content"]')
    - waiting for navigation to finish...
    - navigated to "http://localhost:17430/ui/launch/TestFail"

```

```yaml
- text: test "TestFail" is already running
```

# Test source

```ts
  1  | import { test, expect } from './fixtures';
  2  | import { execSync } from 'child_process';
  3  | 
  4  | const DB_PATH = '/tmp/runlog-e2e.db';
  5  | 
  6  | test.describe('Test timeout', () => {
  7  |   test('run is marked as timed out after reap when test never finishes', async ({ page }) => {
  8  |     let runId: string | undefined;
  9  | 
  10 |     await test.step('launch TestFail from tests table to create a new run', async () => {
  11 |       await page.goto('tests');
  12 |       await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
  13 | 
  14 |       const testRow = page.locator('tr[id^="test-"]').filter({ hasText: 'TestFail' }).first();
  15 |       const runTestBtn = testRow.locator('[data-run-test]');
  16 |       await expect(runTestBtn).toBeAttached();
  17 |       await runTestBtn.dispatchEvent('click');
  18 | 
> 19 |       await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 15000 });
     |                                                                        ^ Error: expect(locator).toBeVisible() failed
  20 |     });
  21 | 
  22 |     await test.step('extract run ID from URL', async () => {
  23 |       const m = page.url().match(/\/ui\/runs\/(\d+)/);
  24 |       expect(m).not.toBeNull();
  25 |       runId = m![1];
  26 |       expect(parseInt(runId!)).toBeGreaterThan(0);
  27 |     });
  28 | 
  29 |     await test.step('backdate started_at so run appears stale (past the default 30min timeout)', async () => {
  30 |       execSync(
  31 |         `sqlite3 "${DB_PATH}" "UPDATE test_runs SET started_at = datetime('now', '-1 hour') WHERE id = ${runId}"`,
  32 |         { timeout: 5000, stdio: 'pipe' },
  33 |       );
  34 |     });
  35 | 
  36 |     await test.step('trigger /reap endpoint on the daemon', async () => {
  37 |       const resp = await page.request.post('http://localhost:17430/reap');
  38 |       expect(resp.ok()).toBeTruthy();
  39 |     });
  40 | 
  41 |     await test.step('navigate to run detail page and verify timed out status', async () => {
  42 |       await page.goto(`runs/${runId}`);
  43 |       await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
  44 |       await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('Fail');
  45 |       await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('timed out');
  46 |     });
  47 |   });
  48 | });
  49 | 
```