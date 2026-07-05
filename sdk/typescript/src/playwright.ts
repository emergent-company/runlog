import { test as base, expect } from '@playwright/test'
import type { Page, TestInfo } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
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

// artifactsDir resolves where screenshots/traces are written on disk. Must
// match the runlog daemon's artifacts_dir (config.yaml) / RUNLOG_ARTIFACTS_DIR
// so the daemon can serve them back at /artifact/<runId>/<file>.
function artifactsDir(): string {
  return process.env.RUNLOG_ARTIFACTS_DIR || path.resolve(process.cwd(), '.runlog', 'artifacts')
}

// summarizeError strips ANSI escape codes and reduces a (possibly multi-line)
// error message to a short, clean one-liner suitable for the events table.
function summarizeError(msg: string): string {
  const clean = msg.replace(/\x1b\[[0-9;]*m/g, '')
  const firstLine = clean.split('\n').find((l) => l.trim().length > 0) || 'Test failed'
  const trimmed = firstLine.trim()
  return trimmed.length > 150 ? trimmed.slice(0, 150) + '…' : trimmed
}

// saveArtifacts captures a full-page screenshot and, if a trace was recorded,
// copies it next to the screenshot — then emits one "artifact" event per file
// so the runlog UI can render them inline on the run detail page.
async function saveArtifacts(client: RunLogClient, runId: string, page: Page, testInfo: TestInfo): Promise<void> {
  const dir = path.join(artifactsDir(), runId)
  try {
    fs.mkdirSync(dir, { recursive: true })

    const ssPath = path.join(dir, 'screenshot.png')
    await page.screenshot({ path: ssPath, fullPage: true }).catch(() => {})
    if (fs.existsSync(ssPath)) {
      await client
        .addEvent(runId, {
          kind: 'artifact',
          message: 'screenshot',
          details: { type: 'screenshot', url: `/artifact/${runId}/screenshot.png`, mime: 'image/png' },
        })
        .catch(() => {})
    }

    const traceSrc = testInfo.outputPath('trace.zip')
    if (fs.existsSync(traceSrc)) {
      const dest = path.join(dir, 'trace.zip')
      fs.copyFileSync(traceSrc, dest)
      await client
        .addEvent(runId, {
          kind: 'artifact',
          message: 'trace',
          details: { type: 'trace', url: `/artifact/${runId}/trace.zip`, mime: 'application/zip' },
        })
        .catch(() => {})
    }
  } catch {
    // fail-open — artifact capture must never fail the test itself
  }
}

// RunLog auto-fixture: registers a run via the daemon HTTP API (never raw SQL),
// auto-captures XHR/fetch/document network traffic and uncaught page errors as
// events, and auto-uploads a screenshot + trace on completion. Marked
// `{ auto: true }` so it applies to every test with zero test-body changes —
// destructure `{ runlog }` only if you want to add manual `.section()`/`.log()`
// annotations on top of the automatic capture.
export const test = base.extend<{ runlog: RunLogFixture }>({
  runlog: [
    async ({ page }, use, testInfo) => {
      const client = new RunLogClient()
      const category = deriveCategory(testInfo.file)
      const startedAt = Date.now()
      const elapsedS = () => (Date.now() - startedAt) / 1000

      let runId: string | null = null
      try {
        const result = await client.createRun({
          testName: testInfo.title,
          category,
          tags: testInfo.titlePath.slice(1),
          runner: 'playwright',
          testType: 'e2e',
        })
        runId = result.id
      } catch {
        // daemon unreachable — degrade to a no-op fixture, never fail the test
      }

      if (runId) {
        const rid = runId
        // Only the response is recorded as an http_call event — it already
        // carries method+url+status+body+elapsed, a strict superset of what
        // the outgoing request alone would show. Emitting both would produce
        // duplicate/confusing rows (a request-time entry with no status or
        // body, interleaved out of order with unrelated events for
        // long-lived requests like SSE streams).
        page.on('response', (resp) => {
          const rt = resp.request().resourceType()
          if (rt !== 'xhr' && rt !== 'fetch' && rt !== 'document') return
          resp
            .text()
            .then((body) =>
              client.addEvent(rid, {
                kind: 'http_call',
                message: `${resp.request().method()} ${resp.url()} → ${resp.status()}`,
                elapsedS: elapsedS(),
                details: {
                  method: resp.request().method(),
                  url: resp.url(),
                  status_code: resp.status(),
                  response_body: body.slice(0, 2048),
                },
              }),
            )
            .catch(() => {})
        })
        page.on('requestfailed', (req) => {
          const rt = req.resourceType()
          if (rt !== 'xhr' && rt !== 'fetch' && rt !== 'document') return
          client
            .addEvent(rid, {
              kind: 'http_call',
              message: `${req.method()} ${req.url()} → failed`,
              elapsedS: elapsedS(),
              details: {
                method: req.method(),
                url: req.url(),
                error: req.failure()?.errorText ?? 'request failed',
              },
            })
            .catch(() => {})
        })
        page.on('pageerror', (err) => {
          client
            .addEvent(rid, { kind: 'log', message: `uncaught page error: ${err.message}`, elapsedS: elapsedS() })
            .catch(() => {})
        })
      }

      const fixture: RunLogFixture = {
        client,
        runId: runId ?? '',
        section: async (name) => {
          if (runId) await client.section(runId, name).catch(() => {})
        },
        log: async (msg) => {
          if (runId) await client.log(runId, msg).catch(() => {})
        },
        fail: async (msg) => {
          if (runId) await client.fail(runId, msg).catch(() => {})
        },
        cli: async (inv, out) => {
          if (runId) await client.cli(runId, inv, out).catch(() => {})
        },
        markDone: async (opts) => {
          if (runId) await client.markDone(runId, opts).catch(() => {})
        },
      }

      try {
        await use(fixture)
        if (runId) {
          await saveArtifacts(client, runId, page, testInfo)
          await client
            .markDone(runId, {
              passed: testInfo.status === testInfo.expectedStatus,
              ...(testInfo.error ? { reason: summarizeError(testInfo.error.message ?? String(testInfo.error)) } : {}),
            })
            .catch(() => {})
        }
      } catch (e) {
        if (runId) {
          await saveArtifacts(client, runId, page, testInfo).catch(() => {})
          await client
            .markDone(runId, {
              passed: false,
              reason: summarizeError(e instanceof Error ? e.message : String(e)),
            })
            .catch(() => {})
        }
        throw e
      }
    },
    { auto: true },
  ],
})

export { expect }
export type { Page }
