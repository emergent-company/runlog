import { execSync, spawn } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';

const PROJECT_ROOT = path.resolve(__dirname, '..', '..', '..');
const DAEMON_BIN = '/tmp/runlog-e2e';
const DB_PATH = '/tmp/runlog-e2e.db';
const DAEMON_PORT = 17430;
const PID_FILE = '/tmp/runlog-e2e-daemon.pid';
const LOG_FILE = '/tmp/runlog-e2e-daemon.log';

async function globalSetup(): Promise<void> {
  // Clean previous state
  for (const f of [DB_PATH, PID_FILE, LOG_FILE]) {
    try { fs.unlinkSync(f); } catch { /* ignore */ }
  }

  // 1. Build daemon binary
  execSync(`go build -o ${DAEMON_BIN} ./cmd/runlog`, {
    cwd: PROJECT_ROOT,
    stdio: 'inherit',
  });

  // 2. Seed test database with fixture data for Playwright tests
  execSync(`go run ./tests/web/fixture/ --db ${DB_PATH}`, {
    cwd: PROJECT_ROOT,
    stdio: 'inherit',
  });

  // 3. Start daemon with --daemon
  const logFd = fs.openSync(LOG_FILE, 'a');
  const proc = spawn(DAEMON_BIN, ['--daemon', '--db', DB_PATH, '--port', String(DAEMON_PORT)], {
    cwd: PROJECT_ROOT,
    stdio: ['ignore', logFd, logFd],
    detached: false,
  });
  fs.writeFileSync(PID_FILE, String(proc.pid!));

  // 4. Wait for /health
  const maxWait = 10_000;
  const pollMs = 200;
  const deadline = Date.now() + maxWait;
  let healthy = false;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`http://localhost:${DAEMON_PORT}/health`);
      if (res.ok) { healthy = true; break; }
    } catch { /* not ready yet */ }
    await new Promise(r => setTimeout(r, pollMs));
  }
  if (!healthy) {
    const log = fs.readFileSync(LOG_FILE, 'utf8').slice(-2000);
    console.error('Daemon logs:\n', log);
    throw new Error(`Daemon did not become healthy within ${maxWait}ms`);
  }
  console.log(`global-setup: daemon ready on port ${DAEMON_PORT} (PID ${proc.pid})`);
}

export default globalSetup;
