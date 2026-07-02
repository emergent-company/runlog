import { RunLogClient } from './client.js'
import type { CreateRunOpts } from './types.js'

export interface JestReporterOpts {
  daemonUrl?: string
  runner?: string
}

interface TestInfo {
  runId: string
  category?: string
  file: string
  startTime: number
}

interface JestTestResult {
  title: string
  status: string
  duration?: number
  failureMessages?: string[]
}

interface JestFileResult {
  numFailingTests: number
  testResults: JestTestResult[]
  perfStats?: { start: number; end: number }
}

function deriveTestType(path: string): string {
  if (path.includes('.e2e.')) return 'e2e'
  if (path.includes('.integration.')) return 'integration'
  if (path.includes('.spec.') || path.includes('.test.')) return 'unit'
  return 'other'
}

function stripAnsi(s: string): string {
  return s.replace(/\x1B\[[0-9;]*[a-zA-Z]/g, '').trim()
}

function deriveCategory(fullName: string): string {
  const parts = fullName.replace(/\\/g, '/').split('/')
  const srcIdx = parts.lastIndexOf('src')
  if (srcIdx >= 0 && srcIdx + 1 < parts.length) {
    return parts.slice(srcIdx + 1, -1).join('/')
  }
  return 'uncategorized'
}

function fileShortName(path: string): string {
  const parts = path.replace(/\\/g, '/').split('/')
  return parts.slice(-2).join('/')
}

class RunLogJestReporter {
  private client: RunLogClient
  private opts: JestReporterOpts
  private activeRuns = new Map<string, TestInfo>()

  constructor(
    _globalConfig: unknown,
    opts?: JestReporterOpts,
  ) {
    this.opts = opts ?? {}
    this.client = new RunLogClient(
      this.opts.daemonUrl ?? 'http://localhost:5002',
    )
  }

  async onTestFileStart(test: { path: string }): Promise<void> {
    const testName = test.path
    const shortName = fileShortName(testName)
    const createOpts: CreateRunOpts = {
      testName: shortName,
      runner: this.opts.runner ?? 'jest',
      category: deriveCategory(testName),
      description: testName,
      testType: deriveTestType(testName),
    }
    try {
      const result = await this.client.createRun(createOpts)
      this.activeRuns.set(testName, {
        runId: result.id,
        category: createOpts.category,
        file: testName,
        startTime: Date.now(),
      })
      await this.client.section(result.id, shortName)
    } catch (err) {
      // daemon not running — silently degrade
    }
  }

  async onTestFileResult(
    test: { path: string },
    result: JestFileResult,
  ): Promise<void> {
    const testName = test.path
    const info = this.activeRuns.get(testName)
    if (!info) return

    try {
      let suiteFailed = false
      for (const tr of result.testResults) {
        const elapsed = tr.duration ?? 0
        const msg = tr.failureMessages?.[0] ?? ''
        suiteFailed = suiteFailed || tr.status === 'failed'
        await this.client.addEvent(info.runId, {
          kind: 'assertion',
          message: tr.title,
          elapsedS: elapsed / 1000,
          details: {
            expected: 'passed',
            actual: tr.status,
            status: tr.status,
            duration_ms: elapsed,
            ...(tr.status === 'failed' ? {
              error: stripAnsi(msg.split('\n')[0]),
              stack: stripAnsi(msg.split('\n').slice(1, 6).join('\n')),
            } : {}),
          },
        })
      }

      await this.client.markDone(info.runId, {
        passed: !suiteFailed,
      })
    } catch {
      // best-effort
    } finally {
      this.activeRuns.delete(testName)
    }
  }

  async onRunComplete(): Promise<void> {
    for (const [testName, info] of this.activeRuns) {
      try {
        await this.client.markDone(info.runId, { passed: false, reason: 'test suite did not complete' })
      } catch {
        // best-effort
      }
      this.activeRuns.delete(testName)
    }
  }
}

export default RunLogJestReporter
