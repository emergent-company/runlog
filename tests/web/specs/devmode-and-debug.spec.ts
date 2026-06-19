import { test, expect } from './fixtures';

test.describe('go-daisy devmode: data-component attributes', () => {
  test('dashboard page emits data-component on go-daisy elements', async ({ page }) => {
    await test.step('navigate to dashboard', async () => {
      await page.goto('./');
      await expect(page.locator('[data-testid="dashboard-content"]')).toBeVisible();
    });
    await test.step('verify at least one data-component attribute exists', async () => {
      const componentEls = page.locator('[data-component]');
      await expect(componentEls.first()).toBeVisible();
      const count = await componentEls.count();
      expect(count).toBeGreaterThan(0);
    });
  });

  test('data-component values follow package/ComponentName format', async ({ page }) => {
    await test.step('navigate to dashboard', async () => {
      await page.goto('./');
      await expect(page.locator('[data-testid="dashboard-content"]')).toBeVisible();
    });
    await test.step('verify each data-component matches pkg/Name pattern', async () => {
      const values: string[] = await page.locator('[data-component]').evaluateAll(
        (els) => els.map((el) => el.getAttribute('data-component') ?? '')
      );
      expect(values.length).toBeGreaterThan(0);
      for (const val of values) {
        expect(val).toMatch(/^[a-z]+\/[A-Za-z]+/);
      }
    });
  });

  test('run detail page emits data-component on badge and table components', async ({ page }) => {
    await test.step('navigate to TestPass detail page and click first run', async () => {
      await page.goto('tests/TestPass');
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
    });
    await test.step('verify run detail content visible', async () => {
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
    await test.step('verify badge component has data-component attribute', async () => {
      const badge = page.locator('[data-component="ui/Badge"]').first();
      await expect(badge).toBeVisible();
    });
  });
});

test.describe('Debug toggle: merges meta events into events table', () => {
  test('debug toggle is a checkbox, unchecked by default', async ({ page }) => {
    await test.step('navigate to TestMetrics run detail', async () => {
      await page.goto('tests/TestMetrics');
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
    await test.step('verify toggle checkbox exists and is unchecked', async () => {
      const toggle = page.locator('[data-testid="debug-toggle"] input[type="checkbox"]');
      await expect(toggle).toBeVisible();
      await expect(toggle).not.toBeChecked();
    });
  });

  test('meta event rows hidden when toggle off', async ({ page }) => {
    await test.step('navigate to TestMetrics run detail', async () => {
      await page.goto('tests/TestMetrics');
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
    await test.step('verify no muted debug rows visible', async () => {
      const debugRows = page.locator('tr.opacity-50');
      const count = await debugRows.count();
      if (count > 0) {
        // Rows exist in DOM but are hidden by display:none via is-debug-event CSS
        await expect(debugRows.first()).not.toBeVisible();
      }
    });
  });

  test('toggling debug on shows meta event rows with muted style', async ({ page }) => {
    await test.step('navigate to TestMetrics run detail', async () => {
      await page.goto('tests/TestMetrics');
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
    await test.step('enable debug toggle', async () => {
      const toggle = page.locator('[data-testid="debug-toggle"] input[type="checkbox"]');
      await toggle.check();
      await page.waitForFunction(() =>
        document.getElementById('events-section')?.classList.contains('debug-visible')
      );
    });
    await test.step('verify debug rows appear with opacity-50 class', async () => {
      const debugRows = page.locator('tr.opacity-50');
      await expect(debugRows.first()).toBeVisible({ timeout: 5000 });
      const count = await debugRows.count();
      expect(count).toBeGreaterThan(0);
    });
  });

  test('toggling debug off again hides meta rows', async ({ page }) => {
    await test.step('navigate to TestMetrics run detail', async () => {
      await page.goto('tests/TestMetrics');
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
    await test.step('enable debug toggle', async () => {
      const toggle = page.locator('[data-testid="debug-toggle"] input[type="checkbox"]');
      await toggle.check();
      await page.waitForFunction(() =>
        document.getElementById('events-section')?.classList.contains('debug-visible')
      );
    });
    await test.step('verify debug rows appear', async () => {
      await expect(page.locator('tr.opacity-50').first()).toBeVisible({ timeout: 5000 });
    });
    await test.step('disable debug toggle', async () => {
      const toggle = page.locator('[data-testid="debug-toggle"] input[type="checkbox"]');
      await toggle.uncheck();
      await page.waitForFunction(() =>
        !document.getElementById('events-section')?.classList.contains('debug-visible')
      );
    });
    await test.step('verify debug rows hidden after toggle off', async () => {
      const debugRows = page.locator('tr.opacity-50');
      const count = await debugRows.count();
      if (count > 0) {
        await expect(debugRows.first()).not.toBeVisible();
      }
    });
  });
});

test.describe('Category cards: correct text format', () => {
  test('category cards show "N tests" with a space', async ({ page }) => {
    await test.step('navigate to dashboard', async () => {
      await page.goto('./');
      await expect(page.locator('[data-testid="category-cards"]')).toBeVisible();
    });
    await test.step('verify each card text matches "N tests" format', async () => {
      const cards = page.locator('[data-testid="category-cards"] .card');
      const count = await cards.count();
      expect(count).toBeGreaterThan(0);
      for (let i = 0; i < count; i++) {
        const text = await cards.nth(i).innerText();
        expect(text).toMatch(/\d+ tests/);
      }
    });
  });
});

test.describe('Run stat cards: responsive grid on run detail', () => {
  test('run detail has test name outside grid plus stat cards for Status, Duration, Started', async ({ page }) => {
    await test.step('navigate to TestPass run detail', async () => {
      await page.goto('tests/TestPass');
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
    await test.step('verify test name displayed above stat grid', async () => {
      await expect(page.locator('[data-testid="run-test-name"]')).toBeVisible();
      await expect(page.locator('[data-testid="run-test-name"]')).toContainText('TestPass');
    });
    await test.step('verify stat grid labels', async () => {
      const grid = page.locator('[data-testid="run-stat-cards"]');
      await expect(grid).toBeVisible();
      await expect(grid).toContainText('Status');
      await expect(grid).toContainText('Duration');
      await expect(grid).toContainText('Started');
    });
  });

  test('finished run shows token cost grid for metrics test', async ({ page }) => {
    await test.step('navigate to TestMetrics run detail', async () => {
      await page.goto('tests/TestMetrics');
      const firstRun = page.locator('tr[id^="run-"]').first();
      await firstRun.click();
      await expect(page.locator('[data-testid="run-detail-content"]')).toBeVisible({ timeout: 5000 });
    });
    await test.step('verify token cost grid labels', async () => {
      const content = page.locator('[data-testid="run-detail-content"]');
      await expect(content).toContainText('Input Tokens');
      await expect(content).toContainText('Output Tokens');
      await expect(content).toContainText('Cost (USD)');
    });
  });
});
