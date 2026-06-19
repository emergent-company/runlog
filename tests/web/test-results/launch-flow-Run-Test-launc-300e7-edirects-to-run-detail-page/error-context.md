# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: launch-flow.spec.ts >> Run Test launch flow >> clicking Run Test on test detail page redirects to run detail page
- Location: specs/launch-flow.spec.ts:23:7

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

```

```yaml
- text: runlog
- paragraph: Navigation
- link "Dashboard":
  - /url: /ui/
- link "Tests":
  - /url: /ui/tests
- link "All Runs":
  - /url: /ui/runs
- paragraph: Reference
- link "Events":
  - /url: /ui/events
- link "Experiments":
  - /url: /ui/experiments
- navigation "Navbar": runlog
- main:
  - button "← Back"
  - heading "TestPass" [level=1]
  - button "test \"TestPass\" is already running"
  - heading "Avg Duration" [level=2]
  - text: 4.7s
  - heading "Min" [level=2]
  - text: 3.0s
  - heading "Max" [level=2]
  - text: 8.0s
  - heading "Pass Rate" [level=2]
  - text: 67% Duration Trend —
  - img
  - paragraph: 4 run(s)
  - textbox "Filter by tag..."
  - table:
    - rowgroup:
      - row "ID Test Status Duration Started Tags":
        - columnheader "ID"
        - columnheader "Test"
        - columnheader "Status"
        - columnheader "Duration"
        - columnheader "Started"
        - columnheader "Tags"
    - rowgroup:
      - row "10 TestPass Running running... 21:25:59 18-06-2026":
        - cell "10"
        - cell "TestPass"
        - cell "Running"
        - cell "running..."
        - cell "21:25:59 18-06-2026"
        - cell
      - row "8 TestPass Fail 3.0s 21:00:59 18-06-2026":
        - cell "8"
        - cell "TestPass"
        - cell "Fail"
        - cell "3.0s"
        - cell "21:00:59 18-06-2026"
        - cell
      - row "7 TestPass Pass 3.0s 20:50:59 18-06-2026":
        - cell "7"
        - cell "TestPass"
        - cell "Pass"
        - cell "3.0s"
        - cell "20:50:59 18-06-2026"
        - cell
      - row "1 TestPass Pass 8.0s 18:55:59 18-06-2026":
        - cell "1"
        - cell "TestPass"
        - cell "Pass"
        - cell "8.0s"
        - cell "18:55:59 18-06-2026"
        - cell
```

# Test source

```ts
  1  | import { test, expect } from './fixtures';
  2  | 
  3  | test.describe('Run Test launch flow', () => {
  4  |   test('clicking Run Test from tests table redirects to run detail', async ({ page }) => {
  5  |     await test.step('navigate to tests page', async () => {
  6  |       await page.goto('tests');
  7  |       await expect(page.locator('[data-testid="tests-content"]')).toBeVisible();
  8  |     });
  9  | 
  10 |     await test.step('click Run Test for TestFail', async () => {
  11 |       const testRow = page.locator('tr[id^="test-"]').filter({ hasText: 'TestFail' }).first();
  12 |       const runTestBtn = testRow.locator('[data-run-test]');
  13 |       await expect(runTestBtn).toBeAttached();
  14 |       await runTestBtn.dispatchEvent('click');
  15 |     });
  16 | 
  17 |     await test.step('verify redirect to run detail page with TestFail name', async () => {
  18 |       await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 15000 });
  19 |       await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('TestFail');
  20 |     });
  21 |   });
  22 | 
  23 |   test('clicking Run Test on test detail page redirects to run detail page', async ({ page }) => {
  24 |     await test.step('navigate to TestPass detail page', async () => {
  25 |       await page.goto('tests/TestPass');
  26 |       await expect(page.locator('[data-testid="test-detail-content"]')).toBeVisible();
  27 |     });
  28 | 
  29 |     await test.step('click Run Test button on detail page', async () => {
  30 |       const runTestBtn = page.locator('[data-testid="run-test-btn"]');
  31 |       await expect(runTestBtn).toBeVisible();
  32 |       await runTestBtn.dispatchEvent('click');
  33 |     });
  34 | 
  35 |     await test.step('verify redirect to run detail page with TestPass name', async () => {
> 36 |       await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 15000 });
     |                                                                        ^ Error: expect(locator).toBeVisible() failed
  37 |       await expect(page.locator('[data-testid="run-detail-content"]')).toContainText('TestPass');
  38 |     });
  39 |   });
  40 | });
  41 | 
```