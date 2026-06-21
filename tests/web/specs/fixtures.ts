import { test as base, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

const DOGFOOD_URL = process.env.RUNLOG_DAEMON_URL || 'http://localhost:17433';
const ARTIFACT_DIR = '/tmp/runlog-artifacts';

async function registerRun(testName: string, category: string): Promise<string | null> {
  try {
    const body = JSON.stringify({ pid: process.pid, env_profile: `${category} / ${testName}` });
    const resp = await fetch(`${DOGFOOD_URL}/runs`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
      signal: AbortSignal.timeout(2000),
    });
    if (!resp.ok) return null;
    const data = await resp.json() as { id: string };
    return data.id;
  } catch { return null; }
}

async function insertEvent(runId: string, kind: string, message: string, elapsedS: number): Promise<void> {
  try {
    await fetch(`${DOGFOOD_URL}/runs/${runId}/events`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind, message, elapsed_s: elapsedS }),
      signal: AbortSignal.timeout(2000),
    });
  } catch { /* fail-open */ }
}

async function insertEventWithDetails(runId: string, kind: string, message: string, elapsedS: number, details: any): Promise<void> {
  try {
    await fetch(`${DOGFOOD_URL}/runs/${runId}/events`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind, message, elapsed_s: elapsedS, details }),
      signal: AbortSignal.timeout(2000),
    });
  } catch { /* fail-open */ }
}

let testStartTime = Date.now();
function elapsedS(): number { return (Date.now() - testStartTime) / 1000; }

async function saveArtifacts(runId: string, page: any, testInfo: any): Promise<string[]> {
  const urls: string[] = [];
  try {
    fs.mkdirSync(path.join(ARTIFACT_DIR, runId), { recursive: true });
    const ssPath = path.join(ARTIFACT_DIR, runId, 'screenshot.png');
    await page.screenshot({ path: ssPath, fullPage: true }).catch(() => {});
    urls.push(`/artifact/${runId}/screenshot.png`);
    const traceSrc = testInfo.outputPath('trace.zip');
    if (traceSrc && fs.existsSync(traceSrc)) {
      fs.copyFileSync(traceSrc, path.join(ARTIFACT_DIR, runId, 'trace.zip'));
      urls.push(`/artifact/${runId}/trace.zip`);
    }
  } catch { /* fail-open */ }
  return urls;
}

async function markDone(runId: string, passed: boolean, errorMsg?: string, artifactUrls?: string[]): Promise<void> {
  try {
    if (artifactUrls && artifactUrls.length > 0) {
      for (const url of artifactUrls) {
        const isImg = url.endsWith('.png') || url.endsWith('.jpg') || url.endsWith('.webp');
        await insertEventWithDetails(runId, 'artifact', isImg ? 'screenshot' : 'trace', elapsedS(), {
          type: isImg ? 'screenshot' : 'trace',
          url,
          mime: isImg ? 'image/png' : 'application/zip',
        });
      }
    }
    if (passed) {
      await insertEvent(runId, 'log', 'Test passed', elapsedS());
    } else {
      const msg = errorMsg || 'Test failed';
      await insertEvent(runId, 'failure', msg, elapsedS());
    }
    await insertEvent(runId, 'state_change', 'test finished', elapsedS());
    await fetch(`${DOGFOOD_URL}/runs/${runId}/done`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ passed, reason: errorMsg || '' }),
      signal: AbortSignal.timeout(2000),
    });
  } catch { /* fail-open */ }
}

export const test = base.extend<{ captureErrors: void; dogfood: string | null }>({
  captureErrors: [async ({ page }, use) => {
    const errors: string[] = [];
    page.on('pageerror', err => errors.push(err.message));
    await use();
    if (errors.length > 0) {
      throw new Error(`Uncaught JS errors:\n${errors.join('\n')}`);
    }
  }, { auto: true }],

  dogfood: [async ({ page }, use, testInfo) => {
    testStartTime = Date.now();
    const fileShort = testInfo.file.replace(/^.*\/(specs\/.+)$/, '$1');
    const fullPath = testInfo.titlePath.join(' › ');
    const category = testInfo.titlePath.slice(0, -1).join(' › ');

    // Event queue: capture page events before runId is available
    let rid: string | null = null;
    const queue: Array<{k: string; m: string; e: number; d?: any}> = [];
    function emit(kind: string, msg: string, el: number, det?: any) {
      if (rid) {
        if (det) { insertEventWithDetails(rid, kind, msg, el, det); }
        else { insertEvent(rid, kind, msg, el); }
      } else {
        queue.push({k: kind, m: msg, e: el, d: det});
      }
    }
    function flush() {
      for (const q of queue) {
        if (q.d) { insertEventWithDetails(rid!, q.k, q.m, q.e, q.d); }
        else { insertEvent(rid!, q.k, q.m, q.e); }
      }
      queue.length = 0;
    }

    page.on('request', req => {
      const u = req.url();
      if (u.includes('/ui/') && req.method() !== 'OPTIONS') {
        emit('cli', `${req.method()} ${u.replace(/.*\/ui\//, '/')}`, elapsedS());
      }
    });
    page.on('response', async resp => {
      const u = resp.url();
      if (u.includes('/ui/') && resp.request().method() !== 'OPTIONS') {
        const path = u.replace(/.*\/ui\//, '/');
        const s = resp.status();
        const m = resp.request().method();
        let bodyStr = '';
        try { bodyStr = (await resp.body()).toString().substring(0, 2048); } catch {}
        emit('http_call', `${m} ${path} → ${s}`, elapsedS(), {
          method: m, url: path, status_code: s,
          response_body: bodyStr,
        });
      }
    });
    page.on('framenavigated', frame => {
      if (frame === page.mainFrame()) {
        emit('log', frame.url().replace(/.*\/ui\//, '/'), elapsedS());
      }
    });
    page.on('console', msg => {
      const t = msg.text();
      if (t.startsWith('[step]') || t.startsWith('[action]')) {
        emit('log', t, elapsedS());
      }
    });

    const runId = await registerRun(fullPath, category);
    rid = runId;
    if (runId) {
      await insertEvent(runId, 'state_change', 'test started', 0);
      await insertEvent(runId, 'log', `file: ${fileShort}`, 0.01);
      await insertEvent(runId, 'log', `category: ${category}`, 0.02);
      flush();
    }

    try {
      await use(runId);
      if (!runId) return;
      const artifactUrls = await saveArtifacts(runId, page, testInfo);
      await markDone(runId, testInfo.status === 'passed', testInfo.error?.message, artifactUrls);
    } catch (e: any) {
      if (runId) {
        const artifactUrls = await saveArtifacts(runId, page, testInfo).catch(() => []);
        await markDone(runId, false, e?.message || String(e), artifactUrls);
      }
      throw e;
    }
  }, { auto: true }],
});

export { expect };
