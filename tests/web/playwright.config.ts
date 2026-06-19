import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './specs',
  fullyParallel: true,
  retries: 1,
  use: {
    baseURL: 'http://localhost:17430/ui/',
    headless: true,
    trace: 'on',  // capture trace for every test (viewable at trace.playwright.dev)
  },
  globalSetup: './helpers/global-setup.ts',
  globalTeardown: './helpers/global-teardown.ts',
});
