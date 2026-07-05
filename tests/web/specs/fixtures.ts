import { test as base, expect } from '@emergent-company/runlog-client/playwright';

// tests/web-specific addition on top of the SDK's `runlog` auto-fixture:
// fail the test if the page under test throws any uncaught JS error. This is
// a strict quality gate appropriate for dogfooding runlog's own web UI — the
// shared SDK fixture intentionally does NOT hard-fail on page errors by
// default (too opinionated for arbitrary consumer apps), so it's added here
// as a local auto-fixture instead.
export const test = base.extend<{ captureErrors: void }>({
  captureErrors: [async ({ page }, use) => {
    const errors: string[] = [];
    page.on('pageerror', err => errors.push(err.message));
    await use();
    if (errors.length > 0) {
      throw new Error(`Uncaught JS errors:\n${errors.join('\n')}`);
    }
  }, { auto: true }],
});

export { expect };
