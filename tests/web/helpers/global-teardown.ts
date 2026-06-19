import * as fs from 'fs';

const PID_FILE = '/tmp/runlog-e2e-daemon.pid';
const BINARIES = ['/tmp/runlog-e2e'];

async function globalTeardown(): Promise<void> {
  // Kill e2e daemon
  try {
    const pid = Number(fs.readFileSync(PID_FILE, 'utf8').trim());
    process.kill(pid, 'SIGTERM');
    console.log(`global-teardown: killed daemon PID ${pid}`);
  } catch { /* already dead or no pid file */ }

  // Clean up binaries
  for (const p of BINARIES) {
    try { fs.unlinkSync(p); } catch { /* ignore */ }
  }
  try { fs.unlinkSync(PID_FILE); } catch { /* ignore */ }
}

export default globalTeardown;
