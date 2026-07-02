import { defineConfig } from 'tsup';

export default defineConfig({
  entry: {
    index: 'src/index.ts',
    reporter: 'src/reporter.ts',
    playwright: 'src/playwright.ts',
  },
  format: ['esm', 'cjs'],
  dts: true,
  clean: true,
  external: ['@playwright/test', 'jest'],
  sourcemap: true,
});
