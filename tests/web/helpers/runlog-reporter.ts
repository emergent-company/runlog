import type {
  FullConfig,
  FullResult,
  Reporter,
  Suite,
  TestCase,
  TestResult,
  TestStep,
} from '@playwright/test/reporter';
import * as fs from 'fs';
import * as path from 'path';

const DOGFOOD_URL = process.env.RUNLOG_DAEMON_URL || 'http://localhost:17433';
// Artifacts directory must match what the dogfood daemon serves from.
// Set RUNLOG_ARTIFACTS_DIR or configure artifacts_dir in .runlog/config.yaml.
const ARTIFACTS_DIR = process.env.RUNLOG_ARTIFACTS_DIR || '/tmp/runlog-artifacts';

// Per-worker: maps test.id -> { runId, startTime, stepFailureEmitted }
const runMap = new Map<string, { runId: string; startTime: number; stepFailureEmitted: boolean }>();

// Playwright-internal steps that carry no useful test information.
const NOISE_STEPS = new Set([
  'Launch browser',
  'Create page',
  'Close context',
  'Close browser',
  'browserContext.close',
  'browser.close',
]);

async function post(path: string, body: unknown): Promise<any> {
  try {
    const resp = await fetch(`${DOGFOOD_URL}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(2000),
    });
    if (!resp.ok) return null;
    return resp.json().catch(() => null);
  } catch {
    return null;
  }
}

async function put(path: string, body: unknown): Promise<void> {
  try {
    await fetch(`${DOGFOOD_URL}${path}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(2000),
    });
  } catch { /* fail-open */ }
}

async function addEvent(
  runId: string,
  kind: string,
  message: string,
  elapsedS: number,
  details?: Record<string, unknown>,
): Promise<void> {
  const body: Record<string, unknown> = { kind, message, elapsed_s: elapsedS };
  if (details && Object.keys(details).length > 0) body.details = details;
  await post(`/runs/${runId}/events`, body);
}

function elapsedFrom(startTime: number): number {
  return (Date.now() - startTime) / 1000;
}

function locationString(step: TestStep): string {
  if (!step.location) return '';
  return `${step.location.file.replace(/^.*\/specs\//, 'specs/')}:${step.location.line}`;
}

// Extract context config details from the test's project use options.
function contextDetails(test: TestCase): Record<string, unknown> {
  const use = test.parent.project()?.use as Record<string, unknown> | undefined;
  if (!use) return {};
  const d: Record<string, unknown> = {};
  if (use.baseURL)              d.baseURL     = use.baseURL;
  if (use.headless !== undefined) d.headless  = use.headless;
  if (use.viewport)             d.viewport    = use.viewport;
  if (use.trace)                d.trace       = use.trace;
  if (use.screenshot)           d.screenshot  = use.screenshot;
  return d;
}

// Parse a Playwright pw:api step title into structured detail fields.
// Returns an object with zero or more of: url, locator, value, key.
// Patterns (from Playwright source title templates):
//   Navigate to "url"
//   Fill locator('...') with "value"
//   Press locator('...') key "key"
//   Click/Check/Uncheck/Hover/Tap/Double click/Select option/Drag and drop  locator('...')
//   Wait for URL "pattern"
function parsePWActionTitle(title: string): Record<string, unknown> {
  // Navigate to "url"
  let m = title.match(/^Navigate to "(.+)"$/);
  if (m) return { url: m[1] };

  // Fill locator('...') with "value"
  m = title.match(/^Fill (locator\(.+?\)) with "(.+)"$/);
  if (m) return { locator: m[1], value: m[2] };

  // Press locator('...') key "key"
  m = title.match(/^Press (locator\(.+?\)) key "(.+)"$/);
  if (m) return { locator: m[1], key: m[2] };

  // Wait for URL "pattern"
  m = title.match(/^Wait for URL "(.+)"$/);
  if (m) return { url: m[1] };

  // Action locator('...')  — click, check, uncheck, hover, tap, select option, etc.
  m = title.match(/^(?:Click|Check|Uncheck|Hover|Tap|Double click|Select option|Drag and drop|Get (?:text|inner|attribute|property|value)|Focus|Blur|Scroll into view) (locator\(.+\))$/);
  if (m) return { locator: m[1] };

  return {};
}

// Parse Playwright expect step title into {matcher, locator}.
// Title format: Expect "toBeVisible" locator('[data-testid="..."]')
//           or: Expect "toContainText" locator('...') with timeout 15000
function parseExpectTitle(title: string): { matcher: string; locator: string } {
  const m = title.match(/^Expect "([^"]+)"\s+(.+?)(?:\s+with\s+\w|$)/);
  if (m) return { matcher: m[1], locator: m[2].trim() };
  return { matcher: title, locator: '' };
}

// Shorten pw_assert message to: expect.matcher (locator stripped — it's in details)
function shortAssertMessage(matcher: string): string {
  return `expect.${matcher}`;
}

// summarizeError strips ANSI escape codes and reduces a (possibly multi-line,
// Playwright-formatted) error message to a short, clean one-liner suitable
// for the events table Message column. The full raw text (with ANSI intact)
// is preserved separately in event details for rich/colorized display.
function summarizeError(msg: string): string {
  const clean = msg.replace(/\x1b\[[0-9;]*m/g, '');
  const firstLine = clean.split('\n').find(l => l.trim().length > 0) || 'Test failed';
  const trimmed = firstLine.trim();
  return trimmed.length > 150 ? trimmed.slice(0, 150) + '…' : trimmed;
}

// Extract expected/actual from a Playwright assertion error message.
function parseExpectError(msg: string): { expected: string; actual: string } | null {
  if (!msg) return null;
  const expectedMatch = msg.match(/Expected(?:\s+\w+)?:\s*(.+)/);
  const receivedMatch = msg.match(/Received(?:\s+\w+)?:\s*(.+)/);
  if (expectedMatch || receivedMatch) {
    return {
      expected: expectedMatch?.[1]?.trim() ?? '',
      actual:   receivedMatch?.[1]?.trim() ?? '',
    };
  }
  return null;
}

class RunLogReporter implements Reporter {
  onBegin(_config: FullConfig, _suite: Suite): void {}

  async onTestBegin(test: TestCase, result: TestResult): Promise<void> {
    const startTime = result.startTime.getTime();
    const titlePath = test.titlePath();
    const fullName  = titlePath.filter(Boolean).join(' › ');
    const category  = titlePath.slice(0, -1).filter(Boolean).join(' › ');
    const fileShort = test.location.file.replace(/^.*\/(specs\/.+)$/, '$1');

    const data = await post('/runs', {
      pid: process.pid,
      env_profile: fullName,
      category,
      runner: 'playwright',
    });

    const runId: string | null = data?.id ?? null;
    if (!runId) return;

    runMap.set(test.id, { runId, startTime, stepFailureEmitted: false });
    process.env.RUNLOG_RUN_ID = runId;

    await addEvent(runId, 'state_change', 'test started', 0);
    await addEvent(runId, 'log', `file: ${fileShort}`, 0.01);
    await addEvent(runId, 'log', `category: ${category}`, 0.02);
  }

  async onStepBegin(test: TestCase, _result: TestResult, step: TestStep): Promise<void> {
    const entry = runMap.get(test.id);
    if (!entry) return;
    const { runId, startTime } = entry;
    const elapsed = (step.startTime.getTime() - startTime) / 1000;

    switch (step.category) {
      case 'test.step':
        await addEvent(runId, 'pw_step', step.title, elapsed);
        break;

      case 'pw:api': {
        if (NOISE_STEPS.has(step.title)) return;

        const details: Record<string, unknown> = {};

        if (step.title === 'Create context') {
          // Include project config so users can see what browser context is in effect.
          Object.assign(details, contextDetails(test));
        } else {
          // Parse structured data from the step title.
          Object.assign(details, parsePWActionTitle(step.title));
        }

        const loc = locationString(step);
        if (loc) details.location = loc;

        await addEvent(runId, 'pw_action', step.title, elapsed, details);
        break;
      }

      case 'expect': {
        const { matcher, locator } = parseExpectTitle(step.title);
        const details: Record<string, unknown> = { matcher, passed: true };
        if (locator) details.locator = locator;
        const loc = locationString(step);
        if (loc) details.location = loc;
        // Shorten the timeline message — locator detail is in the expanded view.
        await addEvent(runId, 'pw_assert', shortAssertMessage(matcher), elapsed, details);
        break;
      }

      default:
        break;
    }
  }

  async onStepEnd(test: TestCase, _result: TestResult, step: TestStep): Promise<void> {
    const entry = runMap.get(test.id);
    if (!entry) return;
    const { runId, startTime } = entry;

    if (!step.error) return;

    const elapsed = (step.startTime.getTime() + step.duration - startTime) / 1000;
    entry.stepFailureEmitted = true;

    if (step.category === 'expect') {
      const { matcher, locator } = parseExpectTitle(step.title);
      const parsed = parseExpectError(step.error.message ?? '');
      const details: Record<string, unknown> = { matcher, passed: false, error: step.error.message };
      if (locator) details.locator = locator;
      if (parsed) {
        details.expected = parsed.expected;
        details.actual   = parsed.actual;
      }
      const loc = locationString(step);
      if (loc) details.location = loc;
      await addEvent(runId, 'failure', summarizeError(step.error.message ?? step.title), elapsed, details);
    } else {
      await addEvent(runId, 'failure', summarizeError(step.error.message ?? step.title), elapsed, {
        step_kind:  step.category,
        step_title: step.title,
        location:   locationString(step),
        error:      step.error.message,
      });
    }
  }

  async onTestEnd(test: TestCase, result: TestResult): Promise<void> {
    const entry = runMap.get(test.id);
    if (!entry) return;
    const { runId, startTime } = entry;
    runMap.delete(test.id);
    if (process.env.RUNLOG_RUN_ID === runId) delete process.env.RUNLOG_RUN_ID;

    const elapsed = elapsedFrom(startTime);
    const passed  = result.status === 'passed';

    // Only emit a failure event if onStepEnd didn't already capture one.
    // This avoids duplicates: onStepEnd covers step-level failures with details;
    // this fallback catches test-level errors (e.g. fixture setup failures).
    if (!passed && result.error?.message && !entry.stepFailureEmitted) {
      await addEvent(runId, 'failure', summarizeError(result.error.message), elapsed, { error: result.error.message });
    }

    // Copy attachments (trace, screenshots) into the artifacts dir and emit events.
    // This runs after Playwright has fully written all attachments.
    const artifactDir = path.join(ARTIFACTS_DIR, runId);
    try {
      fs.mkdirSync(artifactDir, { recursive: true });
      for (const att of result.attachments) {
        if (!att.path || !fs.existsSync(att.path)) continue;
        if (att.name === 'trace' && att.contentType === 'application/zip') {
          const dest = path.join(artifactDir, 'trace.zip');
          fs.copyFileSync(att.path, dest);
          await addEvent(runId, 'artifact', 'trace', elapsedFrom(startTime), {
            type: 'trace',
            url: `/artifact/${runId}/trace.zip`,
            mime: 'application/zip',
          });
        } else if (att.name === 'screenshot' && att.contentType?.startsWith('image/')) {
          const ext = path.extname(att.path) || '.png';
          const dest = path.join(artifactDir, `screenshot${ext}`);
          fs.copyFileSync(att.path, dest);
          await addEvent(runId, 'artifact', 'screenshot', elapsedFrom(startTime), {
            type: 'screenshot',
            url: `/artifact/${runId}/screenshot${ext}`,
            mime: att.contentType,
          });
        }
      }
    } catch { /* fail-open */ }

    await addEvent(runId, 'state_change', 'test finished', elapsedFrom(startTime));
    await put(`/runs/${runId}/done`, { passed, reason: result.error?.message ?? '' });
  }

  onEnd(_result: FullResult): void {}
}

export default RunLogReporter;
