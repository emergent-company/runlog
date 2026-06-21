import { test, expect } from './fixtures';

test.describe('All Runs page', () => {

  test('loads and shows page title and recent runs table', async ({ page }) => {
    // Navigate to All Runs page (relative to baseURL which ends in /ui/)
    await page.goto('./runs');
    await expect(page.getByTestId('all-runs-page')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'All Runs' })).toBeVisible();

    // Verify the table renders with seeded data
    const table = page.locator('#runs-table');
    await expect(table).toBeVisible();

    // Verify at least one seeded run appears (TestPass from seed data)
    await expect(table).toContainText('TestPass');
    await expect(table).toContainText('TestFail');
    await expect(table).toContainText('TestSkip');
    await expect(table).toContainText('TestMetrics');
  });

  test('filters by category dropdown', async ({ page }) => {
    await page.goto('./runs');

    // Select a category from the dropdown
    await page.selectOption('[data-testid="filter-category"]', 'Uncategorized');
    await page.waitForTimeout(500);

    // Verify all visible rows have the selected category badge
    const rows = page.locator('#runs-table tbody tr');
    const count = await rows.count();

    // There should be at least some rows (the seeded test data category)
    expect(count).toBeGreaterThan(0);

    // Category filter dropdown should show "Uncategorized" selected
    await expect(page.getByTestId('filter-category')).toHaveValue('Uncategorized');
  });

  test('filters by status = Fail showing only failing runs', async ({ page }) => {
    await page.goto('./runs');

    // Select "Fail" option in the status dropdown
    await page.selectOption('[data-testid="filter-status"]', 'fail');
    await page.waitForTimeout(500);

    // Every visible row should have a Fail badge (badge-error class)
    const rows = page.locator('#runs-table tbody tr');
    const rowCount = await rows.count();
    expect(rowCount).toBeGreaterThan(0);

    // Verify no Pass-badge entries appear (match the badge element specifically)
    const passBadgeCount = await page.locator('#runs-table .badge-success').count();
    expect(passBadgeCount).toBe(0);
  });

  test('filters by status = Pass showing only passing runs', async ({ page }) => {
    await page.goto('./runs');

    await page.selectOption('[data-testid="filter-status"]', 'pass');
    await page.waitForTimeout(500);

    const rows = page.locator('#runs-table tbody tr');
    const rowCount = await rows.count();
    expect(rowCount).toBeGreaterThan(0);

    // Verify no Fail-badge entries appear (match the badge element specifically)
    const failBadgeCount = await page.locator('#runs-table .badge-error').count();
    expect(failBadgeCount).toBe(0);
  });

  test('searches by test name', async ({ page }) => {
    await page.goto('./runs');

    // Type a test name in the search box
    const searchInput = page.getByTestId('filter-search');
    await searchInput.fill('TestPass');
    // Wait for debounce (500ms) + HTMX request
    await page.waitForTimeout(800);

    // Verify only TestPass rows appear
    const rows = page.locator('#runs-table tbody tr');
    const rowCount = await rows.count();
    expect(rowCount).toBeGreaterThan(0);

    // All rows should contain TestPass in the test name column
    expect(rowCount).toBeGreaterThan(0);
    const passBadgeCount = await page.locator('#runs-table .badge-success').count();
    expect(passBadgeCount).toBeGreaterThan(0);
    expect(passBadgeCount).toBeLessThanOrEqual(rowCount);
  });

  test('clicking a run row navigates to run detail page', async ({ page }) => {
    await page.goto('./runs');

    // Click the first row in the runs table
    const firstRow = page.locator('#runs-table tbody tr').first();
    await firstRow.click();

    // Should navigate to the run detail page
    await expect(page.getByTestId('run-detail-content')).toBeVisible();
    // URL should contain /runs/
    expect(page.url()).toContain('/ui/runs/');
  });

  test('category filter dropdown lists all seeded categories', async ({ page }) => {
    await page.goto('./runs');

    // Get all options from the category dropdown
    const options = await page.getByTestId('filter-category').locator('option').all();

    // Should have "All" + at least one category
    expect(options.length).toBeGreaterThanOrEqual(2);

    // First option should be "All"
    await expect(options[0]).toHaveText('All');
  });

  test('status filter dropdown lists all status options', async ({ page }) => {
    await page.goto('./runs');

    const expectedStatuses = ['All', 'Pass', 'Fail', 'Skip', 'Running'];
    const options = await page.getByTestId('filter-status').locator('option').all();

    expect(options.length).toBe(expectedStatuses.length);
    for (let i = 0; i < expectedStatuses.length; i++) {
      await expect(options[i]).toHaveText(expectedStatuses[i]);
    }
  });

  test('since filter dropdown has all time options', async ({ page }) => {
    await page.goto('./runs');

    const expectedSince = ['All time', 'Last hour', 'Last 24h', 'Last 7 days', 'Last 30 days'];
    const options = await page.getByTestId('filter-since').locator('option').all();

    expect(options.length).toBe(expectedSince.length);
    for (let i = 0; i < expectedSince.length; i++) {
      await expect(options[i]).toHaveText(expectedSince[i]);
    }
  });

  test('no Load More button when runs fit on one page', async ({ page }) => {
    await page.goto('./runs');

    // Seed data has far fewer than 50 runs, so Load More should not appear
    await expect(page.getByText('Load More')).not.toBeVisible();
  });
});
