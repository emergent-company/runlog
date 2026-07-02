import { test as base, expect } from '@playwright/test'
import { RunLogClient } from './client.js'
import type { MarkDoneOpts } from './types.js'

export interface RunLogFixture {
  client: RunLogClient
  runId: string
  section(name: string): Promise<void>
  log(message: string): Promise<void>
  fail(message: string): Promise<void>
  cli(invocation: string, output?: string): Promise<void>
  markDone(opts?: MarkDoneOpts): Promise<void>
}

function deriveCategory(testFile: string): string {
  const file = testFile.replace(/\\/g, '/')
  const parts = file.split('/tests/')
  if (parts.length >= 2) {
    return parts[1].replace(/\.spec\.[^.]+$/, '')
  }
  return 'uncategorized'
}

export const test = base.extend<{ runlog: RunLogFixture }>({
  runlog: async ({}, use, testInfo) => {
    const client = new RunLogClient()
    const category = deriveCategory(testInfo.file)

    const result = await client.createRun({
      testName: testInfo.title,
      category,
      tags: testInfo.titlePath.slice(1),
      runner: 'playwright',
      testType: 'e2e',
    })
    const runId = result.id

    const fixture: RunLogFixture = {
      client,
      runId,
      section: (name) => client.section(runId, name),
      log: (msg) => client.log(runId, msg),
      fail: (msg) => client.fail(runId, msg),
      cli: (inv, out) => client.cli(runId, inv, out),
      markDone: (opts) => client.markDone(runId, opts),
    }

    try {
      await use(fixture)
    } finally {
      await client.markDone(runId, {
        passed: testInfo.status === testInfo.expectedStatus,
        ...(testInfo.error ? { reason: testInfo.error.message } : {}),
      })
    }
  },
})

export { expect }
